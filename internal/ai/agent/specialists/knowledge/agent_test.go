package knowledge

import (
	"context"
	"testing"
	"time"

	"SuperBizAgent/internal/ai/protocol"

	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeKnowledgeTool struct {
	block bool
}

func (f *fakeKnowledgeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "query_internal_docs"}, nil
}

func (f *fakeKnowledgeTool) InvokableRun(ctx context.Context, input string, opts ...toolapi.Option) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "[]", nil
}

func TestKnowledgeAgentDegradesOnTimeout(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	oldTimeout := knowledgeQueryTimeout
	defer func() {
		newQueryInternalDocsTool = oldFactory
		knowledgeQueryTimeout = oldTimeout
	}()

	newQueryInternalDocsTool = func() toolapi.InvokableTool {
		return &fakeKnowledgeTool{block: true}
	}
	knowledgeQueryTimeout = func(context.Context) time.Duration {
		return 20 * time.Millisecond
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "请查询知识库中的 SOP 文档", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
	if result.Summary != "知识库检索超时，已跳过该步骤。" {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
}
