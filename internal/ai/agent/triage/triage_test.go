package triage

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

func TestTriageAlertAnalysis(t *testing.T) {
	agent := New()
	task := protocol.NewRootTask("session-test", "请分析当前 Prometheus 告警并结合日志排查", agent.Name())

	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	intent, _ := result.Metadata["intent"].(string)
	if intent != "alert_analysis" {
		t.Fatalf("expected alert_analysis intent, got %q", intent)
	}

	domains, _ := result.Metadata["domains"].([]string)
	if len(domains) != 3 {
		t.Fatalf("expected 3 routed domains, got %v", domains)
	}
}
