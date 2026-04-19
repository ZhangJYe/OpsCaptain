package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

type FileLedger struct {
	inner *InMemoryLedger
	dir   string
}

func NewFileLedger(baseDir string) (*FileLedger, error) {
	dir := filepath.Join(baseDir, "ledger")
	for _, child := range []string{
		filepath.Join(dir, "tasks"),
		filepath.Join(dir, "results"),
		filepath.Join(dir, "traces"),
	} {
		if err := os.MkdirAll(child, 0o755); err != nil {
			return nil, err
		}
	}
	return &FileLedger{
		inner: NewInMemoryLedger(),
		dir:   dir,
	}, nil
}

func (l *FileLedger) CreateTask(ctx context.Context, task *protocol.TaskEnvelope) error {
	if err := l.inner.CreateTask(ctx, task); err != nil {
		return err
	}
	return writeJSON(filepath.Join(l.dir, "tasks", task.TaskID+".json"), task)
}

func (l *FileLedger) UpdateTaskStatus(ctx context.Context, taskID string, status protocol.TaskStatus) error {
	if err := l.inner.UpdateTaskStatus(ctx, taskID, status); err != nil {
		return err
	}
	l.inner.mu.RLock()
	task, ok := l.inner.tasks[taskID]
	l.inner.mu.RUnlock()
	if !ok {
		return nil
	}
	return writeJSON(filepath.Join(l.dir, "tasks", taskID+".json"), task)
}

func (l *FileLedger) AppendResult(ctx context.Context, taskID string, result *protocol.TaskResult) error {
	if err := l.inner.AppendResult(ctx, taskID, result); err != nil {
		return err
	}
	return writeJSON(filepath.Join(l.dir, "results", taskID+".json"), result)
}

func (l *FileLedger) AppendEvent(ctx context.Context, event *protocol.TaskEvent) error {
	traceFile := filepath.Join(l.dir, "traces", event.TraceID+".jsonl")
	f, err := os.OpenFile(traceFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(event)
}

func (l *FileLedger) EventsByTrace(ctx context.Context, traceID string) ([]*protocol.TaskEvent, error) {
	traceFile := filepath.Join(l.dir, "traces", traceID+".jsonl")
	f, err := os.Open(traceFile)
	if err != nil {
		if os.IsNotExist(err) {
			return l.inner.EventsByTrace(ctx, traceID)
		}
		return nil, err
	}
	defer f.Close()

	events := make([]*protocol.TaskEvent, 0, 16)
	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var event protocol.TaskEvent
			if err := json.Unmarshal(line, &event); err != nil {
				return nil, err
			}
			events = append(events, &event)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt < events[j].CreatedAt
	})
	return events, nil
}

func (l *FileLedger) ListChildren(ctx context.Context, parentTaskID string) ([]*protocol.TaskEnvelope, error) {
	return l.inner.ListChildren(ctx, parentTaskID)
}

type FileArtifactStore struct {
	dir string
}

func NewFileArtifactStore(baseDir string) (*FileArtifactStore, error) {
	dir := filepath.Join(baseDir, "artifacts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileArtifactStore{dir: dir}, nil
}

func (s *FileArtifactStore) Put(_ context.Context, artifact *protocol.Artifact) (*protocol.ArtifactRef, error) {
	if err := writeJSON(filepath.Join(s.dir, artifact.Ref.ID+".json"), artifact); err != nil {
		return nil, err
	}
	ref := artifact.Ref
	ref.URI = filepath.Join(s.dir, artifact.Ref.ID+".json")
	return &ref, nil
}

func (s *FileArtifactStore) Get(_ context.Context, ref *protocol.ArtifactRef) (*protocol.Artifact, error) {
	path := ref.URI
	if path == "" {
		path = filepath.Join(s.dir, ref.ID+".json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact protocol.Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func NewPersistent(baseDir string) (*Runtime, error) {
	if strings.TrimSpace(baseDir) == "" {
		return nil, fmt.Errorf("baseDir is empty")
	}
	ledger, err := NewFileLedger(baseDir)
	if err != nil {
		return nil, err
	}
	artifacts, err := NewFileArtifactStore(baseDir)
	if err != nil {
		return nil, err
	}
	return NewWithStores(ledger, NewLedgerBus(ledger), artifacts), nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
