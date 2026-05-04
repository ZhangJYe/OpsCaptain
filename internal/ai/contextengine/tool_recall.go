package contextengine

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ToolRecallResult 包含召回结果
type ToolRecallResult struct {
	Items     []ContextItem
	LatencyMs int64
}

// ToolRecaller 基于关键词匹配召回工具结果
type ToolRecaller struct {
	// 无状态，不需要外部依赖
}

// NewToolRecaller 创建 ToolRecaller
func NewToolRecaller() *ToolRecaller {
	return &ToolRecaller{}
}

// Recall 基于关键词匹配召回工具结果
func (r *ToolRecaller) Recall(ctx context.Context, query string, items []ContextItem, topK int) ToolRecallResult {
	start := time.Now()

	if len(items) <= topK {
		return ToolRecallResult{
			Items:     items,
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}

	// 1. 从 query 中提取关键词
	keywords := extractKeywords(query)

	// 2. 对每个 item 计算匹配分数
	scored := make([]scoredItem, len(items))
	for i, item := range items {
		scored[i] = scoredItem{
			item:  item,
			score: matchScore(keywords, item),
		}
	}

	// 3. 按分数排序，取 top-K
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	selected := make([]ContextItem, 0, topK)
	for i := 0; i < min(topK, len(scored)); i++ {
		selected = append(selected, scored[i].item)
	}

	return ToolRecallResult{
		Items:     selected,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

// scoredItem 带分数的 ContextItem
type scoredItem struct {
	item  ContextItem
	score float64
}

// matchScore 计算关键词和 item 的匹配分数
func matchScore(keywords []string, item ContextItem) float64 {
	if len(keywords) == 0 {
		return 0
	}

	score := 0.0

	// 优先匹配元数据字段（Title, SourceType, SourceID）— 权重更高
	metaText := strings.ToLower(item.Title + " " + item.SourceType + " " + item.SourceID)
	for _, kw := range keywords {
		if strings.Contains(metaText, strings.ToLower(kw)) {
			score += 2.0
		}
	}

	// 其次匹配内容
	content := strings.ToLower(item.Content)
	for _, kw := range keywords {
		if strings.Contains(content, strings.ToLower(kw)) {
			score += 1.0
		}
	}

	return score
}

// 服务名正则
var serviceNamePattern = regexp.MustCompile(`(?i)[a-z]+service`)

// 错误码正则
var errorCodePattern = regexp.MustCompile(`(?i)\b[45]\d{2}\b|connection\s*refused|timeout|error|failure|down`)

// 指标名正则
var metricPattern = regexp.MustCompile(`(?i)\b(cpu|memory|latency|qps|tps|error.?rate|disk|network)\b`)

// 实体词正则
var entityPattern = regexp.MustCompile(`(?i)\b(redis|mysql|kafka|pod|node|cluster|namespace|docker|k8s|kubernetes)\b`)

// 中文运维术语映射
var cnKeywordsMap = map[string]string{
	"超时": "timeout", "连接": "connection", "拒绝": "refused",
	"延迟": "latency", "挂了": "down", "异常": "error",
	"告警": "alert", "故障": "failure", "宕机": "down",
	"慢查询": "slow query", "瓶颈": "bottleneck", "抖动": "jitter",
	"飙升": "spike", "打满": "full", "耗尽": "exhausted",
	"堆积": "backlog", "阻塞": "block", "溢出": "overflow",
	"重启": "restart", "崩溃": "crash", "OOM": "oom",
}

// extractKeywords 从 query 中提取关键词
func extractKeywords(query string) []string {
	keywords := make([]string, 0)
	lowerQuery := strings.ToLower(query)

	// 1. 服务名：checkoutservice, paymentservice, ...
	keywords = append(keywords, serviceNamePattern.FindAllString(lowerQuery, -1)...)

	// 2. 错误码：503, 502, 404, connection refused, timeout, ...
	keywords = append(keywords, errorCodePattern.FindAllString(lowerQuery, -1)...)

	// 3. 指标名：cpu, memory, latency, qps, ...
	keywords = append(keywords, metricPattern.FindAllString(lowerQuery, -1)...)

	// 4. 实体词：Redis, MySQL, Kafka, Pod, ...
	keywords = append(keywords, entityPattern.FindAllString(lowerQuery, -1)...)

	// 5. 中文运维术语 → 映射为英文关键词（同时保留中文）
	for cn, en := range cnKeywordsMap {
		if strings.Contains(query, cn) {
			keywords = append(keywords, en, cn)
		}
	}

	// 6. 去重
	return uniqueStrings(keywords)
}

// uniqueStrings 字符串去重
func uniqueStrings(strs []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(strs))
	for _, s := range strs {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
