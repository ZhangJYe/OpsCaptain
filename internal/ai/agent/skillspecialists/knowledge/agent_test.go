package knowledge

import (
	"context"
	"testing"
	"time"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"

	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeKnowledgeTool struct {
	block  bool
	output string
}

func (f *fakeKnowledgeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "query_internal_docs"}, nil
}

func (f *fakeKnowledgeTool) InvokableRun(ctx context.Context, input string, opts ...toolapi.Option) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return f.output, nil
}

func TestKnowledgeAgentUsesSOPSkillOnProcedureQuery(t *testing.T) {
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
	task := protocol.NewRootTask("session-test", "请查询知识库里的 SOP 文档", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
	if result.Metadata["skill_name"] != "knowledge_sop_lookup" {
		t.Fatalf("expected knowledge_sop_lookup, got %#v", result.Metadata)
	}
}

func TestKnowledgeAgentEmitsSelectedSkillIntoRuntimeTrace(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	defer func() {
		newQueryInternalDocsTool = oldFactory
	}()

	newQueryInternalDocsTool = func() toolapi.InvokableTool {
		return &fakeKnowledgeTool{output: "[]"}
	}

	rt := runtime.New()
	agent := New()
	if err := rt.Register(agent); err != nil {
		t.Fatalf("register: %v", err)
	}

	task := protocol.NewRootTask("session-test", "继续分析支付服务最近的异常", agent.Name())
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.Metadata["skill_name"] != "knowledge_incident_guidance" {
		t.Fatalf("expected knowledge_incident_guidance, got %#v", result.Metadata)
	}

	details := rt.DetailMessages(context.Background(), task.TraceID)
	found := false
	for _, line := range details {
		if line == "[knowledge] selected skill=knowledge_incident_guidance" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected skill selection in detail trace, got %v", details)
	}
}
