package service

import (
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/utility/safety"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// changeEventBusHolder 通过 atomic.Pointer 暴露 ChangeEventBus，
// 由 main 在启动阶段注入，被 AIOps 请求路径并发读取。
// 包级 var 跨 goroutine 共享必须有同步，避免 -race 触发。
var changeEventBusHolder atomic.Pointer[changeEventQuery]

// changeEventQuery is the subset of ChangeEventBus needed for context injection.
type changeEventQuery interface {
	Query(ctx context.Context, filter protocol.ChangeEventFilter) ([]*protocol.ChangeEvent, error)
	RecentByService(service string, since time.Time, limit int) []*protocol.ChangeEvent
	RecentAll(since time.Time, limit int) []*protocol.ChangeEvent
}

// SetChangeEventBus 注入变更事件总线（从 main 调用）。传 nil 表示禁用。
func SetChangeEventBus(bus changeEventQuery) {
	if bus == nil {
		changeEventBusHolder.Store(nil)
		return
	}
	changeEventBusHolder.Store(&bus)
}

// loadChangeEventBus 原子读取当前注入的总线，可能为 nil。
func loadChangeEventBus() changeEventQuery {
	ptr := changeEventBusHolder.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

type aiOpsMemory interface {
	ResolveSessionID(ctx context.Context) string
	BuildContextPlan(ctx context.Context, mode, sessionID, query string) (string, []protocol.MemoryRef, []string)
	PersistOutcome(ctx context.Context, sessionID, query, summary string)
}

type aiOpsIncidentContextKey struct{}

func WithAIOpsIncidentContext(ctx context.Context, contextText string) context.Context {
	contextText = strings.TrimSpace(contextText)
	if ctx == nil || contextText == "" {
		return ctx
	}
	return context.WithValue(ctx, aiOpsIncidentContextKey{}, contextText)
}

func aiOpsIncidentContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(aiOpsIncidentContextKey{}).(string)
	return strings.TrimSpace(value)
}

// buildChangeContext 从查询中提取服务名，查询最近变更事件，构建上下文。
// 所有写入 LLM prompt 的外部字段（summary/before/after/diff/operator）都会
// 经过 safety.SanitizeForLLMContext 清洗，避免变更事件成为 prompt-injection 通道。
func buildChangeContext(ctx context.Context, query string) string {
	bus := loadChangeEventBus()
	if bus == nil {
		return ""
	}
	services := extractServiceNames(query)
	since := time.Now().Add(-2 * time.Hour)
	if len(services) == 0 {
		since = time.Now().Add(-30 * time.Minute)
	}
	filter := protocol.ChangeEventFilter{
		Services: services,
		Since:    &since,
		Limit:    5,
	}
	changes, err := bus.Query(ctx, filter)
	if err != nil {
		// 多实例下不能再走 RecentByService 内存兜底：ringBuffer 只看本实例的事件，
		// 跨实例查询会得到不同子集（A 收到的 webhook B 看不到）。
		// 这里改为「直接返回空 + 警告」，让调用方明确知道关联数据缺失，
		// 而不是基于本地缓存编出一份「看起来有但不准」的上下文。
		g.Log().Warningf(ctx, "[AIOps] change-event Query failed (skipping change context): %v", err)
		return ""
	}
	if len(changes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n最近相关变更事件（结构化查询）：\n")
	for _, c := range changes {
		b.WriteString(fmt.Sprintf("- [%s] %s %s@%s: %s (风险:%s, 操作者:%s, 时间:%s)\n",
			safety.SanitizeForLLMContext(c.EventType),
			safety.SanitizeForLLMContext(c.Service),
			safety.SanitizeForLLMContext(c.Env),
			safety.SanitizeForLLMContext(c.Cluster),
			safety.SanitizeForLLMContext(c.Summary),
			safety.SanitizeForLLMContext(c.RiskLevel),
			safety.SanitizeForLLMContext(c.Operator),
			c.StartedAt.Format("15:04")))
		if c.Before != nil || c.After != nil {
			b.WriteString(fmt.Sprintf("  Before: %s → After: %s\n",
				safety.SanitizeForLLMContext(stringifyMap(c.Before)),
				safety.SanitizeForLLMContext(stringifyMap(c.After))))
		}
		if c.Diff != "" {
			b.WriteString(fmt.Sprintf("  Diff: %s\n", safety.SanitizeForLLMContext(c.Diff)))
		}
	}
	return b.String()
}

// stringifyMap 用稳定排序的 key 序列化 map，避免 fmt %v 的随机顺序破坏可重现性。
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

// extractServiceNames 从查询文本中提取可能的服务名。
// 使用简单的启发式：查找包含 "-" 的词（如 user-service, order-api）。
func extractServiceNames(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	var services []string
	seen := make(map[string]bool)
	for _, w := range words {
		// 清理标点
		w = strings.Trim(w, ",.;:!?。，；：！？")
		if len(w) > 3 && strings.Contains(w, "-") && !seen[w] {
			seen[w] = true
			services = append(services, w)
		}
	}
	return services
}