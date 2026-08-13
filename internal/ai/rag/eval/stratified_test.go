package eval

import "testing"

func TestBuildStratifiedAnalysis(t *testing.T) {
	t.Parallel()
	cases := []EvalCase{
		{ID: "a", RelevantIDs: []string{"doc-a", "doc-b"}, Category: "multi_document", Difficulty: "hard", Language: "zh"},
		{ID: "b", RelevantIDs: []string{"doc-c"}, Category: "hard_negative", Difficulty: "hard", Language: "en"},
		{ID: "c", RelevantIDs: []string{"doc-d"}, Category: "hard_negative", Difficulty: "medium", Language: "en"},
	}
	results := []QueryCaseResult{
		{CaseResult: CaseResult{CaseID: "a", RelevantIDs: cases[0].RelevantIDs, RankedIDs: []string{"doc-a"}, HitIDsByK: map[int][]string{1: {"doc-a"}, 5: {"doc-a"}}, RecallAtK: map[int]float64{1: .5, 5: .5}}},
		{CaseResult: CaseResult{CaseID: "b", RelevantIDs: cases[1].RelevantIDs, RankedIDs: []string{"doc-c"}, HitIDsByK: map[int][]string{1: {"doc-c"}, 5: {"doc-c"}}, RecallAtK: map[int]float64{1: 1, 5: 1}}},
	}
	analysis := BuildStratifiedAnalysis(cases, results, []CaseFailure{{CaseID: "c", Error: "timeout"}}, []int{1, 5})
	multi := analysis.ByCategory["multi_document"]
	if multi.Cases != 1 || multi.AvgRecallAtK[5] != .5 || multi.MRR != 1 {
		t.Fatalf("unexpected multi-document metrics: %+v", multi)
	}
	hardNegative := analysis.ByCategory["hard_negative"]
	if hardNegative.Cases != 2 || hardNegative.Succeeded != 1 || hardNegative.Failed != 1 || hardNegative.FailureRate != .5 || hardNegative.AvgRecallAtK[5] != 1 {
		t.Fatalf("failure denominator is incorrect: %+v", hardNegative)
	}
	if len(analysis.IncompleteAt5) != 1 || analysis.IncompleteAt5[0].MissingIDs[0] != "doc-b" || analysis.MissingIDFrequency["doc-b"] != 1 {
		t.Fatalf("incomplete recall analysis missing: %+v", analysis)
	}
}

func TestBuildStratifiedAnalysisEmpty(t *testing.T) {
	t.Parallel()
	analysis := BuildStratifiedAnalysis(nil, nil, nil, []int{1, 5})
	if len(analysis.ByCategory) != 0 || len(analysis.IncompleteAt5) != 0 || len(analysis.MissingIDFrequency) != 0 {
		t.Fatalf("empty analysis must remain empty: %+v", analysis)
	}
}
