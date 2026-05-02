package events

import (
	"context"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

// sequenceRecorder 记录事件顺序
type sequenceRecorder struct {
	mu     sync.Mutex
	events []AgentEvent
}

func (r *sequenceRecorder) Emit(ctx context.Context, event AgentEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *sequenceRecorder) Types() []AgentEventType {
	r.mu.Lock()
	defer r.mu.Unlock()
	types := make([]AgentEventType, len(r.events))
	for i, e := range r.events {
		types[i] = e.Type
	}
	return types
}

// TestEventSequence_SingleToolCall 验证单次工具调用的事件顺序
func TestEventSequence_SingleToolCall(t *testing.T) {
	recorder := &sequenceRecorder{}
	wrapper := WrapTool(
		&mockTool{name: "query_metrics", result: "cpu: 80%"},
		recorder, "trace-seq", nil, nil,
	)

	wrapper.InvokableRun(context.Background(), `{}`)

	types := recorder.Types()
	expected := []AgentEventType{EventToolCallStart, EventToolCallEnd}
	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}
	for i, et := range expected {
		if types[i] != et {
			t.Fatalf("event[%d]: expected %s, got %s", i, et, types[i])
		}
	}
}

// TestEventSequence_MultipleToolCalls 验证多次工具调用的事件顺序
func TestEventSequence_MultipleToolCalls(t *testing.T) {
	recorder := &sequenceRecorder{}
	tools := []tool.BaseTool{
		WrapTool(&mockTool{name: "query_metrics", result: "cpu: 80%"}, recorder, "trace-m", nil, nil),
		WrapTool(&mockTool{name: "query_logs", result: "found logs"}, recorder, "trace-m", nil, nil),
		WrapTool(&mockTool{name: "search_docs", result: "found docs"}, recorder, "trace-m", nil, nil),
	}

	for _, tw := range tools {
		if it, ok := tw.(tool.InvokableTool); ok {
			it.InvokableRun(context.Background(), `{}`)
		}
	}

	types := recorder.Types()
	expected := []AgentEventType{
		EventToolCallStart, EventToolCallEnd, // metrics
		EventToolCallStart, EventToolCallEnd, // logs
		EventToolCallStart, EventToolCallEnd, // docs
	}
	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}
	for i, et := range expected {
		if types[i] != et {
			t.Fatalf("event[%d]: expected %s, got %s", i, et, types[i])
		}
	}
}

// TestEventSequence_ToolFailure 验证工具失败时的事件顺序
func TestEventSequence_ToolFailure(t *testing.T) {
	recorder := &sequenceRecorder{}
	wrapper := WrapTool(
		&mockTool{name: "failing_tool", err: errTimeout},
		recorder, "trace-fail", nil, nil,
	)

	wrapper.InvokableRun(context.Background(), `{}`)

	types := recorder.Types()
	expected := []AgentEventType{EventToolCallStart, EventToolCallEnd}
	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}

	// 验证 tool_call_end 的 success=false
	endEvent := recorder.events[1]
	if endEvent.Payload["success"] != false {
		t.Fatalf("expected success false, got %v", endEvent.Payload["success"])
	}
}

// TestEventSequence_AfterHookFailure 验证 after hook 失败时的事件顺序
func TestEventSequence_AfterHookFailure(t *testing.T) {
	recorder := &sequenceRecorder{}
	afterFail := func(ctx context.Context, toolName, args, result string, execErr error) (string, error) {
		return "", errDesensitization
	}
	wrapper := WrapTool(
		&mockTool{name: "query_metrics", result: "sensitive data"},
		recorder, "trace-after-fail", nil, afterFail,
	)

	wrapper.InvokableRun(context.Background(), `{}`)

	types := recorder.Types()
	expected := []AgentEventType{EventToolCallStart, EventToolCallEnd}
	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}

	// 验证 after hook 失败的标记
	endEvent := recorder.events[1]
	if endEvent.Payload["after_error"] != true {
		t.Fatalf("expected after_error true, got %v", endEvent.Payload["after_error"])
	}
	if endEvent.Payload["summary"] != nil {
		t.Fatalf("expected no summary on after hook failure, got %v", endEvent.Payload["summary"])
	}
}

// TestEventSequence_CallbackEmitterAndToolWrapper 验证 callback + tool wrapper 共存时的事件顺序
func TestEventSequence_CallbackEmitterAndToolWrapper(t *testing.T) {
	recorder := &sequenceRecorder{}

	// 模拟 callback emitter 产生的事件
	recorder.Emit(context.Background(), NewEvent(EventModelStart, "trace-combo", "deepseek", nil))
	recorder.Emit(context.Background(), NewEvent(EventModelEnd, "trace-combo", "deepseek", map[string]any{"success": true}))

	// 模拟 tool wrapper 产生的事件
	wrapper := WrapTool(
		&mockTool{name: "query_metrics", result: "ok"},
		recorder, "trace-combo", nil, nil,
	)
	wrapper.InvokableRun(context.Background(), `{}`)

	types := recorder.Types()
	expected := []AgentEventType{
		EventModelStart,    // callback: model start
		EventModelEnd,      // callback: model end
		EventToolCallStart, // wrapper: tool start
		EventToolCallEnd,   // wrapper: tool end
	}
	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}
	for i, et := range expected {
		if types[i] != et {
			t.Fatalf("event[%d]: expected %s, got %s", i, et, types[i])
		}
	}
}
