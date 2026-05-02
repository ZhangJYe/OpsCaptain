package events

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
)

// TraceEmitter 将 AgentEvent 写入日志（用于 trace/audit）
type TraceEmitter struct {
	traceID string
}

// NewTraceEmitter 创建 trace 日志发射器
func NewTraceEmitter(traceID string) *TraceEmitter {
	return &TraceEmitter{traceID: traceID}
}

// Emit 写入结构化日志
func (t *TraceEmitter) Emit(ctx context.Context, event AgentEvent) {
	if event.TraceID == "" {
		event.TraceID = t.traceID
	}

	switch event.Type {
	case EventToolCallStart:
		g.Log().Infof(ctx, "[agent_event] trace=%s type=%s tool=%s",
			event.TraceID, event.Type, event.Payload["tool_name"])

	case EventToolCallEnd:
		g.Log().Infof(ctx, "[agent_event] trace=%s type=%s tool=%s duration_ms=%v success=%v",
			event.TraceID, event.Type,
			event.Payload["tool_name"],
			event.Payload["duration_ms"],
			event.Payload["success"],
		)

	case EventModelStart:
		g.Log().Infof(ctx, "[agent_event] trace=%s type=%s model=%s",
			event.TraceID, event.Type, event.Name)

	case EventModelEnd:
		g.Log().Infof(ctx, "[agent_event] trace=%s type=%s model=%s duration_ms=%v tokens=%v",
			event.TraceID, event.Type,
			event.Name,
			event.Payload["duration_ms"],
			event.Payload["total_tokens"],
		)

	case EventError:
		g.Log().Warningf(ctx, "[agent_event] trace=%s type=%s error=%s",
			event.TraceID, event.Type, event.Payload["error"])

	default:
		g.Log().Debugf(ctx, "[agent_event] trace=%s type=%s", event.TraceID, event.Type)
	}
}
