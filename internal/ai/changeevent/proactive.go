package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/utility/safety"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// AIOpsRunner 是 AIOps 分析的抽象接口。
// 避免 ProactiveAnalyzer 绑死具体实现。
type AIOpsRunner interface {
	RunAsync(ctx context.Context, query string) (*RunInfo, error)
}

// AsyncTimeoutOverride 允许 runner 适配层把 ProactiveAnalyzer 的
// InspectionTimeout 透传到下游 RunAIOpsAsync 的派生 goroutine，
// 让"巡检 N 秒后自动收尾"真正生效。
type AsyncTimeoutOverride interface {
	WithAsyncTimeout(ctx context.Context, timeout time.Duration) context.Context
}

// RunInfo 是异步分析的运行信息。
type RunInfo struct {
	TraceID string
	TaskID  string
	Status  string
}

// ProactiveAnalyzerConfig 配置主动巡检触发策略。
type ProactiveAnalyzerConfig struct {
	Enabled           bool     // 是否启用
	DebounceSeconds   int      // 同服务变更去重窗口（秒）
	RequireEnv        string   // 必须的环境（如 "prod"）
	RequireRiskLevel  string   // 最低风险等级
	RequireEventTypes []string // 必须的变更类型
	InspectionTimeout int      // 巡检超时（毫秒）
}

// DefaultProactiveAnalyzerConfig 返回默认配置。
func DefaultProactiveAnalyzerConfig() ProactiveAnalyzerConfig {
	return ProactiveAnalyzerConfig{
		Enabled:           true,
		DebounceSeconds:   300,
		RequireEnv:        "prod",
		RequireRiskLevel:  protocol.ChangeRiskMedium,
		RequireEventTypes: []string{protocol.ChangeTypeDeploy, protocol.ChangeTypeRollback, protocol.ChangeTypeConfigUpdate, protocol.ChangeTypeScale},
		InspectionTimeout: 120000,
	}
}

// ProactiveAnalyzer 在高风险变更事件到达后触发主动巡检分析。
type ProactiveAnalyzer struct {
	runner   AIOpsRunner
	config   ProactiveAnalyzerConfig
	debounce *DebounceTracker
}

// NewProactiveAnalyzer 创建主动巡检分析器。
func NewProactiveAnalyzer(runner AIOpsRunner, config ProactiveAnalyzerConfig) *ProactiveAnalyzer {
	return &ProactiveAnalyzer{
		runner:   runner,
		config:   config,
		debounce: NewDebounceTracker(time.Duration(config.DebounceSeconds) * time.Second),
	}
}

// Name 实现 ChangeEventHandler 接口。
func (pa *ProactiveAnalyzer) Name() string {
	return "proactive_analyzer"
}

// Handle 对高风险变更事件触发主动巡检。
func (pa *ProactiveAnalyzer) Handle(ctx context.Context, event *protocol.ChangeEvent) error {
	if !pa.config.Enabled {
		return nil
	}
	if !pa.shouldInspect(event) {
		return nil
	}
	if pa.debounce.IsDuplicate(event.Service) {
		g.Log().Debugf(ctx, "[change_event] proactive analysis debounced for service=%s", event.Service)
		return nil
	}

	query := buildInspectionQuery(event)
	g.Log().Infof(ctx, "[change_event] triggering proactive analysis: service=%s type=%s risk=%s",
		event.Service, event.EventType, event.RiskLevel)

	if pa.runner == nil {
		g.Log().Warningf(ctx, "[change_event] AIOpsRunner not configured, skipping proactive analysis")
		return nil
	}

	runCtx := ctx
	// 通过 AsyncTimeoutOverride 把 InspectionTimeout 透传到 runner 派生的
	// 后台 goroutine 而不是简单 WithTimeout —— 后者会在 RunAsync 同步返回后
	// 立刻被 defer cancel() 取消，导致 dispatch goroutine 用默认 5min 超时。
	if pa.config.InspectionTimeout > 0 {
		timeout := time.Duration(pa.config.InspectionTimeout) * time.Millisecond
		if applier, ok := pa.runner.(AsyncTimeoutOverride); ok {
			runCtx = applier.WithAsyncTimeout(runCtx, timeout)
		} else {
			// 退化路径：runner 不支持透传时，仍然给同步部分加上时长上限。
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(runCtx, timeout)
			defer cancel()
		}
	}
	runInfo, err := pa.runner.RunAsync(runCtx, query)
	if err != nil {
		g.Log().Warningf(ctx, "[change_event] proactive analysis failed: %v", err)
		return nil
	}

	g.Log().Infof(ctx, "[change_event] proactive analysis started: trace_id=%s service=%s",
		runInfo.TraceID, event.Service)
	return nil
}

// shouldInspect 决定是否对变更事件触发主动巡检。
func (pa *ProactiveAnalyzer) shouldInspect(event *protocol.ChangeEvent) bool {
	// 必须匹配环境
	if pa.config.RequireEnv != "" && !strings.EqualFold(event.Env, pa.config.RequireEnv) {
		return false
	}
	// 必须满足最低风险等级
	if !meetsRiskLevel(event.RiskLevel, pa.config.RequireRiskLevel) {
		return false
	}
	// 必须是指定的变更类型
	if len(pa.config.RequireEventTypes) > 0 {
		matched := false
		for _, t := range pa.config.RequireEventTypes {
			if event.EventType == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// buildInspectionQuery 构建巡检查询。
// 变更事件可能来自外部 webhook（commit message、PR title 等用户可控字段），
// 对会进入 LLM 上下文的所有自由文本统一做 prompt-injection 清洗。
func buildInspectionQuery(event *protocol.ChangeEvent) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("服务 %s 刚完成了 %s 操作",
		safety.SanitizeForLLMContext(event.Service),
		safety.SanitizeForLLMContext(event.EventType)))
	if event.Env != "" {
		b.WriteString(fmt.Sprintf("（环境: %s）", safety.SanitizeForLLMContext(event.Env)))
	}
	b.WriteString("。\n")
	b.WriteString(fmt.Sprintf("变更摘要: %s\n", safety.SanitizeForLLMContext(event.Summary)))
	if event.Before != nil || event.After != nil {
		b.WriteString(fmt.Sprintf("变更前: %s → 变更后: %s\n",
			safety.SanitizeForLLMContext(stringifyMap(event.Before)),
			safety.SanitizeForLLMContext(stringifyMap(event.After))))
	}
	if event.Diff != "" {
		b.WriteString(fmt.Sprintf("差异: %s\n", safety.SanitizeForLLMContext(event.Diff)))
	}
	b.WriteString("\n请检查该服务的错误率、延迟、CPU、内存是否有异常变化。如果发现异常，请分析是否与本次变更相关。")
	return b.String()
}

// stringifyMap 按 key 排序后序列化，避免 fmt %v 的 map 遍历顺序随机。
func stringifyMap(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString(": ")
		fmt.Fprintf(&b, "%v", m[k])
	}
	b.WriteByte('}')
	return b.String()
}

// meetsRiskLevel 检查实际风险等级是否满足最低要求。
// 未知的 required 值返回 false（安全默认：不触发）；未知的 actual 值视为最低风险。
func meetsRiskLevel(actual, required string) bool {
	riskOrder := map[string]int{
		"low": 0, "medium": 1, "high": 2, "critical": 3,
	}
	reqVal, reqOK := riskOrder[required]
	if !reqOK || required == "" {
		return false
	}
	actVal := riskOrder[actual] // 未知值返回 0，视为 low
	return actVal >= reqVal
}

// DebounceTracker 跟踪同服务变更的去重。
// 多实例语义：构造时传入 g.Redis() → Claim 走 Redis SETNX（跨实例去重）；
// 传 nil → 仍走本地 map（单实例向后兼容）。
type DebounceTracker struct {
	mu     sync.Mutex
	recent map[string]time.Time
	window time.Duration

	// 跨实例去重；nil 表示仅本地
	redis  redisClaimer
	prefix string
}

// redisClaimer 抽象出 Redis SET NX EX 调用，便于在 NewDebounceTracker 不直接依赖
// gredis.Redis 包（同时方便单测注入）。
type redisClaimer interface {
	Do(ctx context.Context, command string, args ...interface{}) (any, error)
}

// NewDebounceTracker 创建去重跟踪器（单实例模式）。
func NewDebounceTracker(window time.Duration) *DebounceTracker {
	return &DebounceTracker{
		recent: make(map[string]time.Time),
		window: window,
	}
}

// NewDebounceTrackerWithRedis 创建跨实例去重跟踪器。
// Redis Claim 失败（已被其他实例 claim）→ IsDuplicate 返回 true。
// Redis 错误（连接 / 超时）→ 退化到本地 map（避免雪崩）。
func NewDebounceTrackerWithRedis(window time.Duration, redis redisClaimer, prefix string) *DebounceTracker {
	if strings.TrimSpace(prefix) == "" {
		prefix = "opscaption:debounce:"
	}
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return &DebounceTracker{
		recent: make(map[string]time.Time),
		window: window,
		redis:  redis,
		prefix: prefix,
	}
}

// IsDuplicate 检查是否在去重窗口内。
// 多实例模式下用 Redis SETNX EX（窗口=ttl）；任何一个实例先 claim 成功，
// 其他实例的同 service 在窗口内全部返回 true。
// 本地 map 作为「同进程瞬时去重」L1 缓存。
func (d *DebounceTracker) IsDuplicate(service string) bool {
	key := strings.ToLower(service)

	// 1) 同进程瞬时检查（避免 1 秒内连续触发，省一次 Redis 往返）
	d.mu.Lock()
	now := time.Now()
	// 顺带清理过期条目防止内存泄漏
	for k, t := range d.recent {
		if now.Sub(t) > d.window*2 {
			delete(d.recent, k)
		}
	}
	if last, ok := d.recent[key]; ok && now.Sub(last) < d.window {
		d.mu.Unlock()
		return true
	}
	d.recent[key] = now
	d.mu.Unlock()

	// 2) 跨实例 SETNX 兜底
	if d.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		ttlSec := int(d.window.Seconds())
		if ttlSec <= 0 {
			ttlSec = 1
		}
		val, err := d.redis.Do(ctx, "SET", d.prefix+key, "1", "NX", "EX", ttlSec)
		if err != nil {
			// Redis 错误退化到本地（已上面更新过 recent），保守返回 false（不阻断）
			return false
		}
		// SET NX 返回 nil/空 = 已存在 = 其他实例占住 = duplicate
		if val == nil {
			return true
		}
		if v, ok := val.(interface{ IsNil() bool }); ok && v.IsNil() {
			return true
		}
		// SET NX 返回 OK = 我们刚 claim 成功 = 非 duplicate
		return false
	}
	return false
}
