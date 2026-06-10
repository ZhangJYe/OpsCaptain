package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
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
// 不同于 events.SSEEmitter（单连接/单 trace），这是一个全局广播中心。
type NotificationBroker struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscriber
	nextID      int
}

// NewNotificationBroker 创建通知广播器。
func NewNotificationBroker() *NotificationBroker {
	return &NotificationBroker{
		subscribers: make(map[string]*Subscriber),
	}
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

// Handle 将变更事件推送给匹配的订阅者。
func (nb *NotificationBroker) Handle(ctx context.Context, event *protocol.ChangeEvent) error {
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
	return nil
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
