package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

func TestFileLedgerAndArtifactStore(t *testing.T) {
	baseDir := t.TempDir()
	rt, err := NewPersistent(baseDir)
	if err != nil {
		t.Fatalf("new persistent runtime: %v", err)
	}
	if err := rt.Register(&fakeAgent{name: "fake"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	task := protocol.NewRootTask("sess-persist", "persist me", "fake")
	if _, err := rt.Dispatch(context.Background(), task); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	taskFile := filepath.Join(baseDir, "ledger", "tasks", task.TaskID+".json")
	if _, err := os.Stat(taskFile); err != nil {
		t.Fatalf("expected persisted task file: %v", err)
	}

	ref, err := rt.CreateArtifact(context.Background(), "note", "hello", map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if _, err := os.Stat(ref.URI); err != nil {
		t.Fatalf("expected persisted artifact file: %v", err)
	}

	events, err := rt.TraceEvents(context.Background(), task.TraceID)
	if err != nil {
		t.Fatalf("trace events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected persisted trace events, got %d", len(events))
	}

	reloaded, err := NewPersistent(baseDir)
	if err != nil {
		t.Fatalf("reload persistent runtime: %v", err)
	}
	reloadedEvents, err := reloaded.TraceEvents(context.Background(), task.TraceID)
	if err != nil {
		t.Fatalf("reloaded trace events: %v", err)
	}
	if len(reloadedEvents) != len(events) {
		t.Fatalf("expected reloaded events len %d, got %d", len(events), len(reloadedEvents))
	}
}
