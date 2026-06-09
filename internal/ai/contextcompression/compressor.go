package contextcompression

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/memory"
)

// Mode 压缩模式
type Mode string

const (
	ModeOff      Mode = "off"
	ModeAudit    Mode = "audit"
	ModeOptimize Mode = "optimize"
)

// SourceType 上下文来源类型
type SourceType string

const (
	SourceTool    SourceType = "tool"
	SourceRAG     SourceType = "rag"
	SourceHistory SourceType = "history"
)

// Request 压缩请求
type Request struct {
	SourceType SourceType
	SourceID   string
	Query      string
	Content    string
}

// Report 压缩报告
type Report struct {
	Enabled           bool    `json:"enabled"`
	Mode              string  `json:"mode"`
	SourceType        string  `json:"source_type"`
	SourceID          string  `json:"source_id"`
	Strategy          string  `json:"strategy"`
	TokensBefore      int     `json:"tokens_before"`
	TokensAfter       int     `json:"tokens_after"`
	CompressionRatio  float64 `json:"compression_ratio"`
	LatencyMs         int64   `json:"latency_ms"`
	Degraded          bool    `json:"degraded"`
	DegradationReason string  `json:"degradation_reason,omitempty"`
}

// Result 压缩结果
type Result struct {
	Content          string
	CandidateContent string
	Report           Report
}

// Compress 上下文压缩入口
// audit 模式: 返回原文 + report（用于采集数据，不改变实际内容）
// optimize 模式: 返回压缩内容 + report
func Compress(ctx context.Context, req Request, cfg *CompressionConfig) Result {
	start := time.Now()
	if cfg == nil {
		cfg = defaultConfig
	}

	result := Result{
		Content:          req.Content,
		CandidateContent: req.Content,
		Report: Report{
			Enabled:    cfg.Enabled,
			Mode:       string(cfg.Mode),
			SourceType: string(req.SourceType),
			SourceID:   req.SourceID,
		},
	}

	tokensBefore := memory.EstimateTokens(req.Content)
	result.Report.TokensBefore = tokensBefore

	// 条件检查: 未启用、off 模式、源类型不允许、内容太短
	if !cfg.Enabled || cfg.Mode == ModeOff {
		result.Report.Strategy = "disabled"
		result.Report.TokensAfter = tokensBefore
		result.Report.CompressionRatio = 1.0
		result.Report.LatencyMs = time.Since(start).Milliseconds()
		return result
	}
	if !cfg.SourceTypeAllowed(req.SourceType) {
		result.Report.Strategy = "source_type_excluded"
		result.Report.TokensAfter = tokensBefore
		result.Report.CompressionRatio = 1.0
		result.Report.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	if tokensBefore < cfg.MinTokens {
		result.Report.Strategy = "below_min_tokens"
		result.Report.TokensAfter = tokensBefore
		result.Report.CompressionRatio = 1.0
		result.Report.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	// 选择压缩策略
	compressed, strategy, degraded, reason := compressByStrategy(req, cfg)

	result.Report.Strategy = strategy
	result.Report.LatencyMs = time.Since(start).Milliseconds()

	if degraded {
		result.Report.Degraded = true
		result.Report.DegradationReason = reason
		result.Report.TokensAfter = tokensBefore
		result.Report.CompressionRatio = 1.0
		// degraded 时 audit 和 optimize 都返回原文
		return result
	}

	tokensAfter := memory.EstimateTokens(compressed)
	result.CandidateContent = compressed
	result.Report.TokensAfter = tokensAfter
	if tokensBefore > 0 {
		result.Report.CompressionRatio = float64(tokensAfter) / float64(tokensBefore)
	}

	// audit 模式: 返回原文，只记录 report
	if cfg.Mode == ModeAudit {
		return result
	}

	// optimize 模式下只在确实节省 token 时替换，避免压缩反而放大上下文。
	if tokensAfter >= tokensBefore {
		result.Report.Strategy = strategy + "_no_savings"
		result.Report.TokensAfter = tokensBefore
		result.Report.CompressionRatio = 1.0
		result.CandidateContent = req.Content
		return result
	}

	// optimize 模式: 返回压缩内容
	result.Content = compressed
	return result
}

// compressByStrategy 根据内容格式选择压缩策略
func compressByStrategy(req Request, cfg *CompressionConfig) (compressed, strategy string, degraded bool, reason string) {
	content := req.Content
	query := req.Query

	// 尝试 JSON 压缩
	if isJSONArray(content) {
		compressed, ok := compressJSON(content, query, cfg.PreserveFirst, cfg.PreserveLast)
		if ok {
			return compressed, "json_array", false, ""
		}
		// JSON 解析失败，fallback 到 log 策略
		return compressLog(content, query, cfg.LogContextLines), "log_fallback", false, ""
	}

	// 尝试 JSON 对象压缩
	if isJSONObject(content) {
		compressed, ok := compressJSONObject(content, query)
		if ok {
			return compressed, "json_object", false, ""
		}
		return compressLog(content, query, cfg.LogContextLines), "log_fallback", false, ""
	}

	// 日志/文本压缩
	return compressLog(content, query, cfg.LogContextLines), "log", false, ""
}

// isJSONArray 检测内容是否为 JSON 数组
func isJSONArray(content string) bool {
	s := strings.TrimSpace(content)
	return len(s) > 1 && s[0] == '[' && s[len(s)-1] == ']'
}

// isJSONObject 检测内容是否为 JSON 对象
func isJSONObject(content string) bool {
	s := strings.TrimSpace(content)
	return len(s) > 1 && s[0] == '{' && s[len(s)-1] == '}'
}

// compressJSONObject 尝试压缩 JSON 对象（提取关键字段）
func compressJSONObject(content, query string) (string, bool) {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(content), &obj); err != nil {
		return "", false
	}

	// 对于 JSON 对象，提取 error/warning 相关字段
	extracted := extractKeyFields(obj, query)
	if len(extracted) == 0 {
		return "", false
	}

	result, err := json.Marshal(extracted)
	if err != nil {
		return "", false
	}
	return string(result), true
}

// extractKeyFields 从 JSON 对象中提取关键字段
func extractKeyFields(obj map[string]interface{}, query string) map[string]interface{} {
	errorKeys := map[string]bool{
		"error": true, "err": true, "message": true, "msg": true,
		"status": true, "code": true, "reason": true, "detail": true,
		"stack": true, "stacktrace": true, "trace": true,
		"data": true, "result": true, "response": true,
	}

	queryLower := strings.ToLower(query)
	result := make(map[string]interface{})

	for key, val := range obj {
		keyLower := strings.ToLower(key)
		// 保留 error 相关字段
		if errorKeys[keyLower] {
			result[key] = val
			continue
		}
		// 保留值中包含 error 的字段
		if valStr, ok := val.(string); ok {
			valLower := strings.ToLower(valStr)
			if containsAny(valLower, errorKeywords) || strings.Contains(valLower, queryLower) {
				result[key] = val
			}
		}
	}

	return result
}

// errorKeywords 错误相关关键词
var errorKeywords = []string{
	"error", "warning", "failure", "fail", "failed",
	"timeout", "exception", "panic", "oom", "killed",
	"5xx", "500", "502", "503", "504",
	"4xx", "400", "401", "403", "404",
	"fatal", "critical", "severe",
	// Kubernetes / release evidence terms commonly appear in events rather than
	// conventional ERROR logs, but losing them breaks AIOps diagnosis.
	"crashloopbackoff", "imagepullbackoff", "errimagepull", "back-off",
	"oomkilled", "failedscheduling", "failedmount", "unhealthy",
	"failedcreatepodsandbox", "manifest unknown", "imagepullsecrets",
	"readiness probe failed", "liveness probe failed",
	"告警", "超时", "异常", "故障", "错误", "失败", "溢出", "耗尽",
}

// containsAny 检查 s 是否包含 keywords 中的任意一个
func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
