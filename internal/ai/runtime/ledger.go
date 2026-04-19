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
	mu          sync.RWMutex
	tasks       map[string]*protocol.TaskEnvelope
	taskOrder   []string
	results     map[string]*protocol.TaskResult
	resultOrder []string
	events      []*protocol.TaskEvent
	maxTasks    int
	maxResults  int
	maxEvents   int
}

func NewInMemoryLedger() *InMemoryLedger {
	return NewInMemoryLedgerWithLimits(20000, 20000, 50000)
}

func NewInMemoryLedgerWithLimits(maxTasks, maxResults, maxEvents int) *InMemoryLedger {
	if maxTasks <= 0 {
		maxTasks = 20000
	}
	if maxResults <= 0 {
		maxResults = 20000
	}
	if maxEvents <= 0 {
		maxEvents = 50000
	}
	return &InMemoryLedger{
		tasks:       make(map[string]*protocol.TaskEnvelope),
		taskOrder:   make([]string, 0, 256),
		results:     make(map[string]*protocol.TaskResult),
		resultOrder: make([]string, 0, 256),
		events:      make([]*protocol.TaskEvent, 0, 32),
		maxTasks:    maxTasks,
		maxResults:  maxResults,
		maxEvents:   maxEvents,
	}
}

func (l *InMemoryLedger) CreateTask(_ context.Context, task *protocol.TaskEnvelope) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := *task
	if _, ok := l.tasks[task.TaskID]; !ok {
		l.taskOrder = append(l.taskOrder, task.TaskID)
	}
	l.tasks[task.TaskID] = &cp
	l.pruneTasksLocked()
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
	if _, ok := l.results[taskID]; !ok {
		l.resultOrder = append(l.resultOrder, taskID)
	}
	l.results[taskID] = &cp
	l.pruneResultsLocked()
	return nil
}

func (l *InMemoryLedger) AppendEvent(_ context.Context, event *protocol.TaskEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := *event
	l.events = append(l.events, &cp)
	l.pruneEventsLocked()
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

func (l *InMemoryLedger) pruneTasksLocked() {
	if l.maxTasks <= 0 {
		return
	}
	for len(l.taskOrder) > l.maxTasks {
		oldest := l.taskOrder[0]
		l.taskOrder = l.taskOrder[1:]
		delete(l.tasks, oldest)
	}
}

func (l *InMemoryLedger) pruneResultsLocked() {
	if l.maxResults <= 0 {
		return
	}
	for len(l.resultOrder) > l.maxResults {
		oldest := l.resultOrder[0]
		l.resultOrder = l.resultOrder[1:]
		delete(l.results, oldest)
	}
}

func (l *InMemoryLedger) pruneEventsLocked() {
	if l.maxEvents <= 0 {
		return
	}
	if len(l.events) <= l.maxEvents {
		return
	}
	overflow := len(l.events) - l.maxEvents
	l.events = append([]*protocol.TaskEvent(nil), l.events[overflow:]...)
}
