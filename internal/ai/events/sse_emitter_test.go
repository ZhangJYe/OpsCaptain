package events

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// mockSSEClient 记录所有发送的 SSE 事件
type mockSSEClient struct {
	mu     sync.Mutex
	events []mockSSEEvent
}

type mockSSEEvent struct {
	EventType string
	Data      string
}

func (m *mockSSEClient) SendToClient(eventType, data string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, mockSSEEvent{EventType: eventType, Data: data})
	return true
}

func (m *mockSSEClient) Events() []mockSSEEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockSSEEvent, len(m.events))
	copy(result, m.events)
	return result
}

func TestSSEEmitter_Emit(t *testing.T) {
	client := &mockSSEClient{}
	emitter := NewSSEEmitter(client, "trace-sse")

	event := NewEvent(EventToolCallStart, "", "query_metrics", map[string]any{
		"tool_name": "query_metrics",
	})

	emitter.Emit(context.Background(), event)

	events := client.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 SSE event, got %d", len(events))
	}

	if events[0].EventType != "agent_event" {
		t.Fatalf("expected event type agent_event, got %q", events[0].EventType)
	}

	// 验证 JSON 可解析
	var parsed AgentEvent
	if err := json.Unmarshal([]byte(events[0].Data), &parsed); err != nil {
		t.Fatalf("failed to unmarshal SSE data: %v", err)
	}

	if parsed.TraceID != "trace-sse" {
		t.Fatalf("expected trace_id trace-sse, got %q", parsed.TraceID)
	}
	if parsed.Type != EventToolCallStart {
		t.Fatalf("expected type tool_call_start, got %q", parsed.Type)
	}
}

func TestSSEEmitter_NilClient(t *testing.T) {
	emitter := NewSSEEmitter(nil, "trace-nil")

	// 不应该 panic
	event := NewEvent(EventToolCallStart, "", "test", nil)
	emitter.Emit(context.Background(), event)
}
