package eval

import (
	"context"
	"fmt"
	"sort"
)

func Run(ctx context.Context, searcher Searcher, cases []EvalCase, ks []int) (Summary, []CaseResult, error) {
	if searcher == nil {
		return Summary{}, nil, fmt.Errorf("searcher is nil")
	}
	ks = normalizeKs(ks)
	if len(ks) == 0 {
		return Summary{}, nil, fmt.Errorf("ks is empty")
	}

	maxK := ks[len(ks)-1]
	results := make([]CaseResult, 0, len(cases))
	summary := Summary{
		Cases:         len(cases),
		AvgRecallAtK:  make(map[int]float64, len(ks)),
		HitRateAtK:    make(map[int]float64, len(ks)),
		FullRecallAtK: make(map[int]int, len(ks)),
	}

	for _, evalCase := range cases {
		rankedDocs, err := searcher.Search(ctx, evalCase.Query, maxK)
		if err != nil {
			return Summary{}, nil, fmt.Errorf("case %s search failed: %w", evalCase.ID, err)
		}
		rankedIDs := make([]string, 0, len(rankedDocs))
		for _, doc := range rankedDocs {
			if doc.ID == "" {
				continue
			}
			rankedIDs = append(rankedIDs, doc.ID)
		}

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
			recall := computeRecall(evalCase.RelevantIDs, hits)
			result.HitIDsByK[k] = hits
			result.RecallAtK[k] = recall
			summary.AvgRecallAtK[k] += recall
			if len(hits) > 0 {
				summary.HitRateAtK[k]++
			}
			if len(hits) == len(uniqueIDs(evalCase.RelevantIDs)) && len(evalCase.RelevantIDs) > 0 {
				summary.FullRecallAtK[k]++
			}
		}
		results = append(results, result)
	}

	if len(cases) == 0 {
		return summary, results, nil
	}

	caseCount := float64(len(cases))
	for _, k := range ks {
		summary.AvgRecallAtK[k] /= caseCount
		summary.HitRateAtK[k] /= caseCount
	}
	return summary, results, nil
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
