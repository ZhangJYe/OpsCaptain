package eval

import "testing"

func TestBuildCaseStageDiagnosticsClassifiesAllGaps(t *testing.T) {
	diagnostics := BuildCaseStageDiagnostics(
		[]string{"never", "fusion-lost", "candidate-lost", "final-lost", "final"},
		RetrievalStages{
			Dense:     []string{"fusion-lost", "candidate-lost", "final-lost", "final"},
			Lexical:   []string{"final"},
			Fusion:    []string{"candidate-lost", "final-lost", "final"},
			Candidate: []string{"final-lost", "final"},
			Final:     []string{"final"},
		},
	)
	want := map[string]string{
		"never": "not_recalled", "fusion-lost": "lost_at_fusion", "candidate-lost": "lost_at_candidate",
		"final-lost": "lost_at_final", "final": "reached_final",
	}
	for _, item := range diagnostics.Relevant {
		if item.GapClass != want[item.RelevantID] {
			t.Fatalf("%s gap=%s want=%s", item.RelevantID, item.GapClass, want[item.RelevantID])
		}
	}
	if diagnostics.RecallAtK["final"][1] != 0.2 || diagnostics.RecallAtK["dense"][5] != 0.8 {
		t.Fatalf("unexpected stage recall: %+v", diagnostics.RecallAtK)
	}
}
