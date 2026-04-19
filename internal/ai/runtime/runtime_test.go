package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"SuperBizAgent/internal/ai/protocol"
)

type fakeAgent struct {
	name    string
	summary string
	status  protocol.ResultStatus
	delay   time.Duration
}

func (f *fakeAgent) Name() string {
	return f.name
}

func (f *fakeAgent) Capabilities() []string {
	return []string{"test"}
}

func (f *fakeAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	summary := f.summary
	if summary == "" {
		summary = "ok"
	}
	status := f.status
	if status == "" {
		status = protocol.ResultStatusSucceeded
	}
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      f.name,
		Status:     status,
		Summary:    summary,
		Confidence: 1,
	}, nil
}

func TestRuntimeDispatchRecordsDetails(t *testing.T) {
	rt := New()
	if err := rt.Register(&fakeAgent{name: "fake"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	task := protocol.NewRootTask("session-test", "do something", "fake")
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result == nil || result.Summary != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}

	details := rt.DetailMessages(context.Background(), task.TraceID)
	if len(details) < 2 {
		t.Fatalf("expected runtime to record start/completion details, got %d", len(details))
	}
}

func TestRuntimeDispatchTimeoutReturnsFailureAndTraceEvent(t *testing.T) {
	rt := New()
	if err := rt.Register(&fakeAgent{name: "slow", delay: 120 * time.Millisecond}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	task := protocol.NewRootTask("session-timeout", "slow work", "slow")
	task.Constraints = map[string]any{"timeout_ms": 20}

	started := time.Now()
	result, err := rt.Dispatch(context.Background(), task)
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("expected dispatch to return before slow handler completed, took %s", elapsed)
	}
	if result == nil || result.Status != protocol.ResultStatusFailed {
		t.Fatalf("expected failed result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != "timeout" {
		t.Fatalf("expected timeout error code, got %#v", result.Error)
	}

	events, traceErr := rt.TraceEvents(context.Background(), task.TraceID)
	if traceErr != nil {
		t.Fatalf("trace events: %v", traceErr)
	}
	foundTimeout := false
	for _, event := range events {
		if event != nil && event.Type == "task_timeout" {
			foundTimeout = true
			break
		}
	}
	if !foundTimeout {
		t.Fatalf("expected task_timeout event, got %#v", events)
	}
}

func TestRuntimeNormalizesDegradedReason(t *testing.T) {
	rt := New()
	if err := rt.Register(&fakeAgent{name: "degraded", status: protocol.ResultStatusDegraded, summary: "partial data available"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	task := protocol.NewRootTask("session-degraded", "degraded work", "degraded")
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result == nil || result.DegradationReason == "" {
		t.Fatalf("expected degradation reason, got %#v", result)
	}

	events, traceErr := rt.TraceEvents(context.Background(), task.TraceID)
	if traceErr != nil {
		t.Fatalf("trace events: %v", traceErr)
	}
	foundReason := false
	for _, event := range events {
		if event != nil && event.Type == "task_completed" && event.Payload != nil {
			if reason, ok := event.Payload["degradation_reason"].(string); ok && reason != "" {
				foundReason = true
			}
		}
	}
	if !foundReason {
		t.Fatal("expected degradation reason in completed event payload")
	}
}

func TestRuntimeDetailMessagesOmitVerboseSummaryBodies(t *testing.T) {
	rt := New()
	longSummary := strings.Repeat("very long summary body ", 20) + "\nwith extra lines"
	if err := rt.Register(&fakeAgent{name: "reporter", summary: longSummary}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	task := protocol.NewRootTask("session-test", "do something", "reporter")
	if _, err := rt.Dispatch(context.Background(), task); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	details := rt.DetailMessages(context.Background(), task.TraceID)
	joined := strings.Join(details, "\n")
	if strings.Contains(joined, "with extra lines") {
		t.Fatalf("expected verbose report body to be omitted from details, got:\n%s", joined)
	}
}

func TestInMemoryLedgerPrunesLimits(t *testing.T) {
	ledger := NewInMemoryLedgerWithLimits(2, 2, 3)
	ctx := context.Background()

	task1 := protocol.NewRootTask("s1", "goal1", "agent")
	task2 := protocol.NewRootTask("s2", "goal2", "agent")
	task3 := protocol.NewRootTask("s3", "goal3", "agent")

	if err := ledger.CreateTask(ctx, task1); err != nil {
		t.Fatalf("create task1: %v", err)
	}
	if err := ledger.CreateTask(ctx, task2); err != nil {
		t.Fatalf("create task2: %v", err)
	}
	if err := ledger.CreateTask(ctx, task3); err != nil {
		t.Fatalf("create task3: %v", err)
	}

	if err := ledger.AppendResult(ctx, task1.TaskID, &protocol.TaskResult{TaskID: task1.TaskID, Agent: "agent", Status: protocol.ResultStatusSucceeded}); err != nil {
		t.Fatalf("append result1: %v", err)
	}
	if err := ledger.AppendResult(ctx, task2.TaskID, &protocol.TaskResult{TaskID: task2.TaskID, Agent: "agent", Status: protocol.ResultStatusSucceeded}); err != nil {
		t.Fatalf("append result2: %v", err)
	}
	if err := ledger.AppendResult(ctx, task3.TaskID, &protocol.TaskResult{TaskID: task3.TaskID, Agent: "agent", Status: protocol.ResultStatusSucceeded}); err != nil {
		t.Fatalf("append result3: %v", err)
	}

	for i := 1; i <= 5; i++ {
		if err := ledger.AppendEvent(ctx, &protocol.TaskEvent{
			TraceID:   "trace-test",
			TaskID:    task3.TaskID,
			Type:      "task_info",
			CreatedAt: int64(i),
		}); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	ledger.mu.RLock()
	taskCount := len(ledger.tasks)
	resultCount := len(ledger.results)
	eventsCount := len(ledger.events)
	_, task1Exists := ledger.tasks[task1.TaskID]
	_, result1Exists := ledger.results[task1.TaskID]
	ledger.mu.RUnlock()

	if taskCount != 2 {
		t.Fatalf("expected 2 tasks after prune, got %d", taskCount)
	}
	if resultCount != 2 {
		t.Fatalf("expected 2 results after prune, got %d", resultCount)
	}
	if eventsCount != 3 {
		t.Fatalf("expected 3 events after prune, got %d", eventsCount)
	}
	if task1Exists {
		t.Fatalf("expected oldest task %s to be pruned", task1.TaskID)
	}
	if result1Exists {
		t.Fatalf("expected oldest result %s to be pruned", task1.TaskID)
	}

	events, err := ledger.EventsByTrace(ctx, "trace-test")
	if err != nil {
		t.Fatalf("events by trace: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 trace events, got %d", len(events))
	}
	if events[0].CreatedAt != 3 {
		t.Fatalf("expected oldest retained event created_at=3, got %d", events[0].CreatedAt)
	}
}
