package evalharness

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const PlanPayloadSchema = "plan-eval/v1"

type PlanAdapter struct{}

type PlanPayload struct {
	Diagnostic        bool        `json:"diagnostic,omitempty"`
	RequiresEvidence  bool        `json:"requires_evidence,omitempty"`
	Status            SuiteStatus `json:"status"`
	Summary           string      `json:"summary"`
	ExpectedKeywords  []string    `json:"expected_keywords"`
	Steps             int         `json:"steps"`
	SuccessfulSteps   int         `json:"successful_steps"`
	Replans           int         `json:"replans"`
	SuccessfulReplans int         `json:"successful_replans"`
	LLMCalls          int         `json:"llm_calls"`
	ToolCalls         int         `json:"tool_calls"`
	RAGCalls          int         `json:"rag_calls"`
	TraceComplete     bool        `json:"trace_complete"`
	EvidenceIDs       []string    `json:"evidence_ids"`
	FailurePhase      string      `json:"failure_phase,omitempty"`
	Reason            string      `json:"reason,omitempty"`
}

type planMetrics struct {
	Cases             int     `json:"cases"`
	CompletionRate    float64 `json:"completion_rate"`
	StepSuccessRate   float64 `json:"step_success_rate"`
	ReplanSuccessRate float64 `json:"replan_success_rate"`
}

func NewPlanAdapter() *PlanAdapter           { return &PlanAdapter{} }
func (a *PlanAdapter) Name() SuiteName       { return SuitePlan }
func (a *PlanAdapter) PayloadSchema() string { return PlanPayloadSchema }
func (a *PlanAdapter) Validate(_ SuiteConfig, _ DatasetRole, profile Profile) error {
	return RejectLiveProfile(profile)
}
func (a *PlanAdapter) RunCase(_ context.Context, evalCase CaseEnvelope) CaseResult {
	start := time.Now()
	var payload PlanPayload
	if err := json.Unmarshal(evalCase.Payload, &payload); err != nil {
		return failedCase(evalCase.ID, "plan", err)
	}
	status := payload.Status
	if status == "" {
		status = StatusSucceeded
	}
	if !payload.Diagnostic {
		payload.Diagnostic = true
	}
	if !payload.RequiresEvidence {
		payload.RequiresEvidence = true
	}
	matched := true
	normalized := strings.ToLower(payload.Summary)
	for _, keyword := range payload.ExpectedKeywords {
		if !strings.Contains(normalized, strings.ToLower(keyword)) {
			matched = false
			break
		}
	}
	return CaseResult{
		CaseID: evalCase.ID, Status: status, Matched: matched, Latency: time.Since(start),
		Usage:         Usage{LLMCalls: payload.LLMCalls, ToolCalls: payload.ToolCalls, RAGCalls: payload.RAGCalls},
		TraceComplete: payload.TraceComplete, EvidenceCount: len(payload.EvidenceIDs), EvidenceIDs: payload.EvidenceIDs,
		FailurePhase: payload.FailurePhase, Reason: payload.Reason, Domain: MarshalDomain(payload),
	}
}
func (a *PlanAdapter) Aggregate(results []CaseResult) (string, json.RawMessage, []GateResult, error) {
	metrics := planMetrics{Cases: len(results)}
	steps, successfulSteps, replans, successfulReplans := 0, 0, 0, 0
	for _, result := range results {
		if result.Status == StatusSucceeded && result.Matched {
			metrics.CompletionRate++
		}
		var payload PlanPayload
		if len(result.Domain) > 0 {
			if err := json.Unmarshal(result.Domain, &payload); err != nil {
				return "", nil, nil, err
			}
			steps += payload.Steps
			successfulSteps += payload.SuccessfulSteps
			replans += payload.Replans
			successfulReplans += payload.SuccessfulReplans
		}
	}
	if len(results) > 0 {
		metrics.CompletionRate /= float64(len(results))
	}
	if steps > 0 {
		metrics.StepSuccessRate = float64(successfulSteps) / float64(steps)
	}
	if replans > 0 {
		metrics.ReplanSuccessRate = float64(successfulReplans) / float64(replans)
	} else {
		metrics.ReplanSuccessRate = 1
	}
	return "plan-metrics/v1", MarshalDomain(metrics), nil, nil
}

type PlanGoSComparison struct {
	CaseID string                        `json:"case_id"`
	Common map[SuiteName]CaseResult      `json:"common"`
	Domain map[SuiteName]json.RawMessage `json:"domain"`
}

func ComparePlanGoSCases(plan, gos []CaseResult) []PlanGoSComparison {
	byCase := make(map[string]*PlanGoSComparison)
	for _, group := range []struct {
		name  SuiteName
		cases []CaseResult
	}{{SuitePlan, plan}, {SuiteGoS, gos}} {
		for _, result := range group.cases {
			comparison := byCase[result.CaseID]
			if comparison == nil {
				comparison = &PlanGoSComparison{CaseID: result.CaseID, Common: make(map[SuiteName]CaseResult), Domain: make(map[SuiteName]json.RawMessage)}
				byCase[result.CaseID] = comparison
			}
			domain := append(json.RawMessage(nil), result.Domain...)
			common := result
			common.Domain = nil
			comparison.Common[group.name] = common
			comparison.Domain[group.name] = domain
		}
	}
	comparisons := make([]PlanGoSComparison, 0, len(byCase))
	for _, comparison := range byCase {
		comparisons = append(comparisons, *comparison)
	}
	return comparisons
}
