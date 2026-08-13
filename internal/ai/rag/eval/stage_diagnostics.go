package eval

var stageMetricKs = []int{1, 3, 5, 20}

type RelevantStageDiagnostic struct {
	RelevantID string         `json:"relevant_id"`
	Ranks      map[string]int `json:"ranks"`
	LastStage  string         `json:"last_stage"`
	GapClass   string         `json:"gap_class"`
}

type CaseStageDiagnostics struct {
	RecallAtK map[string]map[int]float64 `json:"recall_at_k"`
	Relevant  []RelevantStageDiagnostic  `json:"relevant"`
}

func BuildCaseStageDiagnostics(relevantIDs []string, stages RetrievalStages) CaseStageDiagnostics {
	stageIDs := orderedStageIDs(stages)
	result := CaseStageDiagnostics{
		RecallAtK: make(map[string]map[int]float64, len(stageIDs)),
		Relevant:  make([]RelevantStageDiagnostic, 0, len(relevantIDs)),
	}
	for _, stage := range stageIDs {
		result.RecallAtK[stage.name] = make(map[int]float64, len(stageMetricKs))
		for _, k := range stageMetricKs {
			result.RecallAtK[stage.name][k] = computeRecall(relevantIDs, hitIDs(relevantIDs, stage.ids, k))
		}
	}
	for _, relevantID := range uniqueIDs(relevantIDs) {
		diagnostic := RelevantStageDiagnostic{RelevantID: relevantID, Ranks: make(map[string]int)}
		for _, stage := range stageIDs {
			if rank := rankOf(relevantID, stage.ids); rank > 0 {
				diagnostic.Ranks[stage.name] = rank
				diagnostic.LastStage = stage.name
			}
		}
		diagnostic.GapClass = classifyStageGap(diagnostic.Ranks)
		result.Relevant = append(result.Relevant, diagnostic)
	}
	return result
}

func newStageRecallSummary() map[string]map[int]float64 {
	result := make(map[string]map[int]float64, 5)
	for _, stage := range []string{"dense", "lexical", "fusion", "candidate", "final"} {
		result[stage] = make(map[int]float64, len(stageMetricKs))
	}
	return result
}

func accumulateStageDiagnostics(summary *QuerySummary, diagnostic CaseStageDiagnostics) {
	for stage, recallByK := range diagnostic.RecallAtK {
		for k, recall := range recallByK {
			summary.StageRecallAtK[stage][k] += recall
		}
	}
	for _, relevant := range diagnostic.Relevant {
		summary.StageGapCounts[relevant.GapClass]++
	}
}

func orderedStageIDs(stages RetrievalStages) []struct {
	name string
	ids  []string
} {
	return []struct {
		name string
		ids  []string
	}{
		{name: "dense", ids: stages.Dense},
		{name: "lexical", ids: stages.Lexical},
		{name: "fusion", ids: stages.Fusion},
		{name: "candidate", ids: stages.Candidate},
		{name: "final", ids: stages.Final},
	}
}

func classifyStageGap(ranks map[string]int) string {
	if ranks["final"] > 0 {
		return "reached_final"
	}
	if ranks["candidate"] > 0 {
		return "lost_at_final"
	}
	if ranks["fusion"] > 0 {
		return "lost_at_candidate"
	}
	if ranks["dense"] > 0 || ranks["lexical"] > 0 {
		return "lost_at_fusion"
	}
	return "not_recalled"
}

func rankOf(target string, ids []string) int {
	for i, id := range ids {
		if id == target {
			return i + 1
		}
	}
	return 0
}
