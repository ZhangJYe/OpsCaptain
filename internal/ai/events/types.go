package events

import "time"

// AgentEventType 事件类型枚举
type AgentEventType string

const (
	// EventModelStart 模型调用开始
	EventModelStart AgentEventType = "model_start"
	// EventModelEnd 模型调用结束
	EventModelEnd AgentEventType = "model_end"
	// EventToolCallStart 工具调用开始
	EventToolCallStart AgentEventType = "tool_call_start"
	// EventToolCallEnd 工具调用结束
	EventToolCallEnd AgentEventType = "tool_call_end"
	// EventTextDelta 文本增量
	EventTextDelta AgentEventType = "text_delta"
	// EventError 错误
	EventError AgentEventType = "error"
)

// AgentEvent 统一事件协议
type AgentEvent struct {
	Type      AgentEventType `json:"type"`
	TraceID   string         `json:"trace_id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// NewEvent 创建事件
func NewEvent(eventType AgentEventType, traceID, name string, payload map[string]any) AgentEvent {
	return AgentEvent{
		Type:      eventType,
		TraceID:   traceID,
		Name:      name,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

// ModelStartPayload 模型调用开始的 payload
type ModelStartPayload struct {
	ModelName string `json:"model_name"`
}

// ModelEndPayload 模型调用结束的 payload
type ModelEndPayload struct {
	ModelName        string `json:"model_name"`
	DurationMs       int64  `json:"duration_ms"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	Success          bool   `json:"success"`
	Error            string `json:"error,omitempty"`
}

// ToolCallStartPayload 工具调用开始的 payload
type ToolCallStartPayload struct {
	ToolName string `json:"tool_name"`
}

// ToolCallEndPayload 工具调用结束的 payload
type ToolCallEndPayload struct {
	ToolName   string `json:"tool_name"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

// ErrorPayload 错误事件的 payload
type ErrorPayload struct {
	Error  string `json:"error"`
	Source string `json:"source,omitempty"`
}
