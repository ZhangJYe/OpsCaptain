package events

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// mockTool 模拟工具
type mockTool struct {
	name    string
	result  string
	err     error
	called  bool
	lastArg string
}

func (m *mockTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

func (m *mockTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	m.called = true
	m.lastArg = args
	return m.result, m.err
}

func TestToolWrapper_BasicExecution(t *testing.T) {
	mock := &mockTool{name: "query_metrics", result: "cpu: 80%"}
	emitter := &mockEmitter{}
	wrapper := WrapTool(mock, emitter, "trace-1", nil, nil)

	result, err := wrapper.InvokableRun(context.Background(), `{"service":"payment"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "cpu: 80%" {
		t.Fatalf("expected 'cpu: 80%%', got %q", result)
	}
	if !mock.called {
		t.Fatal("expected tool to be called")
	}

	// 验证事件
	events := emitter.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != EventToolCallStart {
		t.Fatalf("expected tool_call_start, got %q", events[0].Type)
	}
	if events[1].Type != EventToolCallEnd {
		t.Fatalf("expected tool_call_end, got %q", events[1].Type)
	}
	if events[1].Payload["success"] != true {
		t.Fatalf("expected success true, got %v", events[1].Payload["success"])
	}
}

func TestToolWrapper_BeforeToolCall_Block(t *testing.T) {
	mock := &mockTool{name: "dangerous_tool", result: "should not reach"}
	emitter := &mockEmitter{}

	blockBefore := func(ctx context.Context, toolName string, args string) (string, error) {
		return "", errors.New("permission denied")
	}

	wrapper := WrapTool(mock, emitter, "trace-block", blockBefore, nil)
	_, err := wrapper.InvokableRun(context.Background(), `{"action":"delete"}`)

	if err == nil {
		t.Fatal("expected error from beforeToolCall block")
	}
	if mock.called {
		t.Fatal("expected tool NOT to be called when blocked")
	}

	// 验证事件：只有 tool_call_start 和 tool_call_end（带 error）
	events := emitter.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].Payload["success"] != false {
		t.Fatalf("expected success false, got %v", events[1].Payload["success"])
	}
}

func TestToolWrapper_BeforeToolCall_ModifyArgs(t *testing.T) {
	mock := &mockTool{name: "query_logs", result: "found logs"}
	emitter := &mockEmitter{}

	modifyBefore := func(ctx context.Context, toolName string, args string) (string, error) {
		return `{"service":"payment","time_range":"1h"}`, nil
	}

	wrapper := WrapTool(mock, emitter, "trace-mod", modifyBefore, nil)
	result, err := wrapper.InvokableRun(context.Background(), `{"service":"payment"}`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "found logs" {
		t.Fatalf("expected 'found logs', got %q", result)
	}
	if mock.lastArg != `{"service":"payment","time_range":"1h"}` {
		t.Fatalf("expected modified args, got %q", mock.lastArg)
	}
}

func TestToolWrapper_AfterToolCall_ModifyResult(t *testing.T) {
	mock := &mockTool{name: "query_metrics", result: "very long result that should be truncated by the after hook"}
	emitter := &mockEmitter{}

	truncateAfter := func(ctx context.Context, toolName string, args string, result string, err error) (string, error) {
		if len(result) > 20 {
			return result[:20] + "...", nil
		}
		return result, nil
	}

	wrapper := WrapTool(mock, emitter, "trace-after", nil, truncateAfter)
	result, err := wrapper.InvokableRun(context.Background(), `{}`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) > 24 { // 20 + "..."
		t.Fatalf("expected truncated result, got %q", result)
	}
}

func TestToolWrapper_ToolExecutionError(t *testing.T) {
	mock := &mockTool{name: "failing_tool", err: errors.New("connection timeout")}
	emitter := &mockEmitter{}

	wrapper := WrapTool(mock, emitter, "trace-err", nil, nil)
	_, err := wrapper.InvokableRun(context.Background(), `{}`)

	if err == nil {
		t.Fatal("expected error from tool execution")
	}

	// 验证事件
	events := emitter.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].Payload["success"] != false {
		t.Fatalf("expected success false, got %v", events[1].Payload["success"])
	}
	if events[1].Payload["error"] != "connection timeout" {
		t.Fatalf("expected 'connection timeout' error, got %v", events[1].Payload["error"])
	}
}

func TestWrapTools_Batch(t *testing.T) {
	tools := []tool.BaseTool{
		&mockTool{name: "tool_a"},
		&mockTool{name: "tool_b"},
		&mockTool{name: "tool_c"},
	}

	emitter := &mockEmitter{}
	wrapped := WrapTools(tools, emitter, "trace-batch", nil, nil)

	if len(wrapped) != 3 {
		t.Fatalf("expected 3 wrapped tools, got %d", len(wrapped))
	}

	// 验证每个工具都能正常执行
	for i, w := range wrapped {
		_, err := w.InvokableRun(context.Background(), "{}")
		if err != nil {
			t.Fatalf("tool %d failed: %v", i, err)
		}
	}

	// 3 个工具 × 2 个事件 = 6 个事件
	events := emitter.Events()
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d", len(events))
	}
}

func TestAuditBeforeToolCall(t *testing.T) {
	audit := AuditBeforeToolCall()
	args, err := audit(context.Background(), "test_tool", `{"key":"value"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args != `{"key":"value"}` {
		t.Fatalf("expected args passthrough, got %q", args)
	}
}

func TestSummaryAfterToolCall(t *testing.T) {
	summary := SummaryAfterToolCall(10)

	// 短结果不截断
	short, err := summary(context.Background(), "t", "{}", "short", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if short != "short" {
		t.Fatalf("expected 'short', got %q", short)
	}

	// 长结果截断
	long := "this is a very long result that should be truncated"
	truncated, err := summary(context.Background(), "t", "{}", long, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(truncated) > 15 { // 10 + "...(truncated)"
		t.Fatalf("expected truncated, got %q", truncated)
	}

	// 错误时不做处理
	_, err = summary(context.Background(), "t", "{}", "", errors.New("fail"))
	if err == nil {
		t.Fatal("expected error passthrough")
	}
}
