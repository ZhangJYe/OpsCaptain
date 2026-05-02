package events

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
)

// Emitter 事件发射器接口
type Emitter interface {
	Emit(ctx context.Context, event AgentEvent)
}

// CallbackEmitter 将 Eino callback 转换为 AgentEvent
type CallbackEmitter struct {
	emitter           Emitter
	traceID           string
	includeToolEvents bool

	mu     sync.Mutex
	timers map[string]time.Time // key: component:name
}

// NewCallbackEmitter 创建 callback emitter
func NewCallbackEmitter(emitter Emitter, traceID string) *CallbackEmitter {
	return &CallbackEmitter{
		emitter:           emitter,
		traceID:           traceID,
		includeToolEvents: true,
		timers:            make(map[string]time.Time),
	}
}

// NewModelCallbackEmitter 创建只上报模型事件的 callback emitter。
func NewModelCallbackEmitter(emitter Emitter, traceID string) *CallbackEmitter {
	callbackEmitter := NewCallbackEmitter(emitter, traceID)
	callbackEmitter.includeToolEvents = false
	return callbackEmitter
}

// Handler 返回 Eino callbacks.Handler，用于 compose.WithCallbacks
func (c *CallbackEmitter) Handler() callbacks.Handler {
	builder := callbacks.NewHandlerBuilder()

	builder.OnStartFn(c.onStart)
	builder.OnEndFn(c.onEnd)
	builder.OnErrorFn(c.onError)

	return builder.Build()
}

func (c *CallbackEmitter) timerKey(info *callbacks.RunInfo) string {
	return string(info.Component) + ":" + info.Name
}

func (c *CallbackEmitter) onStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	key := c.timerKey(info)

	if c.shouldTrackTimer(info) {
		c.mu.Lock()
		c.timers[key] = time.Now()
		c.mu.Unlock()
	}

	switch info.Component {
	case components.ComponentOfChatModel:
		c.emitter.Emit(ctx, NewEvent(EventModelStart, c.traceID, info.Name, map[string]any{
			"model_name": info.Name,
		}))
	case components.ComponentOfTool:
		if !c.includeToolEvents {
			return ctx
		}
		c.emitter.Emit(ctx, NewEvent(EventToolCallStart, c.traceID, info.Name, map[string]any{
			"tool_name": info.Name,
		}))
	}

	return ctx
}

func (c *CallbackEmitter) onEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	key := c.timerKey(info)

	c.mu.Lock()
	start, ok := c.timers[key]
	if ok {
		delete(c.timers, key)
	}
	c.mu.Unlock()

	duration := time.Duration(0)
	if ok {
		duration = time.Since(start)
	}

	switch info.Component {
	case components.ComponentOfChatModel:
		c.handleModelEnd(ctx, info, output, duration)
	case components.ComponentOfTool:
		if !c.includeToolEvents {
			return ctx
		}
		c.handleToolEnd(ctx, info, output, duration)
	}

	return ctx
}

func (c *CallbackEmitter) onError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	key := c.timerKey(info)

	c.mu.Lock()
	start, ok := c.timers[key]
	if ok {
		delete(c.timers, key)
	}
	c.mu.Unlock()

	duration := time.Duration(0)
	if ok {
		duration = time.Since(start)
	}

	switch info.Component {
	case components.ComponentOfChatModel:
		c.emitter.Emit(ctx, NewEvent(EventModelEnd, c.traceID, info.Name, map[string]any{
			"model_name":  info.Name,
			"duration_ms": duration.Milliseconds(),
			"success":     false,
			"error":       err.Error(),
		}))
	case components.ComponentOfTool:
		if !c.includeToolEvents {
			return ctx
		}
		c.emitter.Emit(ctx, NewEvent(EventToolCallEnd, c.traceID, info.Name, map[string]any{
			"tool_name":   info.Name,
			"duration_ms": duration.Milliseconds(),
			"success":     false,
			"error":       err.Error(),
		}))
	}

	return ctx
}

func (c *CallbackEmitter) shouldTrackTimer(info *callbacks.RunInfo) bool {
	return info.Component == components.ComponentOfChatModel ||
		(info.Component == components.ComponentOfTool && c.includeToolEvents)
}

func (c *CallbackEmitter) handleModelEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput, duration time.Duration) {
	convOutput := model.ConvCallbackOutput(output)

	payload := map[string]any{
		"model_name":  info.Name,
		"duration_ms": duration.Milliseconds(),
		"success":     true,
	}

	if convOutput != nil && convOutput.TokenUsage != nil {
		payload["prompt_tokens"] = convOutput.TokenUsage.PromptTokens
		payload["completion_tokens"] = convOutput.TokenUsage.CompletionTokens
		payload["total_tokens"] = convOutput.TokenUsage.TotalTokens
	}

	c.emitter.Emit(ctx, NewEvent(EventModelEnd, c.traceID, info.Name, payload))
}

func (c *CallbackEmitter) handleToolEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput, duration time.Duration) {
	convOutput := tool.ConvCallbackOutput(output)

	payload := map[string]any{
		"tool_name":   info.Name,
		"duration_ms": duration.Milliseconds(),
		"success":     true,
	}

	if convOutput != nil {
		summary := truncateSummary(convOutput.Response, 200)
		if summary != "" {
			payload["summary"] = summary
		}
	}

	c.emitter.Emit(ctx, NewEvent(EventToolCallEnd, c.traceID, info.Name, payload))
}

// truncateSummary 截断摘要，保留前 maxLen 字符
func truncateSummary(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
