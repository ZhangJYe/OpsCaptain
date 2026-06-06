package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/ai/tools"

	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeKnowledgeTool struct {
	block  bool
	output string
	inputs []string
}

func (f *fakeKnowledgeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "query_internal_docs"}, nil
}

func (f *fakeKnowledgeTool) InvokableRun(ctx context.Context, input string, opts ...toolapi.Option) (string, error) {
	f.inputs = append(f.inputs, input)
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return f.output, nil
}

func (f *fakeKnowledgeTool) LastQuery(t *testing.T) string {
	t.Helper()
	if len(f.inputs) == 0 {
		t.Fatal("expected at least one tool invocation")
	}
	var payload tools.QueryInternalDocsInput
	if err := json.Unmarshal([]byte(f.inputs[len(f.inputs)-1]), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload.Query
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

	tool := &fakeKnowledgeTool{output: emptyStructuredKnowledgeOutput(t)}
	newQueryInternalDocsTool = func() toolapi.InvokableTool { return tool }

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
	if got := tool.LastQuery(t); !strings.Contains(got, "troubleshooting guidance") {
		t.Fatalf("expected incident guidance focus in query, got %q", got)
	}
}

func TestKnowledgeAgentUsesReleaseSOPSkillAndRewritesQuery(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	defer func() {
		newQueryInternalDocsTool = oldFactory
	}()

	tool := &fakeKnowledgeTool{output: emptyStructuredKnowledgeOutput(t)}
	newQueryInternalDocsTool = func() toolapi.InvokableTool { return tool }

	agent := New()
	task := protocol.NewRootTask("session-test", "How do we deploy the payment service to production?", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["skill_name"] != "knowledge_release_sop" {
		t.Fatalf("expected knowledge_release_sop, got %#v", result.Metadata)
	}
	if got := tool.LastQuery(t); !strings.Contains(got, "pre-check") || !strings.Contains(got, "rollback") {
		t.Fatalf("expected release SOP focus in query, got %q", got)
	}
}

func TestKnowledgeAgentUsesRollbackSkillAndRewritesQuery(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	defer func() {
		newQueryInternalDocsTool = oldFactory
	}()

	tool := &fakeKnowledgeTool{output: emptyStructuredKnowledgeOutput(t)}
	newQueryInternalDocsTool = func() toolapi.InvokableTool { return tool }

	agent := New()
	task := protocol.NewRootTask("session-test", "Should we rollback the latest release after elevated latency?", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["skill_name"] != "knowledge_rollback_runbook" {
		t.Fatalf("expected knowledge_rollback_runbook, got %#v", result.Metadata)
	}
	if got := tool.LastQuery(t); !strings.Contains(got, "mitigation actions") || !strings.Contains(got, "validation checklist") {
		t.Fatalf("expected rollback focus in query, got %q", got)
	}
}

func TestKnowledgeAgentUsesServiceErrorCodeLookupSkill(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	defer func() {
		newQueryInternalDocsTool = oldFactory
	}()

	tool := &fakeKnowledgeTool{output: emptyStructuredKnowledgeOutput(t)}
	newQueryInternalDocsTool = func() toolapi.InvokableTool { return tool }

	agent := New()
	task := protocol.NewRootTask("session-test", "What does error code 12000000002 mean in the billing service?", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["skill_name"] != "knowledge_service_error_code_lookup" {
		t.Fatalf("expected knowledge_service_error_code_lookup, got %#v", result.Metadata)
	}
	if got := tool.LastQuery(t); !strings.Contains(got, "exact error code meaning") || !strings.Contains(got, "first troubleshooting checks") {
		t.Fatalf("expected error code focus in query, got %q", got)
	}
	codes, ok := result.Metadata["extracted_error_codes"].([]string)
	if !ok || len(codes) != 1 || codes[0] != "12000000002" {
		t.Fatalf("expected extracted error code metadata, got %#v", result.Metadata["extracted_error_codes"])
	}
	if len(result.NextActions) == 0 {
		t.Fatalf("expected next actions for error code lookup, got %#v", result.NextActions)
	}
}

func TestKnowledgeAgentConsumesStructuredKnowledgeOutput(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	defer func() {
		newQueryInternalDocsTool = oldFactory
	}()

	output := structuredKnowledgeOutput(t, rag.KnowledgeSearchOutput{
		Success: true,
		Citations: []rag.Citation{
			{
				ID:      "kb-doc-1",
				Source:  "docs/payment-sop.md",
				Title:   "Payment SOP",
				Score:   0.91,
				Snippet: "fallback snippet",
			},
		},
		Evidence: []rag.Evidence{
			{
				CitationID: "kb-doc-1",
				Text:       "restart payment-service after checking metrics",
			},
		},
	})
	tool := &fakeKnowledgeTool{output: output}
	newQueryInternalDocsTool = func() toolapi.InvokableTool { return tool }

	agent := New()
	task := protocol.NewRootTask("session-test", "请查询知识库里的 SOP 文档", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("expected one evidence item, got %#v", result.Evidence)
	}
	ev := result.Evidence[0]
	if ev.SourceID != "kb-doc-1" || ev.Title != "Payment SOP" || ev.URI != "docs/payment-sop.md" {
		t.Fatalf("unexpected evidence source fields: %#v", ev)
	}
	if !strings.Contains(ev.Snippet, "restart payment-service") {
		t.Fatalf("expected structured evidence text, got %q", ev.Snippet)
	}
	if ev.Score != 0.91 {
		t.Fatalf("expected citation score 0.91, got %f", ev.Score)
	}
	if result.Metadata["document_count"] != 1 {
		t.Fatalf("expected document_count=1, got %#v", result.Metadata)
	}
	if _, ok := result.Metadata["raw_output"]; ok {
		t.Fatalf("raw_output should not be stored in metadata: %#v", result.Metadata)
	}
}

func TestKnowledgeAgentKeepsCitationIDWhenCitationMissing(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	defer func() {
		newQueryInternalDocsTool = oldFactory
	}()

	output := structuredKnowledgeOutput(t, rag.KnowledgeSearchOutput{
		Success: true,
		Evidence: []rag.Evidence{
			{
				CitationID: "kb-doc-missing",
				Text:       "fallback evidence without citation metadata",
			},
		},
	})
	tool := &fakeKnowledgeTool{output: output}
	newQueryInternalDocsTool = func() toolapi.InvokableTool { return tool }

	agent := New()
	task := protocol.NewRootTask("session-test", "请查询知识库里的 SOP 文档", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("expected one evidence item, got %#v", result.Evidence)
	}
	if result.Evidence[0].SourceID != "kb-doc-missing" {
		t.Fatalf("expected citation id fallback, got %#v", result.Evidence[0])
	}
}

func TestKnowledgeAgentDegradesOnStructuredKnowledgeOutput(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	defer func() {
		newQueryInternalDocsTool = oldFactory
	}()

	output := structuredKnowledgeOutput(t, rag.KnowledgeSearchOutput{
		Success:  false,
		Degraded: true,
		Message:  "内部知识库检索暂时不可用",
	})
	tool := &fakeKnowledgeTool{output: output}
	newQueryInternalDocsTool = func() toolapi.InvokableTool { return tool }

	agent := New()
	task := protocol.NewRootTask("session-test", "请查询知识库里的 SOP 文档", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
	if !strings.Contains(result.Summary, "暂时不可用") {
		t.Fatalf("expected degraded summary from tool message, got %q", result.Summary)
	}
	if result.Metadata["tool_degraded"] != true {
		t.Fatalf("expected tool_degraded metadata, got %#v", result.Metadata)
	}
	if _, ok := result.Metadata["raw_output"]; ok {
		t.Fatalf("raw_output should not be stored in metadata: %#v", result.Metadata)
	}
}

func TestKnowledgeAgentKeepsLegacyArrayFallback(t *testing.T) {
	oldFactory := newQueryInternalDocsTool
	defer func() {
		newQueryInternalDocsTool = oldFactory
	}()

	tool := &fakeKnowledgeTool{output: `[{"id":"legacy-doc","content":"legacy content"}]`}
	newQueryInternalDocsTool = func() toolapi.InvokableTool { return tool }

	agent := New()
	task := protocol.NewRootTask("session-test", "请查询知识库里的 SOP 文档", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("expected legacy evidence item, got %#v", result.Evidence)
	}
	if result.Evidence[0].SourceID != "legacy-doc" || result.Evidence[0].Snippet != "legacy content" {
		t.Fatalf("unexpected legacy evidence: %#v", result.Evidence[0])
	}
}

func TestKnowledgeAgentShortenKeepsUTF8(t *testing.T) {
	content := strings.Repeat("支付服务延迟升高", 40)
	snippet := shorten(content, 160)
	if !utf8.ValidString(snippet) {
		t.Fatalf("expected valid utf8 snippet, got %q", snippet)
	}
	if !strings.HasSuffix(snippet, "...") {
		t.Fatal("expected ... suffix")
	}
	if utf8.RuneCountInString(strings.TrimSuffix(snippet, "...")) != 160 {
		t.Fatalf("expected 160 runes before suffix, got %d", utf8.RuneCountInString(strings.TrimSuffix(snippet, "...")))
	}
}

func emptyStructuredKnowledgeOutput(t *testing.T) string {
	return structuredKnowledgeOutput(t, rag.KnowledgeSearchOutput{Success: true})
}

func structuredKnowledgeOutput(t *testing.T, output rag.KnowledgeSearchOutput) string {
	t.Helper()
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal structured knowledge output: %v", err)
	}
	return string(raw)
}
