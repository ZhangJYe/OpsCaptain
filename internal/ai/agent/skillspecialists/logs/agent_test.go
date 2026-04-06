package logs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	inputs []string
}

func (f *fakeLogTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: f.name,
		Desc: f.desc,
	}, nil
}

func (f *fakeLogTool) InvokableRun(_ context.Context, input string, opts ...toolapi.Option) (string, error) {
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return "", f.err
	}
	return f.output, nil
}

func (f *fakeLogTool) LastPayload(t *testing.T) map[string]any {
	t.Helper()
	if len(f.inputs) == 0 {
		t.Fatal("expected at least one tool invocation")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(f.inputs[len(f.inputs)-1]), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func TestLogAgentUsesEvidenceSkillForErrorQuery(t *testing.T) {
	oldDiscover := discoverLogTools
	defer func() {
		discoverLogTools = oldDiscover
	}()

	tool := &fakeLogTool{
		name:   "query_logs",
		desc:   "query service logs",
		output: `[{"timestamp":"2026-04-04T10:00:00Z","level":"ERROR","service":"payment","message":"db timeout"},{"timestamp":"2026-04-04T10:00:02Z","level":"WARN","service":"payment","message":"retry started"}]`,
	}
	discoverLogTools = func() ([]toolapi.BaseTool, error) {
		return []toolapi.BaseTool{
			tool,
		}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "排查 payment 服务报错日志", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
	if result.Metadata["skill_name"] != "logs_payment_timeout_trace" {
		t.Fatalf("expected logs_payment_timeout_trace, got %#v", result.Metadata)
	}
	if got := tool.LastPayload(t)["query"].(string); !strings.Contains(got, "gateway timeout") {
		t.Fatalf("expected payment timeout focus in query, got %q", got)
	}
}

func TestLogAgentFallsBackToRawReviewSkill(t *testing.T) {
	oldDiscover := discoverLogTools
	defer func() {
		discoverLogTools = oldDiscover
	}()

	discoverLogTools = func() ([]toolapi.BaseTool, error) {
		return []toolapi.BaseTool{
			&fakeLogTool{
				name: "query_logs",
				desc: "query service logs",
				err:  errors.New("mcp timeout"),
			},
		}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "查看 payment 服务日志", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
	if result.Metadata["skill_name"] != "logs_payment_timeout_trace" {
		t.Fatalf("expected logs_payment_timeout_trace, got %#v", result.Metadata)
	}
}

func TestLogAgentUsesGenericEvidenceSkill(t *testing.T) {
	oldDiscover := discoverLogTools
	defer func() {
		discoverLogTools = oldDiscover
	}()

	tool := &fakeLogTool{
		name:   "query_logs",
		desc:   "query service logs",
		output: `[{"timestamp":"2026-04-04T10:00:00Z","level":"ERROR","service":"catalog","message":"panic in indexer"}]`,
	}
	discoverLogTools = func() ([]toolapi.BaseTool, error) {
		return []toolapi.BaseTool{tool}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "Investigate catalog service panic logs", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["skill_name"] != "logs_evidence_extract" {
		t.Fatalf("expected logs_evidence_extract, got %#v", result.Metadata)
	}
	if got := tool.LastPayload(t)["query"].(string); !strings.Contains(got, "stack trace signals") {
		t.Fatalf("expected generic evidence focus in query, got %q", got)
	}
}

func TestLogAgentFallsBackToRawReviewSkillWithoutSpecificKeywords(t *testing.T) {
	oldDiscover := discoverLogTools
	defer func() {
		discoverLogTools = oldDiscover
	}()

	discoverLogTools = func() ([]toolapi.BaseTool, error) {
		return []toolapi.BaseTool{
			&fakeLogTool{
				name: "query_logs",
				desc: "query service logs",
				err:  errors.New("mcp timeout"),
			},
		}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "Review inventory service logs", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["skill_name"] != "logs_raw_review" {
		t.Fatalf("expected logs_raw_review, got %#v", result.Metadata)
	}
}

func TestLogAgentUsesPaymentTimeoutSkill(t *testing.T) {
	oldDiscover := discoverLogTools
	defer func() {
		discoverLogTools = oldDiscover
	}()

	tool := &fakeLogTool{
		name:   "query_logs",
		desc:   "query service logs",
		output: `[{"timestamp":"2026-04-04T10:00:00Z","level":"ERROR","service":"payment","message":"checkout timeout"}]`,
	}
	discoverLogTools = func() ([]toolapi.BaseTool, error) {
		return []toolapi.BaseTool{tool}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "Investigate payment timeout on checkout", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["skill_name"] != "logs_payment_timeout_trace" {
		t.Fatalf("expected logs_payment_timeout_trace, got %#v", result.Metadata)
	}
	payload := tool.LastPayload(t)
	if payload["skill_mode"] != "payment_timeout_trace" {
		t.Fatalf("expected payment timeout mode, got %#v", payload)
	}
	if !strings.Contains(payload["query"].(string), "gateway timeout") {
		t.Fatalf("expected payment timeout focus in query, got %#v", payload)
	}
}

func TestLogAgentUsesAuthFailureSkill(t *testing.T) {
	oldDiscover := discoverLogTools
	defer func() {
		discoverLogTools = oldDiscover
	}()

	tool := &fakeLogTool{
		name:   "query_logs",
		desc:   "query service logs",
		output: `[{"timestamp":"2026-04-04T10:00:00Z","level":"WARN","service":"gateway","message":"jwt token expired"}]`,
	}
	discoverLogTools = func() ([]toolapi.BaseTool, error) {
		return []toolapi.BaseTool{tool}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "Why are users seeing unauthorized login errors?", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["skill_name"] != "logs_auth_failure_trace" {
		t.Fatalf("expected logs_auth_failure_trace, got %#v", result.Metadata)
	}
	payload := tool.LastPayload(t)
	if payload["skill_mode"] != "auth_failure_trace" {
		t.Fatalf("expected auth failure mode, got %#v", payload)
	}
	if !strings.Contains(payload["query"].(string), "auth middleware") {
		t.Fatalf("expected auth failure focus in query, got %#v", payload)
	}
}
