package safety

import (
	"testing"
)

func TestParseInjectionVerdictValidJSON(t *testing.T) {
	verdict := parseInjectionVerdict(`{"score": 0.85, "reason": "attempting to override system instructions"}`)
	if verdict.Score != 0.85 {
		t.Fatalf("expected score 0.85, got %f", verdict.Score)
	}
	if verdict.Reason != "attempting to override system instructions" {
		t.Fatalf("unexpected reason: %q", verdict.Reason)
	}
}

func TestParseInjectionVerdictWithMarkdownFence(t *testing.T) {
	raw := "```json\n{\"score\": 0.9, \"reason\": \"clear injection\"}\n```"
	verdict := parseInjectionVerdict(raw)
	if verdict.Score != 0.9 {
		t.Fatalf("expected score 0.9, got %f", verdict.Score)
	}
}

func TestParseInjectionVerdictClampHigh(t *testing.T) {
	verdict := parseInjectionVerdict(`{"score": 1.5, "reason": "test"}`)
	if verdict.Score != 1.0 {
		t.Fatalf("expected clamped score 1.0, got %f", verdict.Score)
	}
}

func TestParseInjectionVerdictClampLow(t *testing.T) {
	verdict := parseInjectionVerdict(`{"score": -0.5, "reason": "test"}`)
	if verdict.Score != 0 {
		t.Fatalf("expected clamped score 0, got %f", verdict.Score)
	}
}

func TestParseInjectionVerdictInvalidJSON(t *testing.T) {
	verdict := parseInjectionVerdict("not json at all")
	if verdict.Score != 0 {
		t.Fatalf("expected score 0 for parse error, got %f", verdict.Score)
	}
	if verdict.Reason != "parse error" {
		t.Fatalf("expected reason 'parse error', got %q", verdict.Reason)
	}
}

func TestParseInjectionVerdictEmptyString(t *testing.T) {
	verdict := parseInjectionVerdict("")
	if verdict.Score != 0 {
		t.Fatalf("expected score 0 for empty input, got %f", verdict.Score)
	}
}

func TestClassifyInjectionEmptyInput(t *testing.T) {
	verdict := ClassifyInjection(nil, "")
	if verdict.Score != 0 {
		t.Fatalf("expected score 0 for empty input, got %f", verdict.Score)
	}
	if verdict.Reason != "empty input" {
		t.Fatalf("expected reason 'empty input', got %q", verdict.Reason)
	}
}

func TestClassifierThresholdDefault(t *testing.T) {
	// Without config, should return default threshold
	thresh := ClassifierThreshold(nil)
	if thresh != defaultClassifierThreshold {
		t.Fatalf("expected default threshold %f, got %f", defaultClassifierThreshold, thresh)
	}
}
