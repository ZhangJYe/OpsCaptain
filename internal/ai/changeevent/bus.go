package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

// ChangeEventHandler 是变更事件处理器接口。
type ChangeEventHandler interface {
	Name() string
	Handle(ctx context.Context, event *protocol.ChangeEvent) error
}

// ChangeEventBus 是变更事件的核心处理管道。
// 接收标准化的 ChangeEvent，依次执行：Normalize → Dedupe → Enrich → Fan-out。
type ChangeEventBus struct {
	mu           sync.RWMutex
	handlers     []ChangeEventHandler
	store        ChangeEventStore
	recentBuffer *ringBuffer
}

// NewChangeEventBus 创建变更事件总线。
func NewChangeEventBus(store ChangeEventStore, bufferSize int) *ChangeEventBus {
	return &ChangeEventBus{
		store:        store,
		recentBuffer: newRingBuffer(bufferSize),
	}
}

// Register 注册变更事件处理器。
func (b *ChangeEventBus) Register(handler ChangeEventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, handler)
}

// Ingest 是事件接入的主入口。
// 依次执行：Normalize → Dedupe → Enrich → Persist → Fan-out。
func (b *ChangeEventBus) Ingest(ctx context.Context, raw *protocol.ChangeEvent) (string, bool, error) {
	if raw == nil {
		return "", false, fmt.Errorf("change event is nil")
	}

	// 1. Normalize
	if err := normalizeEvent(raw); err != nil {
		return "", false, fmt.Errorf("normalize: %w", err)
	}

	// 2. Dedupe (atomic reserve)
	if raw.DedupeKey == "" {
		raw.DedupeKey = generateDedupeKey(raw)
	}
	reserved, existingID, err := b.store.ReserveDedupeKey(ctx, raw.DedupeKey, raw.EventID)
	if err != nil {
		return "", false, fmt.Errorf("reserve dedupe key: %w", err)
	}
	if !reserved {
		g.Log().Debugf(ctx, "[change_event] duplicate event ignored, dedupe_key=%s", raw.DedupeKey)
		if existingID != "" {
			return existingID, false, nil
		}
		return raw.EventID, false, nil
	}

	// 3. Enrich
	enrichEvent(raw)

	// 4. Persist (store + ring buffer)
	if err := b.store.Save(ctx, raw); err != nil {
		_ = b.store.ReleaseDedupeKey(ctx, raw.DedupeKey)
		return "", false, fmt.Errorf("persist: %w", err)
	}
	// ring buffer 持有一份独立深拷贝，避免后续 handler 修改 map
	// 影响 RecentByService 的并发读者。
	b.recentBuffer.Add(deepCopyEvent(raw))

	g.Log().Infof(ctx, "[change_event] ingested: id=%s service=%s type=%s risk=%s",
		raw.EventID, raw.Service, raw.EventType, raw.RiskLevel)

	// 5. Fan-out to handlers (async, don't block ingest)
	b.mu.RLock()
	handlers := make([]ChangeEventHandler, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	// 派生一个无取消信号但保留追踪/请求元数据的 ctx，
	// 避免 context.Background() 让 fan-out goroutine 脱离 trace 链路。
	parentCtx := context.WithoutCancel(ctx)
	for _, h := range handlers {
		handler := h
		// 深拷贝事件：浅拷贝只复制 struct 头，Before/After/RawPayload/Metadata
		// 都是 map，引用相同的底层哈希。多 handler 并发读写会触发
		// "concurrent map iteration and map write" 运行时崩溃。
		eventCopy := deepCopyEvent(raw)
		go func() {
			bgCtx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
			defer cancel()
			if err := handler.Handle(bgCtx, eventCopy); err != nil {
				g.Log().Warningf(bgCtx, "[change_event] handler %s failed: %v", handler.Name(), err)
			}
		}()
	}

	return raw.EventID, true, nil
}

// RecentByService 返回指定服务最近的变更事件（内存快速路径）。
func (b *ChangeEventBus) RecentByService(service string, since time.Time, limit int) []*protocol.ChangeEvent {
	return b.recentBuffer.RecentByService(service, since, limit)
}

// RecentAll 返回最近的变更事件（内存快速路径）。
func (b *ChangeEventBus) RecentAll(since time.Time, limit int) []*protocol.ChangeEvent {
	return b.recentBuffer.RecentAll(since, limit)
}

// Query performs structured query against the source-of-truth event store.
func (b *ChangeEventBus) Query(ctx context.Context, filter protocol.ChangeEventFilter) ([]*protocol.ChangeEvent, error) {
	if b == nil || b.store == nil {
		return nil, fmt.Errorf("change event store is not configured")
	}
	return b.store.Query(ctx, filter)
}

// Store 返回底层存储（用于结构化查询）。
func (b *ChangeEventBus) Store() ChangeEventStore {
	return b.store
}

// normalizeEvent 填充默认值、生成 event_id、验证必填字段。
func normalizeEvent(event *protocol.ChangeEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	if event.StartedAt.IsZero() {
		event.StartedAt = event.CreatedAt
	}
	if event.FinishedAt.IsZero() {
		event.FinishedAt = event.StartedAt
	}

	// 验证必填字段
	if strings.TrimSpace(event.Service) == "" {
		return fmt.Errorf("service is required")
	}
	event.Service = strings.TrimSpace(event.Service)

	if strings.TrimSpace(event.EventType) == "" {
		return fmt.Errorf("event_type is required")
	}
	event.EventType = strings.TrimSpace(event.EventType)

	// 默认值
	if event.Env == "" {
		event.Env = "unknown"
	}
	if event.Source == "" {
		event.Source = protocol.ChangeSourceManual
	}
	if event.Summary == "" {
		event.Summary = fmt.Sprintf("%s %s on %s", event.EventType, event.Service, event.Env)
	}

	return nil
}

// enrichEvent 从 metadata 提取关联键、评估风险等级。
func enrichEvent(event *protocol.ChangeEvent) {
	// 自动评估风险等级（如果未指定）
	if event.RiskLevel == "" {
		event.RiskLevel = assessRiskLevel(event)
	}

	// 构建关联键
	if len(event.CorrelationKeys) == 0 {
		keys := []string{strings.ToLower(event.Service)}
		if event.Env != "" {
			keys = append(keys, strings.ToLower(event.Env))
		}
		if event.Cluster != "" {
			keys = append(keys, strings.ToLower(event.Cluster))
		}
		event.CorrelationKeys = keys
	}
}

// assessRiskLevel 根据变更内容自动评估风险等级。
func assessRiskLevel(event *protocol.ChangeEvent) string {
	isProd := strings.EqualFold(event.Env, "prod")

	// 生产环境 deploy/rollback → high
	if isProd && (event.EventType == protocol.ChangeTypeDeploy || event.EventType == protocol.ChangeTypeRollback) {
		return protocol.ChangeRiskHigh
	}
	// 生产环境 config_update → medium
	if isProd && event.EventType == protocol.ChangeTypeConfigUpdate {
		return protocol.ChangeRiskMedium
	}
	// 生产环境 scale/restart → medium
	if isProd && (event.EventType == protocol.ChangeTypeScale || event.EventType == protocol.ChangeTypeRestart) {
		return protocol.ChangeRiskMedium
	}
	// 非生产环境 → low
	return protocol.ChangeRiskLow
}

// generateDedupeKey 生成幂等键。
func generateDedupeKey(event *protocol.ChangeEvent) string {
	payload, _ := json.Marshal(map[string]any{
		"summary": event.Summary,
		"before":  event.Before,
		"after":   event.After,
		"diff":    event.Diff,
		"meta":    event.Metadata,
	})
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%s:%s:%s:%s",
		event.Source,
		strings.ToLower(event.Service),
		event.EventType,
		event.StartedAt.UTC().Format(time.RFC3339Nano)+":"+hex.EncodeToString(sum[:6]),
	)
}

// deepCopyEvent 复制 ChangeEvent 的所有 map / slice 字段，
// 让各 handler、ringBuffer 之间不共享底层引用，从根本上避免 map 并发读写。
func deepCopyEvent(src *protocol.ChangeEvent) *protocol.ChangeEvent {
	if src == nil {
		return nil
	}
	dst := *src // 复制 struct 头（含值类型字段）
	dst.Before = copyAnyMap(src.Before)
	dst.After = copyAnyMap(src.After)
	dst.RawPayload = copyAnyMap(src.RawPayload)
	dst.Metadata = copyAnyMap(src.Metadata)
	if len(src.CorrelationKeys) > 0 {
		dst.CorrelationKeys = append([]string(nil), src.CorrelationKeys...)
	}
	return &dst
}

func copyAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
