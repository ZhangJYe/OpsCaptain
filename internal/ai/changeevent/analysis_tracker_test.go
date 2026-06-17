package changeevent

import (
	"testing"
	"time"
)

func TestAnalysisTracker_StoreAndLoad(t *testing.T) {
	tracker := NewAnalysisTracker(10)

	record := &AnalysisRecord{
		TraceID:   "trace-001",
		TaskID:    "task-001",
		Service:   "payment-service",
		EventType: "deploy",
		Summary:   "test",
		StartedAt: time.Now(),
	}

	tracker.Store("event-001", record)

	got, ok := tracker.Load("event-001")
	if !ok {
		t.Fatal("expected to find record")
	}
	if got.TraceID != "trace-001" {
		t.Errorf("expected trace-001, got %s", got.TraceID)
	}
	if got.Service != "payment-service" {
		t.Errorf("expected payment-service, got %s", got.Service)
	}
}

func TestAnalysisTracker_Delete(t *testing.T) {
	tracker := NewAnalysisTracker(10)
	tracker.Store("event-001", &AnalysisRecord{TraceID: "t1"})
	tracker.Delete("event-001")

	_, ok := tracker.Load("event-001")
	if ok {
		t.Fatal("expected record to be deleted")
	}
}

func TestAnalysisTracker_Evict(t *testing.T) {
	tracker := NewAnalysisTracker(4)

	for i := 0; i < 6; i++ {
		tracker.Store(
			string(rune('a'+i)),
			&AnalysisRecord{
				TraceID:   string(rune('0' + i)),
				StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			},
		)
	}

	tracker.mu.RLock()
	count := len(tracker.records)
	tracker.mu.RUnlock()

	if count > 4 {
		t.Errorf("expected at most 4 records after evict, got %d", count)
	}
}
