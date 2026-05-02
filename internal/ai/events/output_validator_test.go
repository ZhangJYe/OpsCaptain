package events

import (
	"testing"
)

func TestValidateOutput_NoToolResults(t *testing.T) {
	warnings := ValidateOutputAgainstToolResults("P99延迟是200ms", nil)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings when no tool results, got %d", len(warnings))
	}
}

func TestValidateOutput_MetricFound(t *testing.T) {
	toolResults := []string{"P99: 200ms, error_rate: 5%"}
	warnings := ValidateOutputAgainstToolResults("P99延迟是200ms，错误率5%", toolResults)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings when metrics found, got %v", warnings)
	}
}

func TestValidateOutput_MetricNotFound(t *testing.T) {
	toolResults := []string{"P99: 200ms"}
	warnings := ValidateOutputAgainstToolResults("P99延迟是200ms，错误率5%", toolResults)
	if len(warnings) == 0 {
		t.Fatal("expected warning for missing error_rate metric")
	}
}

func TestValidateOutput_NoMetricsInOutput(t *testing.T) {
	toolResults := []string{"P99: 200ms"}
	warnings := ValidateOutputAgainstToolResults("系统运行正常", toolResults)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings when no metrics in output, got %v", warnings)
	}
}

func TestExtractMetrics(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"P99延迟是200ms", 1},
		{"错误率5%，延迟200ms", 2},
		{"没有指标", 0},
		{"P99: 200ms, P50: 100ms", 2},
	}

	for _, tt := range tests {
		metrics := extractMetrics(tt.input)
		if len(metrics) != tt.expected {
			t.Errorf("extractMetrics(%q) = %d metrics, want %d: %v", tt.input, len(metrics), tt.expected, metrics)
		}
	}
}
