package eval

import (
	"context"
	"fmt"
	"sort"
)

func Run(ctx context.Context, searcher Searcher, cases []EvalCase, ks []int) (Summary, []CaseResult, error) {
	return RunWithOpts(ctx, searcher, cases, ks, defaultRunOptions())
}

func RunWithOpts(ctx context.Context, searcher Searcher, cases []EvalCase, ks []int, opts RunOptions) (Summary, []CaseResult, error) {
	if searcher == nil {
		return Summary{}, nil, fmt.Errorf("searcher is nil")
	}
	ks = normalizeKs(ks)
	if len(ks) == 0 {
		return Summary{}, nil, fmt.Errorf("ks is empty")
	}

	maxK := ks[len(ks)-1]
	results := make([]CaseResult, 0, len(cases))
	summary := newSummary(len(cases), ks)

	for _, evalCase := range cases {
		rankedDocs, err := searcher.Search(ctx, evalCase.Query, maxK)
		if err != nil {
			if opts.ContinueOnError {
				summary.Failures = append(summary.Failures, CaseFailure{
					CaseID: evalCase.ID,
					Error:  err.Error(),
				})
				summary.Failed++
				continue
			}
			return Summary{}, nil, fmt.Errorf("case %s search failed: %w", evalCase.ID, err)
		}

		rankedIDs := extractRankedIDs(rankedDocs)
		result := buildCaseResult(evalCase, rankedIDs, ks)
		accumulateMetrics(&summary, result, evalCase, ks)
		summary.Succeeded++
		results = append(results, result)
	}

	finalizeSummary(&summary, ks)
	return summary, results, nil
}

func buildCaseResult(evalCase EvalCase, rankedIDs []string, ks []int) CaseResult {
	result := CaseResult{
		CaseID:      evalCase.ID,
		Query:       evalCase.Query,
		RelevantIDs: append([]string(nil), evalCase.RelevantIDs...),
		RankedIDs:   rankedIDs,
		HitIDsByK:   make(map[int][]string, len(ks)),
		RecallAtK:   make(map[int]float64, len(ks)),
	}
	for _, k := range ks {
		hits := hitIDs(evalCase.RelevantIDs, rankedIDs, k)
		result.HitIDsByK[k] = hits
		result.RecallAtK[k] = computeRecall(evalCase.RelevantIDs, hits)
	}
	return result
}

func accumulateMetrics(summary *Summary, result CaseResult, evalCase EvalCase, ks []int) {
	uniqueRelevant := uniqueIDs(evalCase.RelevantIDs)
	for _, k := range ks {
		summary.AvgRecallAtK[k] += result.RecallAtK[k]
		if len(result.HitIDsByK[k]) > 0 {
			summary.HitRateAtK[k]++
		}
		if len(evalCase.RelevantIDs) > 0 && len(result.HitIDsByK[k]) == len(uniqueRelevant) {
			summary.FullRecallAtK[k]++
		}
	}
}

func finalizeSummary(summary *Summary, ks []int) {
	if summary.Succeeded == 0 {
		return
	}
	caseCount := float64(summary.Succeeded)
	for _, k := range ks {
		summary.AvgRecallAtK[k] /= caseCount
		summary.HitRateAtK[k] /= caseCount
	}
}

func newSummary(totalCases int, ks []int) Summary {
	return Summary{
		Cases:         totalCases,
		AvgRecallAtK:  make(map[int]float64, len(ks)),
		HitRateAtK:    make(map[int]float64, len(ks)),
		FullRecallAtK: make(map[int]int, len(ks)),
	}
}

func extractRankedIDs(docs []RetrievedDoc) []string {
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc.ID == "" {
			continue
		}
		ids = append(ids, doc.ID)
	}
	return ids
}

func normalizeKs(ks []int) []int {
	if len(ks) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(ks))
	out := make([]int, 0, len(ks))
	for _, k := range ks {
		if k <= 0 {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func hitIDs(relevantIDs, rankedIDs []string, k int) []string {
	relevant := make(map[string]struct{}, len(relevantIDs))
	for _, id := range relevantIDs {
		if id == "" {
			continue
		}
		relevant[id] = struct{}{}
	}

	limit := k
	if limit > len(rankedIDs) {
		limit = len(rankedIDs)
	}
	hits := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, id := range rankedIDs[:limit] {
		if _, ok := relevant[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		hits = append(hits, id)
	}
	return hits
}

func computeRecall(relevantIDs, hits []string) float64 {
	relevantCount := len(uniqueIDs(relevantIDs))
	if relevantCount == 0 {
		return 0
	}
	return float64(len(hits)) / float64(relevantCount)
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
