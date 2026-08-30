package evalharness

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	rageval "SuperBizAgent/internal/ai/rag/eval"
)

const RAGPayloadSchema = "rag-eval/v1"

type RAGAdapter struct{}

type ragPayload struct {
	RelevantIDs []string             `json:"relevant_ids"`
	RankedIDs   []string             `json:"ranked_ids"`
	Metrics     rageval.QueryMetrics `json:"metrics"`
	Error       string               `json:"error,omitempty"`
}

type ragCaseDomain struct {
	Summary rageval.QuerySummary    `json:"summary"`
	Result  rageval.QueryCaseResult `json:"result"`
}

func NewRAGAdapter() *RAGAdapter            { return &RAGAdapter{} }
func (a *RAGAdapter) Name() SuiteName       { return SuiteRAG }
func (a *RAGAdapter) PayloadSchema() string { return RAGPayloadSchema }
func (a *RAGAdapter) Validate(_ SuiteConfig, _ DatasetRole, profile Profile) error {
	return RejectLiveProfile(profile)
}

func (a *RAGAdapter) RunCase(ctx context.Context, evalCase CaseEnvelope) CaseResult {
	start := time.Now()
	var payload ragPayload
	if err := json.Unmarshal(evalCase.Payload, &payload); err != nil {
		return failedCase(evalCase.ID, "retrieve", err)
	}
	docs := make([]rageval.RetrievedDoc, 0, len(payload.RankedIDs))
	for index, id := range payload.RankedIDs {
		docs = append(docs, rageval.RetrievedDoc{ID: id, Score: float64(len(payload.RankedIDs) - index)})
	}
	summary, results, err := rageval.RunQueryEval(ctx, func(context.Context, string) ([]rageval.RetrievedDoc, rageval.QueryMetrics, error) {
		if payload.Error != "" {
			return nil, payload.Metrics, fmt.Errorf("%s", payload.Error)
		}
		return docs, payload.Metrics, nil
	}, []rageval.EvalCase{{ID: evalCase.ID, Query: evalCase.Input.Query, RelevantIDs: payload.RelevantIDs}}, []int{1, 3, 5})
	if err != nil {
		return CaseResult{CaseID: evalCase.ID, Status: StatusFailed, Latency: time.Since(start), FailurePhase: "retrieve", Reason: err.Error(), Usage: Usage{RAGCalls: 1}}
	}
	domain := ragCaseDomain{Summary: summary}
	if len(results) > 0 {
		domain.Result = results[0]
		domain.Result.Query = ""
	}
	matched := summary.HitRateAtK[5] > 0
	return CaseResult{
		CaseID: evalCase.ID, Status: StatusSucceeded, Matched: matched, Latency: time.Since(start),
		Usage: Usage{RAGCalls: 1}, TraceComplete: true, EvidenceCount: len(domain.Result.HitIDsByK[5]),
		Domain: MarshalDomain(domain),
	}
}

func (a *RAGAdapter) Aggregate(results []CaseResult) (string, json.RawMessage, []GateResult, error) {
	type replay struct {
		caseInput rageval.EvalCase
		result    rageval.QueryCaseResult
	}
	replays := make([]replay, 0, len(results))
	for _, result := range results {
		if len(result.Domain) == 0 {
			continue
		}
		var domain ragCaseDomain
		if err := json.Unmarshal(result.Domain, &domain); err != nil {
			return "", nil, nil, err
		}
		replays = append(replays, replay{
			caseInput: rageval.EvalCase{ID: domain.Result.CaseID, Query: domain.Result.Query, RelevantIDs: append([]string(nil), domain.Result.RelevantIDs...)},
			result:    domain.Result,
		})
	}
	if len(replays) == 0 {
		return "rag-metrics/v1", MarshalDomain(rageval.QuerySummary{}), nil, nil
	}
	index := 0
	cases := make([]rageval.EvalCase, 0, len(replays))
	for _, item := range replays {
		cases = append(cases, item.caseInput)
	}
	summary, _, err := rageval.RunQueryEval(context.Background(), func(context.Context, string) ([]rageval.RetrievedDoc, rageval.QueryMetrics, error) {
		item := replays[index]
		index++
		docs := make([]rageval.RetrievedDoc, 0, len(item.result.RankedIDs))
		for rank, id := range item.result.RankedIDs {
			docs = append(docs, rageval.RetrievedDoc{ID: id, Score: float64(len(item.result.RankedIDs) - rank)})
		}
		return docs, item.result.Metrics, nil
	}, cases, []int{1, 3, 5})
	if err != nil {
		return "", nil, nil, err
	}
	return "rag-metrics/v1", MarshalDomain(summary), nil, nil
}

func failedCase(id, phase string, err error) CaseResult {
	return CaseResult{CaseID: id, Status: StatusFailed, FailurePhase: phase, Reason: err.Error()}
}
