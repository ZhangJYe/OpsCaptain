package eval

import "sort"

type StratumMetrics struct {
	Cases        int             `json:"cases"`
	Succeeded    int             `json:"succeeded"`
	Failed       int             `json:"failed"`
	AvgRecallAtK map[int]float64 `json:"avg_recall_at_k"`
	HitRateAtK   map[int]float64 `json:"hit_rate_at_k"`
	MRR          float64         `json:"mrr"`
	FailureRate  float64         `json:"failure_rate"`
}

type IncompleteRecallCase struct {
	CaseID     string   `json:"case_id"`
	RecallAt5  float64  `json:"recall_at_5"`
	MissingIDs []string `json:"missing_ids"`
}

type StratifiedAnalysis struct {
	ByCategory         map[string]StratumMetrics `json:"by_category"`
	ByDifficulty       map[string]StratumMetrics `json:"by_difficulty"`
	ByLanguage         map[string]StratumMetrics `json:"by_language"`
	IncompleteAt5      []IncompleteRecallCase    `json:"incomplete_recall_at_5"`
	MissingIDFrequency map[string]int            `json:"missing_id_frequency"`
}

func BuildStratifiedAnalysis(cases []EvalCase, results []QueryCaseResult, failures []CaseFailure, ks []int) StratifiedAnalysis {
	analysis := StratifiedAnalysis{
		ByCategory: make(map[string]StratumMetrics), ByDifficulty: make(map[string]StratumMetrics), ByLanguage: make(map[string]StratumMetrics),
		MissingIDFrequency: make(map[string]int),
	}
	resultByID := make(map[string]QueryCaseResult, len(results))
	for _, result := range results {
		resultByID[result.CaseID] = result
	}
	failureIDs := make(map[string]struct{}, len(failures))
	for _, failure := range failures {
		failureIDs[failure.CaseID] = struct{}{}
	}
	for _, evalCase := range cases {
		result, succeeded := resultByID[evalCase.ID]
		_, failed := failureIDs[evalCase.ID]
		accumulateStratum(analysis.ByCategory, stratumValue(evalCase.Category), evalCase, result, succeeded, failed, ks)
		accumulateStratum(analysis.ByDifficulty, stratumValue(evalCase.Difficulty), evalCase, result, succeeded, failed, ks)
		accumulateStratum(analysis.ByLanguage, stratumValue(evalCase.Language), evalCase, result, succeeded, failed, ks)
		if !succeeded || result.RecallAtK[5] >= 1 {
			continue
		}
		missing := missingRelevantIDs(evalCase.RelevantIDs, result.HitIDsByK[5])
		analysis.IncompleteAt5 = append(analysis.IncompleteAt5, IncompleteRecallCase{CaseID: evalCase.ID, RecallAt5: result.RecallAtK[5], MissingIDs: missing})
		for _, id := range missing {
			analysis.MissingIDFrequency[id]++
		}
	}
	finalizeStrata(analysis.ByCategory, ks)
	finalizeStrata(analysis.ByDifficulty, ks)
	finalizeStrata(analysis.ByLanguage, ks)
	sort.Slice(analysis.IncompleteAt5, func(i, j int) bool { return analysis.IncompleteAt5[i].CaseID < analysis.IncompleteAt5[j].CaseID })
	return analysis
}

func accumulateStratum(groups map[string]StratumMetrics, key string, evalCase EvalCase, result QueryCaseResult, succeeded, failed bool, ks []int) {
	item := groups[key]
	if item.AvgRecallAtK == nil {
		item.AvgRecallAtK = make(map[int]float64, len(ks))
		item.HitRateAtK = make(map[int]float64, len(ks))
	}
	item.Cases++
	if failed {
		item.Failed++
	}
	if succeeded {
		item.Succeeded++
		for _, k := range ks {
			item.AvgRecallAtK[k] += result.RecallAtK[k]
			if len(result.HitIDsByK[k]) > 0 {
				item.HitRateAtK[k]++
			}
		}
		item.MRR += reciprocalRank(evalCase.RelevantIDs, result.RankedIDs)
	}
	groups[key] = item
}

func finalizeStrata(groups map[string]StratumMetrics, ks []int) {
	for key, item := range groups {
		if item.Cases > 0 {
			item.FailureRate = float64(item.Failed) / float64(item.Cases)
		}
		if item.Succeeded > 0 {
			denominator := float64(item.Succeeded)
			for _, k := range ks {
				item.AvgRecallAtK[k] /= denominator
				item.HitRateAtK[k] /= denominator
			}
			item.MRR /= denominator
		}
		groups[key] = item
	}
}

func missingRelevantIDs(relevant, hit []string) []string {
	hitSet := make(map[string]struct{}, len(hit))
	for _, id := range hit {
		hitSet[id] = struct{}{}
	}
	missing := make([]string, 0)
	for _, id := range uniqueIDs(relevant) {
		if _, ok := hitSet[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing
}

func stratumValue(value string) string {
	if value == "" {
		return "unspecified"
	}
	return value
}
