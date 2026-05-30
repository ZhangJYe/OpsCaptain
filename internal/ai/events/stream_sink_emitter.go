package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type StreamSinkSender interface {
	SendEvent(eventType, data string)
}

type StreamSinkEmitter struct {
	sender  StreamSinkSender
	traceID string
}

func NewStreamSinkEmitter(sender StreamSinkSender, traceID string) *StreamSinkEmitter {
	return &StreamSinkEmitter{
		sender:  sender,
		traceID: traceID,
	}
}

func (s *StreamSinkEmitter) Emit(ctx context.Context, event AgentEvent) {
	if s.sender == nil {
		return
	}
	if event.TraceID == "" {
		event.TraceID = s.traceID
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	data, err := json.Marshal(event)
	if err != nil {
		g.Log().Warningf(ctx, "[events] failed to marshal agent_event for stream sink: %v", err)
		return
	}
	s.sender.SendEvent("agent_event", string(data))
}
