package chat_pipeline

import (
	"SuperBizAgent/internal/ai/events"
	"context"
	"testing"

	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

type testEmitter struct{}

func (testEmitter) Emit(ctx context.Context, event events.AgentEvent) {}

type namedBaseTool struct {
	name string
}

func (t namedBaseTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name}, nil
}

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

func TestConfigureAutoDiagnosisToolKeepsOnlySafeChatTools(t *testing.T) {
	config := &react.AgentConfig{ToolReturnDirectly: map[string]struct{}{}}
	config.ToolsConfig.Tools = []toolapi.BaseTool{
		namedBaseTool{name: "get_current_time"},
		namedBaseTool{name: "query_internal_docs"},
		namedBaseTool{name: "query_logs"},
	}

	if err := configureAutoDiagnosisTool(context.Background(), config, namedBaseTool{name: diagnoseIncidentToolNameForTest}); err != nil {
		t.Fatalf("configureAutoDiagnosisTool() error = %v", err)
	}

	var names []string
	for _, candidate := range config.ToolsConfig.Tools {
		info, err := candidate.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info error = %v", err)
		}
		names = append(names, info.Name)
	}
	want := []string{"get_current_time", "query_internal_docs", diagnoseIncidentToolNameForTest}
	if len(names) != len(want) {
		t.Fatalf("unexpected tools: %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("unexpected tools: %v", names)
		}
	}
	if _, ok := config.ToolReturnDirectly[diagnoseIncidentToolNameForTest]; !ok {
		t.Fatal("diagnosis tool must return directly")
	}
}

const diagnoseIncidentToolNameForTest = "diagnose_incident"
