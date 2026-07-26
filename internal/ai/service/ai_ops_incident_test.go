package service

import (
	"context"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

func TestAIOpsIncidentAppendUsesExistingIncidentContext(t *testing.T) {
	setupIncidentTest(t)

	var contexts []string
	runIncidentAIOps = func(ctx context.Context, query string) (ExecutionResponse, error) {
		contexts = append(contexts, aiOpsIncidentContext(ctx))
		return ExecutionResponse{
			Content: "result: " + query,
			TraceID: "trace-" + query,
			Engine:  "plan_execute_replan",
			Status:  protocol.ResultStatusSucceeded,
		}, nil
	}

	created, err := CreateAIOpsIncident(context.Background(), "payment timeout", "", nil)
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	if created.IncidentID == "" || created.SessionID == "" || len(created.Turns) != 1 || created.Turns[0].TurnID == "" {
		t.Fatalf("unexpected incident identifiers: %#v", created)
	}

	appended, err := AppendAIOpsIncidentTurn(context.Background(), created.IncidentID, "checkout errors", nil)
	if err != nil {
		t.Fatalf("append incident turn: %v", err)
	}
	if appended.IncidentID != created.IncidentID {
		t.Fatalf("expected same incident id, got %q", appended.IncidentID)
	}

	restored, err := GetAIOpsIncident(context.Background(), created.IncidentID)
	if err != nil {
		t.Fatalf("restore incident: %v", err)
	}
	if restored.Status != IncidentStatusCompleted || len(restored.Turns) != 2 {
		t.Fatalf("unexpected restored incident: %#v", restored)
	}
	if len(contexts) != 2 {
		t.Fatalf("expected two executions, got %d", len(contexts))
	}
	if !strings.Contains(contexts[1], "payment timeout") || !strings.Contains(contexts[1], "result: payment timeout") {
		t.Fatalf("expected previous turn context, got %q", contexts[1])
	}
}

func TestAIOpsIncidentRejectsConcurrentTurn(t *testing.T) {
	setupIncidentTest(t)

	startIncidentRun = func(func()) {}
	created, err := CreateAIOpsIncident(context.Background(), "api latency", "", nil)
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}

	if _, err := AppendAIOpsIncidentTurn(context.Background(), created.IncidentID, "more evidence", nil); err != ErrIncidentTurnRunning {
		t.Fatalf("expected running turn error, got %v", err)
	}
}

func TestAIOpsIncidentApprovalExecutionUpdatesOriginalTurn(t *testing.T) {
	setupIncidentTest(t)

	runIncidentAIOps = func(context.Context, string) (ExecutionResponse, error) {
		return ExecutionResponse{
			Content:           "approval needed",
			ApprovalRequired:  true,
			ApprovalRequestID: "approval-1",
			ApprovalStatus:    string(ApprovalStatusPending),
			Status:            protocol.ResultStatusSucceeded,
		}, nil
	}

	created, err := CreateAIOpsIncident(context.Background(), "restart paymentservice", "", nil)
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	waiting, err := GetAIOpsIncident(context.Background(), created.IncidentID)
	if err != nil {
		t.Fatalf("get waiting incident: %v", err)
	}
	if waiting.Status != IncidentStatusWaitingApproval {
		t.Fatalf("expected waiting approval, got %s", waiting.Status)
	}

	err = RecordIncidentApprovalExecution(context.Background(), "approval-1", ExecutionResponse{
		Content:        "restart verified",
		TraceID:        "trace-approved",
		Engine:         "plan_execute_replan",
		Status:         protocol.ResultStatusSucceeded,
		ApprovalStatus: string(ApprovalStatusExecuted),
	})
	if err != nil {
		t.Fatalf("record approval execution: %v", err)
	}

	completed, err := GetAIOpsIncident(context.Background(), created.IncidentID)
	if err != nil {
		t.Fatalf("get completed incident: %v", err)
	}
	if completed.Status != IncidentStatusCompleted || len(completed.Turns) != 1 {
		t.Fatalf("unexpected completed incident: %#v", completed)
	}
	if completed.Turns[0].Result != "restart verified" || completed.Turns[0].ApprovalStatus != string(ApprovalStatusExecuted) {
		t.Fatalf("unexpected approved turn: %#v", completed.Turns[0])
	}
	if !hasIncidentEventType(completed.Events, "approval_executed") {
		t.Fatalf("expected approval execution event: %#v", completed.Events)
	}
}

func setupIncidentTest(t *testing.T) {
	t.Helper()

	oldConfigString := incidentConfigString
	oldConfigBool := incidentConfigBool
	oldConfigInt := incidentConfigInt
	oldRun := runIncidentAIOps
	oldStart := startIncidentRun
	oldNewStore := newIncidentStore
	oldStores := incidentStores
	dir := t.TempDir()

	incidentStores = map[string]IncidentStore{}
	incidentConfigString = func(_ context.Context, key string) (string, bool) {
		if key == "aiops.incident.store_dir" {
			return dir, true
		}
		return "", false
	}
	incidentConfigBool = func(_ context.Context, key string) (bool, bool) {
		if key == "aiops.incident.enabled" {
			return true, true
		}
		return false, false
	}
	incidentConfigInt = func(_ context.Context, _ string) (int, bool) {
		return 0, false
	}
	newIncidentStore = func(storeDir string) (IncidentStore, error) {
		return NewFileIncidentStore(storeDir)
	}
	startIncidentRun = func(run func()) {
		run()
	}

	t.Cleanup(func() {
		incidentConfigString = oldConfigString
		incidentConfigBool = oldConfigBool
		incidentConfigInt = oldConfigInt
		runIncidentAIOps = oldRun
		startIncidentRun = oldStart
		newIncidentStore = oldNewStore
		incidentStores = oldStores
	})
}

func hasIncidentEventType(events []IncidentEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
