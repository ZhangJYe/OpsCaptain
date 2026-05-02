package events

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
)

// mockEmitter 记录所有发射的事件
type mockEmitter struct {
	mu     sync.Mutex
	events []AgentEvent
}

func (m *mockEmitter) Emit(ctx context.Context, event AgentEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockEmitter) Events() []AgentEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]AgentEvent, len(m.events))
	copy(result, m.events)
	return result
}

func TestAgentEvent_NewEvent(t *testing.T) {
	payload := map[string]any{"tool_name": "query_metrics"}
	event := NewEvent(EventToolCallStart, "trace-123", "query_metrics", payload)

	if event.Type != EventToolCallStart {
		t.Fatalf("expected type %q, got %q", EventToolCallStart, event.Type)
	}
	if event.TraceID != "trace-123" {
		t.Fatalf("expected trace_id %q, got %q", "trace-123", event.TraceID)
	}
	if event.Name != "query_metrics" {
		t.Fatalf("expected name %q, got %q", "query_metrics", event.Name)
	}
	if event.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestCallbackEmitter_ToolCallEvents(t *testing.T) {
	mock := &mockEmitter{}
	emitter := NewCallbackEmitter(mock, "trace-abc")
	handler := emitter.Handler()

	ctx := context.Background()
	info := &callbacks.RunInfo{
		Component: components.ComponentOfTool,
		Name:      "query_metrics",
		Type:      "Tool",
	}

	// OnStart
	handler.OnStart(ctx, info, `{"args": "test"}`)

	// 模拟工具执行时间
	time.Sleep(10 * time.Millisecond)

	// OnEnd
	handler.OnEnd(ctx, info, `{"result": "ok"}`)

	events := mock.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// 验证 tool_call_start
	startEvent := events[0]
	if startEvent.Type != EventToolCallStart {
		t.Fatalf("expected tool_call_start, got %q", startEvent.Type)
	}
	if startEvent.TraceID != "trace-abc" {
		t.Fatalf("expected trace_id trace-abc, got %q", startEvent.TraceID)
	}
	if startEvent.Payload["tool_name"] != "query_metrics" {
		t.Fatalf("expected tool_name query_metrics, got %v", startEvent.Payload["tool_name"])
	}

	// 验证 tool_call_end
	endEvent := events[1]
	if endEvent.Type != EventToolCallEnd {
		t.Fatalf("expected tool_call_end, got %q", endEvent.Type)
	}
	if endEvent.Payload["success"] != true {
		t.Fatalf("expected success true, got %v", endEvent.Payload["success"])
	}
	durationMs, ok := endEvent.Payload["duration_ms"].(int64)
	if !ok || durationMs < 0 {
		t.Fatalf("expected non-negative duration_ms, got %v", endEvent.Payload["duration_ms"])
	}
}

func TestCallbackEmitter_ModelCallEvents(t *testing.T) {
	mock := &mockEmitter{}
	emitter := NewCallbackEmitter(mock, "trace-def")
	handler := emitter.Handler()

	ctx := context.Background()
	info := &callbacks.RunInfo{
		Component: components.ComponentOfChatModel,
		Name:      "deepseek-v3",
		Type:      "ChatModel",
	}

	handler.OnStart(ctx, info, nil)
	time.Sleep(10 * time.Millisecond)
	handler.OnEnd(ctx, info, nil)

	events := mock.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].Type != EventModelStart {
		t.Fatalf("expected model_start, got %q", events[0].Type)
	}
	if events[1].Type != EventModelEnd {
		t.Fatalf("expected model_end, got %q", events[1].Type)
	}
	if events[1].Payload["success"] != true {
		t.Fatalf("expected success true, got %v", events[1].Payload["success"])
	}
}

func TestModelCallbackEmitter_SuppressesToolEvents(t *testing.T) {
	mock := &mockEmitter{}
	emitter := NewModelCallbackEmitter(mock, "trace-model-only")
	handler := emitter.Handler()

	ctx := context.Background()
	toolInfo := &callbacks.RunInfo{
		Component: components.ComponentOfTool,
		Name:      "query_metrics",
		Type:      "Tool",
	}
	modelInfo := &callbacks.RunInfo{
		Component: components.ComponentOfChatModel,
		Name:      "deepseek-v3",
		Type:      "ChatModel",
	}

	handler.OnStart(ctx, toolInfo, nil)
	handler.OnEnd(ctx, toolInfo, `{"result":"ok"}`)
	handler.OnStart(ctx, modelInfo, nil)
	handler.OnEnd(ctx, modelInfo, nil)

	events := mock.Events()
	if len(events) != 2 {
		t.Fatalf("expected only 2 model events, got %d", len(events))
	}
	if events[0].Type != EventModelStart {
		t.Fatalf("expected model_start, got %q", events[0].Type)
	}
	if events[1].Type != EventModelEnd {
		t.Fatalf("expected model_end, got %q", events[1].Type)
	}
}

func TestCallbackEmitter_ErrorEvent(t *testing.T) {
	mock := &mockEmitter{}
	emitter := NewCallbackEmitter(mock, "trace-err")
	handler := emitter.Handler()

	ctx := context.Background()
	info := &callbacks.RunInfo{
		Component: components.ComponentOfTool,
		Name:      "failing_tool",
		Type:      "Tool",
	}

	handler.OnStart(ctx, info, nil)
	handler.OnError(ctx, info, context.DeadlineExceeded)

	events := mock.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	endEvent := events[1]
	if endEvent.Type != EventToolCallEnd {
		t.Fatalf("expected tool_call_end, got %q", endEvent.Type)
	}
	if endEvent.Payload["success"] != false {
		t.Fatalf("expected success false, got %v", endEvent.Payload["success"])
	}
	if endEvent.Payload["error"] != context.DeadlineExceeded.Error() {
		t.Fatalf("expected deadline exceeded error, got %v", endEvent.Payload["error"])
	}
}

func TestCallbackEmitter_IgnoresOtherComponents(t *testing.T) {
	mock := &mockEmitter{}
	emitter := NewCallbackEmitter(mock, "trace-ign")
	handler := emitter.Handler()

	ctx := context.Background()
	info := &callbacks.RunInfo{
		Component: components.ComponentOfRetriever,
		Name:      "milvus",
		Type:      "Retriever",
	}

	handler.OnStart(ctx, info, nil)
	handler.OnEnd(ctx, info, nil)

	events := mock.Events()
	if len(events) != 0 {
		t.Fatalf("expected 0 events for retriever, got %d", len(events))
	}
}

func TestCallbackEmitter_EventJSON(t *testing.T) {
	mock := &mockEmitter{}
	emitter := NewCallbackEmitter(mock, "trace-json")
	handler := emitter.Handler()

	ctx := context.Background()
	info := &callbacks.RunInfo{
		Component: components.ComponentOfTool,
		Name:      "query_logs",
		Type:      "Tool",
	}

	handler.OnStart(ctx, info, nil)
	handler.OnEnd(ctx, info, `{"result": "found 42 logs"}`)

	events := mock.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// 验证可以序列化为 JSON
	data, err := json.Marshal(events[1])
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if parsed["type"] != "tool_call_end" {
		t.Fatalf("expected type tool_call_end, got %v", parsed["type"])
	}
}

func TestTruncateSummary(t *testing.T) {
	short := truncateSummary("hello", 10)
	if short != "hello" {
		t.Fatalf("expected hello, got %q", short)
	}

	long := truncateSummary("this is a very long string that should be truncated", 20)
	if len(long) > 24 { // 20 + "..."
		t.Fatalf("expected truncated string, got %q", long)
	}
	if long[len(long)-3:] != "..." {
		t.Fatalf("expected trailing ..., got %q", long)
	}
}

func TestCallbackEmitter_ConcurrentSafety(t *testing.T) {
	mock := &mockEmitter{}
	emitter := NewCallbackEmitter(mock, "trace-conc")
	handler := emitter.Handler()

	ctx := context.Background()
	var wg sync.WaitGroup

	// 并发发射 100 个事件
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			info := &callbacks.RunInfo{
				Component: components.ComponentOfTool,
				Name:      "tool_" + string(rune('A'+idx%26)),
				Type:      "Tool",
			}
			handler.OnStart(ctx, info, nil)
			handler.OnEnd(ctx, info, nil)
		}(i)
	}

	wg.Wait()

	events := mock.Events()
	if len(events) != 200 {
		t.Fatalf("expected 200 events, got %d", len(events))
	}
}
