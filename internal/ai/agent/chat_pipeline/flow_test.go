package chat_pipeline

import (
	"SuperBizAgent/internal/ai/events"
	"context"
	"testing"
)

type testEmitter struct{}

func (testEmitter) Emit(ctx context.Context, event events.AgentEvent) {}

func TestWithChatToolEmitter_IsRequestScoped(t *testing.T) {
	base := context.Background()
	ctxA := WithChatToolEmitter(base, testEmitter{}, "trace-a")
	ctxB := WithChatToolEmitter(base, testEmitter{}, "trace-b")

	_, traceA, ok := chatToolEmitterFromContext(ctxA)
	if !ok {
		t.Fatal("expected emitter in ctxA")
	}
	if traceA != "trace-a" {
		t.Fatalf("expected trace-a, got %q", traceA)
	}

	_, traceB, ok := chatToolEmitterFromContext(ctxB)
	if !ok {
		t.Fatal("expected emitter in ctxB")
	}
	if traceB != "trace-b" {
		t.Fatalf("expected trace-b, got %q", traceB)
	}

	if _, _, ok := chatToolEmitterFromContext(base); ok {
		t.Fatal("expected base context to stay unchanged")
	}
}
