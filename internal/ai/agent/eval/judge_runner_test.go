package eval

import (
	"testing"
)

func TestParseDiagScores_ValidJSON(t *testing.T) {
	input := `{"correctness": 4, "completeness": 3, "coherence": 5, "actionability": 2, "overall": 4, "comments": "good"}`
	scores, err := parseDiagScores(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scores.Correctness != 4 || scores.Completeness != 3 || scores.Coherence != 5 || scores.Actionability != 2 || scores.Overall != 4 {
		t.Fatalf("unexpected scores: %+v", scores)
	}
	if scores.Comments != "good" {
		t.Fatalf("expected comments 'good', got %q", scores.Comments)
	}
}

func TestParseDiagScores_MarkdownJSON(t *testing.T) {
	input := "Here is the evaluation:\n```json\n{\"correctness\": 3, \"completeness\": 4, \"coherence\": 3, \"actionability\": 4, \"overall\": 3, \"comments\": \"ok\"}\n```"
	scores, err := parseDiagScores(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scores.Correctness != 3 {
		t.Fatalf("expected correctness 3, got %d", scores.Correctness)
	}
}

func TestParseDiagScores_InvalidJSON(t *testing.T) {
	input := "this is not json at all"
	_, err := parseDiagScores(input)
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestClampScores_BelowMin(t *testing.T) {
	s := &DiagScores{Correctness: 0, Completeness: -1, Coherence: 1, Actionability: 5, Overall: 3}
	clamped := clampScores(s)
	if clamped.Correctness != 1 || clamped.Completeness != 1 {
		t.Fatalf("expected clamped to 1, got %+v", clamped)
	}
}

func TestClampScores_AboveMax(t *testing.T) {
	s := &DiagScores{Correctness: 6, Completeness: 10, Coherence: 5, Actionability: 4, Overall: 3}
	clamped := clampScores(s)
	if clamped.Correctness != 5 || clamped.Completeness != 5 {
		t.Fatalf("expected clamped to 5, got %+v", clamped)
	}
}

func TestParseDiagScores_EmbeddedJSON(t *testing.T) {
	input := "评分结果如下：\n\n{\"correctness\": 4, \"completeness\": 4, \"coherence\": 4, \"actionability\": 3, \"overall\": 4, \"comments\": \"不错\"}\n\n以上。"
	scores, err := parseDiagScores(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scores.Correctness != 4 || scores.Overall != 4 {
		t.Fatalf("unexpected scores: %+v", scores)
	}
}
