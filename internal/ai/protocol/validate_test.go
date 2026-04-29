package protocol

import (
	"strings"
	"testing"
)

func validResult() *TaskResult {
	return &TaskResult{
		TaskID:     "task-001",
		Agent:      "metrics",
		Status:     ResultStatusSucceeded,
		Summary:    "All metrics within normal range",
		Confidence: 0.95,
		Evidence: []EvidenceItem{
			{SourceType: "prometheus_alert", SourceID: "alert-1", Title: "CPU Alert", Snippet: "CPU usage at 85%"},
		},
	}
}

func TestValidateTaskResult_Nil(t *testing.T) {
	err := ValidateTaskResult(nil)
	if err == nil {
		t.Fatal("expected error for nil result")
	}
}

func TestValidateTaskResult_ValidSucceeded(t *testing.T) {
	if err := ValidateTaskResult(validResult()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTaskResult_ValidDegraded(t *testing.T) {
	r := validResult()
	r.Status = ResultStatusDegraded
	r.DegradationReason = "Prometheus timeout"
	if err := ValidateTaskResult(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTaskResult_ValidFailed(t *testing.T) {
	r := validResult()
	r.Status = ResultStatusFailed
	r.Error = &TaskError{Code: "TIMEOUT", Message: "prometheus unreachable"}
	if err := ValidateTaskResult(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTaskResult_EmptyTaskID(t *testing.T) {
	r := validResult()
	r.TaskID = ""
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("expected task_id error, got: %v", err)
	}
}

func TestValidateTaskResult_EmptyAgent(t *testing.T) {
	r := validResult()
	r.Agent = ""
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "agent") {
		t.Fatalf("expected agent error, got: %v", err)
	}
}

func TestValidateTaskResult_InvalidStatus(t *testing.T) {
	r := validResult()
	r.Status = "partial"
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "invalid status") {
		t.Fatalf("expected status error, got: %v", err)
	}
}

func TestValidateTaskResult_EmptySummary(t *testing.T) {
	r := validResult()
	r.Summary = ""
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "summary") {
		t.Fatalf("expected summary error, got: %v", err)
	}
}

func TestValidateTaskResult_SummaryTooLong(t *testing.T) {
	r := validResult()
	r.Summary = strings.Repeat("x", 4097)
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "4096") {
		t.Fatalf("expected length error, got: %v", err)
	}
}

func TestValidateTaskResult_DegradedNoReason(t *testing.T) {
	r := validResult()
	r.Status = ResultStatusDegraded
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "degradation_reason") {
		t.Fatalf("expected degradation_reason error, got: %v", err)
	}
}

func TestValidateTaskResult_FailedNoError(t *testing.T) {
	r := validResult()
	r.Status = ResultStatusFailed
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "error is required") {
		t.Fatalf("expected error field error, got: %v", err)
	}
}

func TestValidateTaskResult_InvalidConfidence(t *testing.T) {
	r := validResult()
	r.Confidence = 1.5
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "confidence") {
		t.Fatalf("expected confidence error, got: %v", err)
	}
}

func TestValidateTaskResult_EvidenceMissingSourceType(t *testing.T) {
	r := validResult()
	r.Evidence = []EvidenceItem{{Title: "test", Snippet: "data"}}
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "source_type") {
		t.Fatalf("expected source_type error, got: %v", err)
	}
}

func TestValidateTaskResult_EvidenceMissingTitle(t *testing.T) {
	r := validResult()
	r.Evidence = []EvidenceItem{{SourceType: "log", Snippet: "data"}}
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("expected title error, got: %v", err)
	}
}

func TestValidateTaskResult_ErrorEmptyCodeAndMessage(t *testing.T) {
	r := validResult()
	r.Error = &TaskError{}
	err := ValidateTaskResult(r)
	if err == nil || !strings.Contains(err.Error(), "error must have") {
		t.Fatalf("expected error validation error, got: %v", err)
	}
}

func TestValidateTaskResult_EmptyEvidenceOK(t *testing.T) {
	r := validResult()
	r.Evidence = nil
	if err := ValidateTaskResult(r); err != nil {
		t.Fatalf("unexpected error for nil evidence: %v", err)
	}
}
