package evalharness

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	goseval "SuperBizAgent/internal/ai/agent/gos_engine/eval"
	"SuperBizAgent/internal/ai/protocol"
	rageval "SuperBizAgent/internal/ai/rag/eval"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func TestRAGAdapterMatchesExistingEvaluator(t *testing.T) {
	payload := ragPayload{RelevantIDs: []string{"doc-b"}, RankedIDs: []string{"doc-a", "doc-b"}, Metrics: rageval.QueryMetrics{Stages: rageval.RetrievalStages{Final: []string{"doc-a", "doc-b"}}}}
	result := NewRAGAdapter().RunCase(context.Background(), adapterCase(SuiteRAG, RAGPayloadSchema, payload))
	var domain ragCaseDomain
	if err := json.Unmarshal(result.Domain, &domain); err != nil {
		t.Fatal(err)
	}
	docs := []rageval.RetrievedDoc{{ID: "doc-a"}, {ID: "doc-b"}}
	legacy, _, err := rageval.RunQueryEval(context.Background(), func(context.Context, string) ([]rageval.RetrievedDoc, rageval.QueryMetrics, error) {
		return docs, payload.Metrics, nil
	}, []rageval.EvalCase{{ID: "case-1", Query: "q", RelevantIDs: payload.RelevantIDs}}, []int{1, 3, 5})
	if err != nil {
		t.Fatal(err)
	}
	if domain.Summary.MRR != legacy.MRR || domain.Summary.AvgRecallAtK[5] != legacy.AvgRecallAtK[5] {
		t.Fatalf("adapter metrics drift: %#v vs %#v", domain.Summary, legacy)
	}
	if domain.Result.Query != "" {
		t.Fatal("raw query must not be written to the report payload")
	}
}

func TestGoSAdapterMatchesExistingRunner(t *testing.T) {
	evalCase := goseval.EvalCase{ID: "case-1", Symptom: "redis timeout", GroundTruth: "redis timeout", ExpectedKeywords: []string{"redis"}, ExpectedStatus: "succeeded"}
	taskResult := protocol.TaskResult{Status: protocol.ResultStatusSucceeded, Summary: "redis timeout", Evidence: []protocol.EvidenceItem{{SourceType: "metric", SourceID: "m1", Title: "redis", Snippet: "timeout"}}, Metadata: map[string]any{"belief_graph": map[string]any{}, "fsm_history": []any{}, "graph_valid": true}}
	payload := gosPayload{Case: evalCase, TaskResult: taskResult}
	result := NewGoSAdapter().RunCase(context.Background(), adapterCase(SuiteGoS, GoSPayloadSchema, payload))
	legacyMetrics, legacyResults, err := goseval.NewRunner(fixedGoSEngine{result: &taskResult}).RunFromCases(context.Background(), []goseval.EvalCase{evalCase})
	if err != nil {
		t.Fatal(err)
	}
	var domain gosCaseDomain
	if err := json.Unmarshal(result.Domain, &domain); err != nil {
		t.Fatal(err)
	}
	if domain.Result.Matched != legacyResults[0].Matched || domain.Result.EvidenceCount != legacyResults[0].EvidenceCount || legacyMetrics.RootCauseAccuracy != 1 {
		t.Fatalf("gos adapter metrics drift")
	}
	_, aggregateRaw, _, err := NewGoSAdapter().Aggregate([]CaseResult{result})
	if err != nil {
		t.Fatal(err)
	}
	var aggregate goseval.EvalMetrics
	if err := json.Unmarshal(aggregateRaw, &aggregate); err != nil {
		t.Fatal(err)
	}
	if aggregate.RootCauseAccuracy != legacyMetrics.RootCauseAccuracy || aggregate.EvidencePrecision != legacyMetrics.EvidencePrecision || aggregate.EvidenceCoverage != legacyMetrics.EvidenceCoverage || aggregate.GraphValidity != legacyMetrics.GraphValidity || aggregate.BacktrackSuccess != legacyMetrics.BacktrackSuccess || aggregate.DegradationRate != legacyMetrics.DegradationRate {
		t.Fatalf("GoS aggregate metrics drift: %#v vs %#v", aggregate, legacyMetrics)
	}
	if domain.Result.Symptom != "" {
		t.Fatal("raw symptom must not be written to the report payload")
	}
}

func TestPlanAdapterMetricsAndPlanGoSComparison(t *testing.T) {
	payload := PlanPayload{Status: StatusSucceeded, Summary: "Redis timeout", ExpectedKeywords: []string{"redis"}, Steps: 2, SuccessfulSteps: 2, Replans: 1, SuccessfulReplans: 1, TraceComplete: true, EvidenceIDs: []string{"e1"}}
	result := NewPlanAdapter().RunCase(context.Background(), adapterCase(SuitePlan, PlanPayloadSchema, payload))
	_, raw, _, err := NewPlanAdapter().Aggregate([]CaseResult{result})
	if err != nil {
		t.Fatal(err)
	}
	var metrics planMetrics
	if err := json.Unmarshal(raw, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.CompletionRate != 1 || metrics.StepSuccessRate != 1 || metrics.ReplanSuccessRate != 1 {
		t.Fatalf("unexpected plan metrics: %#v", metrics)
	}
	gos := result
	gos.Domain = json.RawMessage(`{"gos":"domain"}`)
	comparisons := ComparePlanGoSCases([]CaseResult{result}, []CaseResult{gos})
	if len(comparisons) != 1 || len(comparisons[0].Common) != 2 || string(comparisons[0].Domain[SuitePlan]) == string(comparisons[0].Domain[SuiteGoS]) {
		t.Fatal("expected common comparison with separate domains")
	}
}

type fakeTool struct {
	output string
	err    error
	wait   time.Duration
}

func (f fakeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "fake", Desc: "fake tool", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{"q": {Type: schema.String}})}, nil
}
func (f fakeTool) InvokableRun(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
	if f.wait > 0 {
		select {
		case <-time.After(f.wait):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return f.output, f.err
}

func TestToolAdapterContracts(t *testing.T) {
	tools := map[string]tool.InvokableTool{"success": fakeTool{output: `{"success":true}`}, "degraded": fakeTool{output: `{"success":false,"degraded":true}`}, "timeout": fakeTool{wait: 50 * time.Millisecond}, "cancelled": fakeTool{wait: 50 * time.Millisecond}, "malformed": fakeTool{output: `{broken`}, "error": fakeTool{err: errors.New("external failure")}}
	adapter := NewToolAdapter(func(name string) (tool.InvokableTool, bool) { value, ok := tools[name]; return value, ok })
	tests := []ToolPayload{{ToolName: "success", Arguments: `{"q":"x"}`, ExpectedOutcome: "success", MaxCalls: 1}, {ToolName: "degraded", Arguments: `{"q":"x"}`, ExpectedOutcome: "degraded", MaxCalls: 1}, {ToolName: "timeout", Arguments: `{"q":"x"}`, ExpectedOutcome: "timeout", TimeoutMS: 1, MaxCalls: 1}, {ToolName: "malformed", Arguments: `{"q":"x"}`, ExpectedOutcome: "malformed", MaxCalls: 1}, {ToolName: "error", Arguments: `{"q":"x"}`, ExpectedOutcome: "error", MaxCalls: 1}, {ToolName: "denied", ExpectedOutcome: "permission_denied", PermissionDenied: true}}
	var results []CaseResult
	for index, payload := range tests {
		evalCase := adapterCase(SuiteTool, ToolPayloadSchema, payload)
		evalCase.ID = string(rune('a' + index))
		results = append(results, adapter.RunCase(context.Background(), evalCase))
	}
	_, metricsRaw, _, err := adapter.Aggregate(results)
	if err != nil {
		t.Fatal(err)
	}
	metrics := DomainMetricMap(metricsRaw)
	if metrics["contract_compliance"].Value != 1 || metrics["observed_degradation_rate"].Value != 1.0/6.0 || results[5].Usage.ToolCalls != 0 {
		t.Fatalf("tool contract failed: %s", metricsRaw)
	}
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := adapter.RunCase(cancelledCtx, adapterCase(SuiteTool, ToolPayloadSchema, ToolPayload{ToolName: "cancelled", Arguments: `{"q":"x"}`, ExpectedOutcome: "cancelled", MaxCalls: 1}))
	if !cancelled.Matched || cancelled.Status != StatusSucceeded {
		t.Fatalf("cancellation contract failed: %#v", cancelled)
	}
}

func TestRAGAdapterPreservesFailurePhase(t *testing.T) {
	result := NewRAGAdapter().RunCase(context.Background(), adapterCase(SuiteRAG, RAGPayloadSchema, ragPayload{RelevantIDs: []string{"doc"}, Error: "retriever unavailable"}))
	if result.Status != StatusFailed || result.FailurePhase != "retrieve" {
		t.Fatalf("unexpected RAG failure: %#v", result)
	}
}

func TestEvidenceAdapterTraceabilityGate(t *testing.T) {
	payload := EvidencePayload{Claims: []EvidenceClaim{{Text: "Redis timeout", CitationIDs: []string{"c1"}, RequiresEvidence: true}}, Citations: []EvidenceCitation{{ID: "c1", Source: "kb.md", TraceID: "trace-1", Text: "Redis timeout"}}, Evidence: []EvidenceLink{{CitationID: "c1", Text: "timeout"}}, ExpectedKeywords: []string{"timeout"}}
	adapter := NewEvidenceAdapter()
	result := adapter.RunCase(context.Background(), adapterCase(SuiteEvidence, EvidencePayloadSchema, payload))
	_, raw, gates, err := adapter.Aggregate([]CaseResult{result})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || len(gates) != 2 || !gates[0].Passed || !gates[1].Passed || DomainMetricMap(raw)["claim_support_rate"].Value != 1 {
		t.Fatalf("unexpected evidence result: %#v", result)
	}
}

func TestEvidenceAdapterRejectsUnsupportedClaim(t *testing.T) {
	payload := EvidencePayload{Claims: []EvidenceClaim{{Text: "unsupported", CitationIDs: []string{"missing"}, RequiresEvidence: true}}}
	result := NewEvidenceAdapter().RunCase(context.Background(), adapterCase(SuiteEvidence, EvidencePayloadSchema, payload))
	if result.Status != StatusFailed || result.Matched {
		t.Fatalf("unsupported claim must fail: %#v", result)
	}
}

func TestReplayAdaptersRejectLiveProfile(t *testing.T) {
	adapters := []Adapter{NewRAGAdapter(), NewPlanAdapter(), NewGoSAdapter(), NewToolAdapter(func(string) (tool.InvokableTool, bool) { return nil, false }), NewEvidenceAdapter()}
	for _, adapter := range adapters {
		if err := adapter.Validate(SuiteConfig{}, DatasetHoldout, ProfileLive); err == nil {
			t.Fatalf("%s adapter must reject unconfigured live execution", adapter.Name())
		}
	}
}

func adapterCase(suite SuiteName, schemaVersion string, payload any) CaseEnvelope {
	raw, _ := json.Marshal(payload)
	return CaseEnvelope{SchemaVersion: CaseSchemaVersion, ID: "case-1", Suite: suite, Input: CaseInput{Query: "q"}, Expectation: json.RawMessage(`{"ok":true}`), PayloadSchemaVersion: schemaVersion, Payload: raw}
}
