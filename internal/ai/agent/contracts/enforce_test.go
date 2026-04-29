package contracts

import (
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

func result(agent string, status protocol.ResultStatus, summary string) *protocol.TaskResult {
	return &protocol.TaskResult{
		TaskID:     "task-001",
		Agent:      agent,
		Status:     status,
		Summary:    summary,
		Confidence: 0.9,
	}
}

func TestValidateAgainstContract_Nil(t *testing.T) {
	if err := ValidateAgainstContract(nil); err == nil {
		t.Fatal("expected error for nil")
	}
}

func TestValidateAgainstContract_UnknownAgent(t *testing.T) {
	r := result("unknown_agent", protocol.ResultStatusSucceeded, "done")
	if err := ValidateAgainstContract(r); err != nil {
		t.Fatalf("unknown agent should pass, got: %v", err)
	}
}

func TestValidateAgainstContract_ValidTriage(t *testing.T) {
	r := result("triage", protocol.ResultStatusSucceeded, "intent=deployment domains=[metrics,logs] priority=high")
	if err := ValidateAgainstContract(r); err != nil {
		t.Fatalf("valid triage result should pass, got: %v", err)
	}
}

func TestValidateAgainstContract_ValidMetrics(t *testing.T) {
	r := result("metrics", protocol.ResultStatusSucceeded, "no active alerts")
	r.Evidence = []protocol.EvidenceItem{
		{SourceType: "prometheus_alert", Title: "CPU Alert", Snippet: "85%"},
	}
	if err := ValidateAgainstContract(r); err != nil {
		t.Fatalf("valid metrics result should pass, got: %v", err)
	}
}

func TestValidateAgainstContract_DegradedMissingReason(t *testing.T) {
	r := result("logs", protocol.ResultStatusDegraded, "log evidence unavailable")
	if err := ValidateAgainstContract(r); err == nil {
		t.Fatal("expected error for degraded without reason")
	}
}

func TestEnforceContract_NormalPasses(t *testing.T) {
	r := result("metrics", protocol.ResultStatusSucceeded, "all clear")
	r.Evidence = []protocol.EvidenceItem{
		{SourceType: "prometheus_alert", Title: "CPU", Snippet: "ok"},
	}
	out := EnforceContract(r)
	if out.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", out.Status)
	}
}

func TestEnforceContract_DegradesOnInvalid(t *testing.T) {
	r := result("metrics", protocol.ResultStatusSucceeded, "")
	out := EnforceContract(r)
	if out.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded, got %s", out.Status)
	}
	if !strings.Contains(out.DegradationReason, "summary") {
		t.Fatalf("expected summary error in degradation reason, got: %s", out.DegradationReason)
	}
}

func TestEnforceContract_DegradesInvalidStatus(t *testing.T) {
	r := result("metrics", "partial", "something")
	out := EnforceContract(r)
	if out.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded for invalid status, got %s", out.Status)
	}
}

func TestEnforceContract_ConfidenceHalved(t *testing.T) {
	r := result("metrics", protocol.ResultStatusSucceeded, "")
	out := EnforceContract(r)
	if out.Confidence > 0.45 {
		t.Fatalf("expected confidence <= 0.45, got %.2f", out.Confidence)
	}
}

func TestEnforceContract_NilReturnsNil(t *testing.T) {
	if out := EnforceContract(nil); out != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestEnforceContract_AppendsDegradationReason(t *testing.T) {
	r := result("metrics", protocol.ResultStatusSucceeded, "")
	r.DegradationReason = "original reason"
	out := EnforceContract(r)
	if out.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", out.Status)
	}
	if !strings.Contains(out.DegradationReason, "original reason") {
		t.Fatalf("expected original reason preserved, got: %s", out.DegradationReason)
	}
	if !strings.Contains(out.DegradationReason, "contract enforcement failed") {
		t.Fatalf("expected enforcement error appended, got: %s", out.DegradationReason)
	}
}
