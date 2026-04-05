package contextengine

import (
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

func TestToolItemsFromResultsPrefersEvidence(t *testing.T) {
	results := []*protocol.TaskResult{
		{
			TaskID:     "task-1",
			Agent:      "metrics",
			Status:     protocol.ResultStatusSucceeded,
			Summary:    "发现 1 条告警。",
			Confidence: 0.8,
			Evidence: []protocol.EvidenceItem{
				{
					SourceType: "prometheus",
					SourceID:   "HighLatency",
					Title:      "HighLatency",
					Snippet:    "payment-service 延迟过高",
					Score:      0.8,
				},
			},
		},
	}

	items := ToolItemsFromResults(results)
	if len(items) != 1 {
		t.Fatalf("expected 1 tool item, got %d", len(items))
	}
	if items[0].SourceType != "prometheus" {
		t.Fatalf("unexpected source type: %q", items[0].SourceType)
	}
	if !strings.Contains(items[0].Content, "payment-service") {
		t.Fatalf("unexpected item content: %q", items[0].Content)
	}
}

func TestToolItemsFromResultsFallsBackToSummary(t *testing.T) {
	results := []*protocol.TaskResult{
		{
			TaskID:     "task-2",
			Agent:      "logs",
			Status:     protocol.ResultStatusDegraded,
			Summary:    "日志工具已执行，但没有结构化证据。",
			Confidence: 0.4,
		},
	}

	items := ToolItemsFromResults(results)
	if len(items) != 1 {
		t.Fatalf("expected 1 fallback tool item, got %d", len(items))
	}
	if items[0].SourceType != "tool_result" {
		t.Fatalf("unexpected source type: %q", items[0].SourceType)
	}
	if !strings.Contains(items[0].Content, "日志工具已执行") {
		t.Fatalf("unexpected fallback content: %q", items[0].Content)
	}
}
