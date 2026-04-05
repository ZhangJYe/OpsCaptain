package logs

import (
	"context"
	"errors"
	"testing"

	"SuperBizAgent/internal/ai/protocol"

	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeLogTool struct {
	name   string
	desc   string
	output string
	err    error
}

func (f *fakeLogTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: f.name,
		Desc: f.desc,
	}, nil
}

func (f *fakeLogTool) InvokableRun(context.Context, string, ...toolapi.Option) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.output, nil
}

func TestLogAgentReturnsStructuredEvidence(t *testing.T) {
	oldDiscover := discoverLogTools
	defer func() {
		discoverLogTools = oldDiscover
	}()

	discoverLogTools = func() ([]toolapi.BaseTool, error) {
		return []toolapi.BaseTool{
			&fakeLogTool{
				name:   "query_logs",
				desc:   "查询应用日志",
				output: `[{"timestamp":"2026-04-04T10:00:00Z","level":"ERROR","service":"payment","message":"db timeout"},{"timestamp":"2026-04-04T10:00:02Z","level":"WARN","service":"payment","message":"retry started"}]`,
			},
		}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "排查 payment 服务日志", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
	if len(result.Evidence) < 2 {
		t.Fatalf("expected structured evidence, got %#v", result.Evidence)
	}
	if result.Metadata["successful_tool"] != "query_logs" {
		t.Fatalf("expected successful_tool metadata, got %#v", result.Metadata)
	}
}

func TestLogAgentDegradesOnInvocationError(t *testing.T) {
	oldDiscover := discoverLogTools
	defer func() {
		discoverLogTools = oldDiscover
	}()

	discoverLogTools = func() ([]toolapi.BaseTool, error) {
		return []toolapi.BaseTool{
			&fakeLogTool{
				name: "query_logs",
				desc: "查询应用日志",
				err:  errors.New("mcp timeout"),
			},
		}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "排查 payment 服务日志", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
	if result.Summary == "" {
		t.Fatal("expected degraded summary")
	}
}

func TestBuildLogEvidenceFromPlainText(t *testing.T) {
	evidence := buildLogEvidence("query_logs", "ERROR payment db timeout\nWARN payment retry started\n", 2)
	if len(evidence) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(evidence))
	}
	if evidence[0].SourceType != "log" {
		t.Fatalf("unexpected source type: %#v", evidence[0])
	}
}
