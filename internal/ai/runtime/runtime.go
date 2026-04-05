package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"

	"github.com/google/uuid"
)

type Runtime struct {
	registry  *Registry
	ledger    Ledger
	bus       Bus
	artifacts ArtifactStore
}

func New() *Runtime {
	ledger := NewInMemoryLedger()
	return NewWithStores(ledger, NewLedgerBus(ledger), NewInMemoryArtifactStore())
}

func NewWithStores(ledger Ledger, bus Bus, artifacts ArtifactStore) *Runtime {
	return &Runtime{
		registry:  NewRegistry(),
		ledger:    ledger,
		bus:       bus,
		artifacts: artifacts,
	}
}

func (r *Runtime) Register(agent Agent) error {
	return r.registry.Register(agent)
}

func (r *Runtime) Dispatch(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	if task.TaskID == "" {
		task.TaskID = uuid.NewString()
	}
	if task.TraceID == "" {
		task.TraceID = uuid.NewString()
	}
	if task.CreatedAt == 0 {
		task.CreatedAt = nowMillis()
	}
	task.UpdatedAt = nowMillis()
	if task.Status == "" {
		task.Status = protocol.TaskStatusPending
	}

	if err := r.ledger.CreateTask(ctx, task); err != nil {
		return nil, err
	}

	agent, ok := r.registry.Get(task.Assignee)
	if !ok {
		return nil, fmt.Errorf("agent %q not registered", task.Assignee)
	}

	_ = r.ledger.UpdateTaskStatus(ctx, task.TaskID, protocol.TaskStatusRunning)
	_ = r.Publish(ctx, &protocol.TaskEvent{
		EventID:   uuid.NewString(),
		TaskID:    task.TaskID,
		TraceID:   task.TraceID,
		Type:      "task_started",
		Agent:     task.Assignee,
		Message:   fmt.Sprintf("%s started", task.Assignee),
		CreatedAt: nowMillis(),
	})

	startedAt := nowMillis()
	runCtx := withRuntime(ctx, r)
	result, err := agent.Handle(runCtx, task)
	if err != nil {
		failResult := &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      task.Assignee,
			Status:     protocol.ResultStatusFailed,
			Summary:    err.Error(),
			Confidence: 0,
			Error: &protocol.TaskError{
				Message: err.Error(),
			},
			StartedAt:  startedAt,
			FinishedAt: nowMillis(),
		}
		_ = r.ledger.AppendResult(ctx, task.TaskID, failResult)
		_ = r.ledger.UpdateTaskStatus(ctx, task.TaskID, protocol.TaskStatusFailed)
		_ = r.Publish(ctx, &protocol.TaskEvent{
			EventID:   uuid.NewString(),
			TaskID:    task.TaskID,
			TraceID:   task.TraceID,
			Type:      "task_failed",
			Agent:     task.Assignee,
			Message:   err.Error(),
			CreatedAt: nowMillis(),
		})
		return failResult, err
	}

	if result == nil {
		result = &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      task.Assignee,
			Status:     protocol.ResultStatusSucceeded,
			StartedAt:  startedAt,
			FinishedAt: nowMillis(),
		}
	}
	if result.TaskID == "" {
		result.TaskID = task.TaskID
	}
	if result.Agent == "" {
		result.Agent = task.Assignee
	}
	if result.StartedAt == 0 {
		result.StartedAt = startedAt
	}
	if result.FinishedAt == 0 {
		result.FinishedAt = nowMillis()
	}
	if result.Status == "" {
		result.Status = protocol.ResultStatusSucceeded
	}
	if err := r.ledger.AppendResult(ctx, task.TaskID, result); err != nil {
		return nil, err
	}

	finalStatus := protocol.TaskStatusSucceeded
	if result.Status == protocol.ResultStatusFailed {
		finalStatus = protocol.TaskStatusFailed
	}
	_ = r.ledger.UpdateTaskStatus(ctx, task.TaskID, finalStatus)
	_ = r.Publish(ctx, &protocol.TaskEvent{
		EventID: uuid.NewString(),
		TaskID:  task.TaskID,
		TraceID: task.TraceID,
		Type:    "task_completed",
		Agent:   task.Assignee,
		Message: taskCompletionMessage(result),
		Payload: map[string]any{
			"status":          result.Status,
			"summary_length":  len(strings.TrimSpace(result.Summary)),
			"summary_omitted": shouldOmitSummary(result.Summary),
		},
		CreatedAt: nowMillis(),
	})
	return result, nil
}

func (r *Runtime) Publish(ctx context.Context, event *protocol.TaskEvent) error {
	return r.bus.Publish(ctx, event)
}

func (r *Runtime) EmitInfo(ctx context.Context, task *protocol.TaskEnvelope, agentName, message string, payload map[string]any) {
	_ = r.Publish(ctx, &protocol.TaskEvent{
		EventID:   uuid.NewString(),
		TaskID:    task.TaskID,
		TraceID:   task.TraceID,
		Type:      "task_info",
		Agent:     agentName,
		Message:   message,
		Payload:   payload,
		CreatedAt: nowMillis(),
	})
}

func (r *Runtime) CreateArtifact(ctx context.Context, artifactType, content string, metadata map[string]any) (*protocol.ArtifactRef, error) {
	artifact := &protocol.Artifact{
		Ref: protocol.ArtifactRef{
			ID:   uuid.NewString(),
			Type: artifactType,
		},
		Content:   content,
		Metadata:  metadata,
		CreatedAt: nowMillis(),
	}
	return r.artifacts.Put(ctx, artifact)
}

func (r *Runtime) DetailMessages(ctx context.Context, traceID string) []string {
	events, err := r.TraceEvents(ctx, traceID)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(events))
	for _, event := range events {
		if event.Message == "" || !includeDetailEvent(event.Type) {
			continue
		}
		out = append(out, fmt.Sprintf("[%s] %s", event.Agent, event.Message))
	}
	return out
}

func (r *Runtime) TraceEvents(ctx context.Context, traceID string) ([]*protocol.TaskEvent, error) {
	return r.ledger.EventsByTrace(ctx, traceID)
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}

func includeDetailEvent(eventType string) bool {
	switch eventType {
	case "task_started", "task_info", "task_failed", "task_completed":
		return true
	default:
		return false
	}
}

func taskCompletionMessage(result *protocol.TaskResult) string {
	if result == nil {
		return "任务已完成。"
	}
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		return statusSummary(result.Status)
	}
	if shouldOmitSummary(summary) {
		return statusSummary(result.Status) + " 详细摘要已折叠。"
	}
	return previewSummary(summary, 160)
}

func shouldOmitSummary(summary string) bool {
	summary = strings.TrimSpace(summary)
	return strings.Contains(summary, "\n") || len(summary) > 180
}

func statusSummary(status protocol.ResultStatus) string {
	switch status {
	case protocol.ResultStatusDegraded:
		return "任务已降级完成。"
	case protocol.ResultStatusFailed:
		return "任务执行失败。"
	default:
		return "任务已完成。"
	}
}

func previewSummary(summary string, max int) string {
	summary = strings.Join(strings.Fields(strings.TrimSpace(summary)), " ")
	if max <= 0 || len(summary) <= max {
		return summary
	}
	return summary[:max] + "..."
}
