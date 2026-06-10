package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/utility/clusterbus"
	"SuperBizAgent/utility/leader"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

// Subscriber 是变更事件的 SSE 订阅者。
type Subscriber struct {
	ID     string
	Ch     chan *protocol.ChangeEvent
	Filter ChangeEventFilter
}

// ChangeEventFilter 是订阅过滤条件。
type ChangeEventFilter struct {
	Services []string // 只接收这些服务的变更
	Env      string   // 只接收这个环境的变更
}

// NotificationBroker 管理变更事件的 SSE 广播。
// 多实例语义：本实例 ingest 的事件本地 fan-out 之后还会 publish 到 clusterbus；
// 其他实例 subscribe 到同一个 channel，收到 ingest 事件后只做本地 fan-out（不再回写）。
// 通过 sourceInstance 字段去重，避免本实例自己 publish 收到自己再 fan-out 一次。
type NotificationBroker struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscriber
	nextID      int

	// 跨实例广播；nil 时退化为单实例模式（仅本地 fan-out）
	cluster        *clusterbus.Bus
	clusterChannel string
	clusterCancel  context.CancelFunc
	instanceID     string
}

// crossInstanceEnvelope 是跨实例广播的 wire 格式。
// source 字段防止本实例 publish 后收到自己再次 fan-out（环路）。
type crossInstanceEnvelope struct {
	Source string                 `json:"source"`
	Event  *protocol.ChangeEvent  `json:"event"`
}

// NewNotificationBroker 创建通知广播器。单实例模式（不跨实例）。
// 多实例部署用 NewNotificationBrokerWithCluster。
func NewNotificationBroker() *NotificationBroker {
	return &NotificationBroker{
		subscribers: make(map[string]*Subscriber),
		instanceID:  leader.InstanceID(),
	}
}

// NewNotificationBrokerWithCluster 创建支持跨实例广播的 broker。
// channel 是 clusterbus 上的 channel 名（建议 "change_events"）。
// 启动后会订阅该 channel；收到来自其他实例的事件会本地 fan-out 给 SSE 订阅者。
// ctx 控制 subscriber 生命周期；调用方在 shutdown 时取消即可。
func NewNotificationBrokerWithCluster(ctx context.Context, bus *clusterbus.Bus, channel string) *NotificationBroker {
	nb := NewNotificationBroker()
	if bus == nil || strings.TrimSpace(channel) == "" {
		return nb
	}
	nb.cluster = bus
	nb.clusterChannel = channel
	cancel, err := bus.Subscribe(ctx, channel, nb.handleClusterMessage)
	if err != nil {
		g.Log().Warningf(ctx, "[change_event] cluster subscribe failed (degrading to single-instance): %v", err)
		nb.cluster = nil
		return nb
	}
	nb.clusterCancel = cancel
	g.Log().Infof(ctx, "[change_event] cluster broker subscribed to %s (instance=%s)", channel, nb.instanceID)
	return nb
}

// Subscribe 订阅变更通知。返回一个 channel 和取消函数。
func (nb *NotificationBroker) Subscribe(ctx context.Context, filter ChangeEventFilter) (string, <-chan *protocol.ChangeEvent, func()) {
	nb.mu.Lock()
	defer nb.mu.Unlock()

	nb.nextID++
	id := fmt.Sprintf("sub_%d", nb.nextID)

	ch := make(chan *protocol.ChangeEvent, 64)
	sub := &Subscriber{
		ID:     id,
		Ch:     ch,
		Filter: filter,
	}
	nb.subscribers[id] = sub

	cancel := func() {
		nb.Unsubscribe(id)
	}

	return id, ch, cancel
}

// Unsubscribe 取消订阅。
func (nb *NotificationBroker) Unsubscribe(subscriberID string) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if sub, ok := nb.subscribers[subscriberID]; ok {
		close(sub.Ch)
		delete(nb.subscribers, subscriberID)
	}
}

// Name 实现 ChangeEventHandler 接口。
func (nb *NotificationBroker) Name() string {
	return "notification_broker"
}

// Handle 将变更事件推送给本实例的匹配订阅者，并广播到 cluster 让其他实例也能 fan-out。
func (nb *NotificationBroker) Handle(ctx context.Context, event *protocol.ChangeEvent) error {
	nb.dispatchLocal(ctx, event)
	nb.publishToCluster(ctx, event)
	return nil
}

// dispatchLocal 仅 fan-out 给本实例 SSE 订阅者，不再 publish 到 cluster。
// 用于：(a) 本实例 ingest 调 Handle 时；(b) 从 cluster 收到其他实例 publish 的事件时。
func (nb *NotificationBroker) dispatchLocal(ctx context.Context, event *protocol.ChangeEvent) {
	nb.mu.RLock()
	defer nb.mu.RUnlock()

	for _, sub := range nb.subscribers {
		if !nb.matchesFilter(event, sub.Filter) {
			continue
		}
		select {
		case sub.Ch <- event:
		default:
			// 订阅者 channel 满了，跳过（不阻塞）
			g.Log().Debugf(ctx, "[change_event] subscriber %s channel full, skipping", sub.ID)
		}
	}
}

// publishToCluster 把事件序列化后广播到 clusterbus。
// 失败不影响本地 fan-out（属于「best effort」广播）。
func (nb *NotificationBroker) publishToCluster(ctx context.Context, event *protocol.ChangeEvent) {
	if nb.cluster == nil || nb.clusterChannel == "" {
		return
	}
	payload, err := json.Marshal(crossInstanceEnvelope{
		Source: nb.instanceID,
		Event:  event,
	})
	if err != nil {
		g.Log().Warningf(ctx, "[change_event] cluster publish marshal failed: %v", err)
		return
	}
	if err := nb.cluster.Publish(ctx, nb.clusterChannel, payload); err != nil {
		g.Log().Warningf(ctx, "[change_event] cluster publish failed: %v", err)
	}
}

// handleClusterMessage 是 clusterbus subscribe 的回调。
// 收到其他实例发来的事件 → dispatchLocal；收到自己发的（echo）→ 跳过。
func (nb *NotificationBroker) handleClusterMessage(ctx context.Context, payload []byte) {
	var envelope crossInstanceEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		g.Log().Warningf(ctx, "[change_event] cluster message unmarshal failed: %v", err)
		return
	}
	if envelope.Event == nil {
		return
	}
	if envelope.Source == nb.instanceID {
		// 本实例自己 publish 的 echo，跳过避免双发
		return
	}
	nb.dispatchLocal(ctx, envelope.Event)
}

// Close 关闭跨实例订阅 + 清理所有本地订阅者（用于 shutdown）。
func (nb *NotificationBroker) Close() {
	if nb == nil {
		return
	}
	if nb.clusterCancel != nil {
		nb.clusterCancel()
		nb.clusterCancel = nil
	}
	nb.mu.Lock()
	defer nb.mu.Unlock()
	for id, sub := range nb.subscribers {
		close(sub.Ch)
		delete(nb.subscribers, id)
	}
}

// matchesFilter 检查事件是否匹配订阅过滤条件。
func (nb *NotificationBroker) matchesFilter(event *protocol.ChangeEvent, filter ChangeEventFilter) bool {
	if len(filter.Services) > 0 {
		matched := false
		for _, svc := range filter.Services {
			if strings.EqualFold(event.Service, svc) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if filter.Env != "" && !strings.EqualFold(event.Env, filter.Env) {
		return false
	}
	return true
}
