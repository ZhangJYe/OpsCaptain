package evalharness

import (
	"context"
	"encoding/json"
	"time"

	goseval "SuperBizAgent/internal/ai/agent/gos_engine/eval"
	"SuperBizAgent/internal/ai/protocol"
)

const GoSPayloadSchema = "gos-eval/v1"

type GoSAdapter struct{}

type gosPayload struct {
	Case       goseval.EvalCase    `json:"case"`
	TaskResult protocol.TaskResult `json:"task_result"`
}

type gosCaseDomain struct {
	Diagnostic       bool               `json:"diagnostic"`
	RequiresEvidence bool               `json:"requires_evidence"`
	Result           goseval.EvalResult `json:"result"`
}
type fixedGoSEngine struct{ result *protocol.TaskResult }

func (e fixedGoSEngine) Run(context.Context, string) *protocol.TaskResult { return e.result }

func NewGoSAdapter() *GoSAdapter            { return &GoSAdapter{} }
func (a *GoSAdapter) Name() SuiteName       { return SuiteGoS }
func (a *GoSAdapter) PayloadSchema() string { return GoSPayloadSchema }
func (a *GoSAdapter) Validate(_ SuiteConfig, _ DatasetRole, profile Profile) error {
	return RejectLiveProfile(profile)
}
func (a *GoSAdapter) RunCase(ctx context.Context, evalCase CaseEnvelope) CaseResult {
	start := time.Now()
	var payload gosPayload
	if err := json.Unmarshal(evalCase.Payload, &payload); err != nil {
		return failedCase(evalCase.ID, "update", err)
	}
	if payload.Case.ID == "" {
		payload.Case.ID = evalCase.ID
	}
	if payload.Case.Symptom == "" {
		payload.Case.Symptom = evalCase.Input.Query
	}
	metrics, results, err := goseval.NewRunner(fixedGoSEngine{result: &payload.TaskResult}).RunFromCases(ctx, []goseval.EvalCase{payload.Case})
	if err != nil || len(results) == 0 {
		if err == nil {
			err = context.Canceled
		}
		return failedCase(evalCase.ID, "update", err)
	}
	result := results[0]
	result.Symptom = ""
	status := StatusSucceeded
	if result.Status == string(protocol.ResultStatusDegraded) {
		status = StatusDegraded
	}
	if result.Status == string(protocol.ResultStatusFailed) {
		status = StatusFailed
	}
	_ = metrics
	evidenceIDs := make([]string, 0, len(payload.TaskResult.Evidence))
	for _, evidence := range payload.TaskResult.Evidence {
		if evidence.SourceID != "" {
			evidenceIDs = append(evidenceIDs, evidence.SourceID)
		}
	}
	return CaseResult{
		CaseID: evalCase.ID, Status: status, Matched: result.Matched && result.ContractMatched,
		Latency: time.Since(start), Usage: Usage{LLMCalls: result.LLMCalls, ToolCalls: result.ToolCalls, RAGCalls: result.RAGCalls},
		TraceComplete: result.TraceComplete, EvidenceCount: result.EvidenceCount, EvidenceIDs: evidenceIDs, FailurePhase: result.FailurePhase,
		Reason: result.DegradationReason, Domain: MarshalDomain(gosCaseDomain{Diagnostic: true, RequiresEvidence: true, Result: result}),
	}
}
func (a *GoSAdapter) Aggregate(results []CaseResult) (string, json.RawMessage, []GateResult, error) {
	metrics := goseval.NewEvalMetrics()
	for _, result := range results {
		var domain gosCaseDomain
		if len(result.Domain) == 0 {
			continue
		}
		if err := json.Unmarshal(result.Domain, &domain); err != nil {
			return "", nil, nil, err
		}
		metrics.AddResult(&domain.Result)
	}
	metrics.Finalize()
	return "gos-metrics/v1", MarshalDomain(metrics), nil, nil
}
