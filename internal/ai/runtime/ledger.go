package runtime

import (
	"context"
	"sort"
	"sync"

	"SuperBizAgent/internal/ai/protocol"
)

type Ledger interface {
	CreateTask(ctx context.Context, task *protocol.TaskEnvelope) error
	UpdateTaskStatus(ctx context.Context, taskID string, status protocol.TaskStatus) error
	AppendResult(ctx context.Context, taskID string, result *protocol.TaskResult) error
	AppendEvent(ctx context.Context, event *protocol.TaskEvent) error
	EventsByTrace(ctx context.Context, traceID string) ([]*protocol.TaskEvent, error)
	ListChildren(ctx context.Context, parentTaskID string) ([]*protocol.TaskEnvelope, error)
}

type InMemoryLedger struct {
	mu      sync.RWMutex
	tasks   map[string]*protocol.TaskEnvelope
	results map[string]*protocol.TaskResult
	events  []*protocol.TaskEvent
}

func NewInMemoryLedger() *InMemoryLedger {
	return &InMemoryLedger{
		tasks:   make(map[string]*protocol.TaskEnvelope),
		results: make(map[string]*protocol.TaskResult),
		events:  make([]*protocol.TaskEvent, 0, 32),
	}
}

func (l *InMemoryLedger) CreateTask(_ context.Context, task *protocol.TaskEnvelope) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := *task
	l.tasks[task.TaskID] = &cp
	return nil
}

func (l *InMemoryLedger) UpdateTaskStatus(_ context.Context, taskID string, status protocol.TaskStatus) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if task, ok := l.tasks[taskID]; ok {
		task.Status = status
		task.UpdatedAt = nowMillis()
	}
	return nil
}

func (l *InMemoryLedger) AppendResult(_ context.Context, taskID string, result *protocol.TaskResult) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := *result
	l.results[taskID] = &cp
	return nil
}

func (l *InMemoryLedger) AppendEvent(_ context.Context, event *protocol.TaskEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := *event
	l.events = append(l.events, &cp)
	return nil
}

func (l *InMemoryLedger) EventsByTrace(_ context.Context, traceID string) ([]*protocol.TaskEvent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*protocol.TaskEvent, 0)
	for _, event := range l.events {
		if event.TraceID == traceID {
			cp := *event
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out, nil
}

func (l *InMemoryLedger) ListChildren(_ context.Context, parentTaskID string) ([]*protocol.TaskEnvelope, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*protocol.TaskEnvelope, 0)
	for _, task := range l.tasks {
		if task.ParentTaskID == parentTaskID {
			cp := *task
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out, nil
}
