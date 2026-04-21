package reporter

import (
	"context"
	"strings"
	"testing"

	agentcontracts "SuperBizAgent/internal/ai/agent/contracts"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

func TestReporterBuildsToolItemContextAndEmitsTrace(t *testing.T) {
	rt := runtime.New()
	agent := New()
	if err := rt.Register(agent); err != nil {
		t.Fatalf("register reporter: %v", err)
	}

	task := protocol.NewRootTask("sess-reporter", "分析当前告警", AgentName)
	task.Input = map[string]any{
		"query":         "分析当前告警",
		"response_mode": "chat",
		"results": []*protocol.TaskResult{
			{
				TaskID:     "metrics-task",
				Agent:      "metrics",
				Status:     protocol.ResultStatusSucceeded,
				Summary:    "found 1 alert",
				Confidence: 0.8,
				Evidence: []protocol.EvidenceItem{{
					SourceType: "prometheus",
					SourceID:   "HighLatency",
					Title:      "HighLatency",
					Snippet:    "payment-service latency is high",
					Score:      0.8,
				}},
			},
		},
	}

	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch reporter: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if !strings.Contains(result.Summary, "依据：") {
		t.Fatalf("expected chat response to include Chinese evidence section, got %q", result.Summary)
	}
	if got, _ := result.Metadata["tool_item_count"].(int); got != 1 {
		t.Fatalf("expected tool_item_count=1, got %v", result.Metadata["tool_item_count"])
	}
	if result.Metadata["agent_contract_id"] != "reporter:"+agentcontracts.Version {
		t.Fatalf("expected reporter contract metadata, got %#v", result.Metadata)
	}

	detail := rt.DetailMessages(context.Background(), task.TraceID)
	foundReporterContext := false
	for _, line := range detail {
		if strings.Contains(line, "tool_results selected=") {
			foundReporterContext = true
			break
		}
	}
	if !foundReporterContext {
		t.Fatalf("expected reporter context detail in trace, got %v", detail)
	}
}

func TestReporterUsesEnglishOnlyWhenRequested(t *testing.T) {
	rt := runtime.New()
	agent := New()
	if err := rt.Register(agent); err != nil {
		t.Fatalf("register reporter: %v", err)
	}

	task := protocol.NewRootTask("sess-reporter-en", "please answer in english about current alerts", AgentName)
	task.Input = map[string]any{
		"query":         "please answer in english about current alerts",
		"response_mode": "chat",
		"intent":        "incident_analysis",
		"results": []*protocol.TaskResult{
			{
				TaskID:     "logs-task",
				Agent:      "logs",
				Status:     protocol.ResultStatusSucceeded,
				Summary:    "found timeout errors",
				Confidence: 0.8,
			},
		},
	}

	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch reporter: %v", err)
	}
	if !strings.Contains(result.Summary, "Question:") {
		t.Fatalf("expected English response when explicitly requested, got %q", result.Summary)
	}
	if strings.Contains(result.Summary, "问题：") {
		t.Fatalf("expected no Chinese question label in English mode, got %q", result.Summary)
	}
}
