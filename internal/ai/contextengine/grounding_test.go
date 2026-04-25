package contextengine

import (
	"testing"
)

func TestEvaluateGrounding_HighConfidence(t *testing.T) {
	t.Parallel()
	items := []ContextItem{
		{Score: 0.85, Content: "doc1"},
		{Score: 0.78, Content: "doc2"},
	}
	sig := EvaluateGrounding(items)
	if sig.Confidence != RetrievalConfidenceHigh {
		t.Fatalf("expected high confidence, got %s", sig.Confidence)
	}
	if sig.TopScore != 0.85 {
		t.Fatalf("expected top score 0.85, got %.2f", sig.TopScore)
	}
}

func TestEvaluateGrounding_MediumConfidence(t *testing.T) {
	t.Parallel()
	items := []ContextItem{
		{Score: 0.55, Content: "doc1"},
	}
	sig := EvaluateGrounding(items)
	if sig.Confidence != RetrievalConfidenceMedium {
		t.Fatalf("expected medium confidence, got %s", sig.Confidence)
	}
}

func TestEvaluateGrounding_LowConfidence(t *testing.T) {
	t.Parallel()
	items := []ContextItem{
		{Score: 0.30, Content: "doc1"},
	}
	sig := EvaluateGrounding(items)
	if sig.Confidence != RetrievalConfidenceLow {
		t.Fatalf("expected low confidence, got %s", sig.Confidence)
	}
}

func TestEvaluateGrounding_None(t *testing.T) {
	t.Parallel()
	sig := EvaluateGrounding(nil)
	if sig.Confidence != RetrievalConfidenceNone {
		t.Fatalf("expected none confidence, got %s", sig.Confidence)
	}
}

func TestGroundingPreamble_HasWarningForLow(t *testing.T) {
	t.Parallel()
	sig := GroundingSignal{Confidence: RetrievalConfidenceLow}
	preamble := GroundingPreamble(sig)
	if preamble == "" {
		t.Fatal("expected non-empty preamble for low confidence")
	}
}

func TestGroundingPreamble_EmptyForHigh(t *testing.T) {
	t.Parallel()
	sig := GroundingSignal{Confidence: RetrievalConfidenceHigh}
	preamble := GroundingPreamble(sig)
	if preamble != "" {
		t.Fatalf("expected empty preamble for high confidence, got %q", preamble)
	}
}

func TestGroundingPromptDirective_VariesByConfidence(t *testing.T) {
	t.Parallel()
	highDir := GroundingPromptDirective(GroundingSignal{Confidence: RetrievalConfidenceHigh})
	lowDir := GroundingPromptDirective(GroundingSignal{Confidence: RetrievalConfidenceLow})
	if highDir == lowDir {
		t.Fatal("expected different directives for high vs low confidence")
	}
}

func TestDocumentsContentWithCitation_EmptyDocs(t *testing.T) {
	t.Parallel()
	result := DocumentsContentWithCitation(nil)
	if result == "" {
		t.Fatal("expected non-empty placeholder for nil package")
	}
}

func TestDocumentsContentWithCitation_HasCitationNumbers(t *testing.T) {
	t.Parallel()
	pkg := &ContextPackage{
		DocumentItems: []ContextItem{
			{Title: "alert-001", Score: 0.88, Content: "checkoutservice timeout"},
			{Title: "alert-002", Score: 0.72, Content: "paymentservice error"},
		},
	}
	result := DocumentsContentWithCitation(pkg)
	if !contains(result, "[1]") || !contains(result, "[2]") {
		t.Fatalf("expected citation markers [1] and [2], got: %s", result)
	}
	if !contains(result, "0.88") || !contains(result, "0.72") {
		t.Fatal("expected scores in citation header")
	}
}

func TestCheckFaithfulness_SkippedWhenDisabled(t *testing.T) {
	t.Parallel()
	result := CheckFaithfulness(nil, "some answer", []ContextItem{{Content: "doc"}})
	if !result.Skipped {
		t.Fatal("expected faithfulness check to be skipped when disabled")
	}
}

func TestCheckFaithfulness_SkippedEmptyAnswer(t *testing.T) {
	t.Parallel()
	result := CheckFaithfulness(nil, "", []ContextItem{{Content: "doc"}})
	if !result.Skipped {
		t.Fatal("expected faithfulness check to be skipped for empty answer")
	}
}

func TestDetectViolations_CitationOutOfRange(t *testing.T) {
	t.Parallel()
	answer := "根据 [1] 和 [5]，服务超时"
	documents := []ContextItem{
		{Content: "checkoutservice timeout 500ms"},
		{Content: "paymentservice error"},
	}
	violations := detectViolations(answer, documents)
	found := false
	for _, v := range violations {
		if contains(v, "citation_out_of_range") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected citation_out_of_range violation, got: %v", violations)
	}
}

func TestNumbersFoundInDocs(t *testing.T) {
	t.Parallel()
	if !numbersFoundInDocs("timeout is 500ms", "service timeout 500ms") {
		t.Fatal("expected 500 to be found in docs")
	}
	if numbersFoundInDocs("timeout is 99999ms and 88888ms and 77777ms", "no numbers here") {
		t.Fatal("expected numbers NOT to be found in docs")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstr(s, substr)
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
