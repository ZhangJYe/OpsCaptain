package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// SSEClient SSE 客户端接口，抽象 sse.Client 的发送能力
type SSEClient interface {
	SendToClient(eventType, data string) bool
}

// SSEEmitter 将 AgentEvent 通过 SSE 推送给前端
type SSEEmitter struct {
	client  SSEClient
	traceID string
}

// NewSSEEmitter 创建 SSE 事件发射器
func NewSSEEmitter(client SSEClient, traceID string) *SSEEmitter {
	return &SSEEmitter{
		client:  client,
		traceID: traceID,
	}
}

// Emit 发射事件到 SSE
func (s *SSEEmitter) Emit(ctx context.Context, event AgentEvent) {
	if s.client == nil {
		return
	}

	// 填充 traceID
	if event.TraceID == "" {
		event.TraceID = s.traceID
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	data, err := json.Marshal(event)
	if err != nil {
		g.Log().Warningf(ctx, "[events] failed to marshal agent_event: %v", err)
		return
	}

	s.client.SendToClient("agent_event", string(data))
}
