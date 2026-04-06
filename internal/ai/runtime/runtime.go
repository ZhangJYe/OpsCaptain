package runtime

import (
	"SuperBizAgent/internal/consts"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/utility/metrics"
	traceutil "SuperBizAgent/utility/tracing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type Runtime struct {
	registry  *Registry
	ledger    Ledger
	bus       Bus
	artifacts ArtifactStore
}

const defaultAgentDispatchTimeout = 30 * time.Second

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

	persistCtx := context.WithoutCancel(ctx)

	if err := r.ledger.CreateTask(persistCtx, task); err != nil {
		return nil, err
	}

	agent, ok := r.registry.Get(task.Assignee)
	if !ok {
		return nil, fmt.Errorf("agent %q not registered", task.Assignee)
	}

	_ = r.ledger.UpdateTaskStatus(persistCtx, task.TaskID, protocol.TaskStatusRunning)
	_ = r.Publish(persistCtx, &protocol.TaskEvent{
		EventID:   uuid.NewString(),
		TaskID:    task.TaskID,
		TraceID:   task.TraceID,
		Type:      "task_started",
		Agent:     task.Assignee,
		Message:   fmt.Sprintf("%s started", task.Assignee),
		CreatedAt: nowMillis(),
	})

	startedAt := nowMillis()
	dispatchStarted := time.Now()
	runCtx := withTask(withRuntime(ctx, r), task)
	runCtx = context.WithValue(runCtx, consts.CtxKeySessionID, task.SessionID)
	runCtx, span := traceutil.StartSpan(
		runCtx,
		"runtime.dispatch",
		"agent."+task.Assignee,
		oteltrace.WithAttributes(
			attribute.String("agent.name", task.Assignee),
			attribute.String("session.id", task.SessionID),
			attribute.String("task.id", task.TaskID),
			attribute.String("task.trace_id", task.TraceID),
		),
	)
	defer span.End()
	dispatchTimeout := agentDispatchTimeout(ctx, task)
	result, err := r.executeAgent(runCtx, agent, task, dispatchTimeout)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(
			attribute.String("task.status", string(protocol.TaskStatusFailed)),
			attribute.String("result.status", string(protocol.ResultStatusFailed)),
		)
		failResult := failureResultForError(task, startedAt, err, dispatchTimeout)
		_ = r.ledger.AppendResult(persistCtx, task.TaskID, failResult)
		_ = r.ledger.UpdateTaskStatus(persistCtx, task.TaskID, protocol.TaskStatusFailed)
		if isDispatchTimeout(err) {
			_ = r.Publish(persistCtx, &protocol.TaskEvent{
				EventID: uuid.NewString(),
				TaskID:  task.TaskID,
				TraceID: task.TraceID,
				Type:    "task_timeout",
				Agent:   task.Assignee,
				Message: failResult.Summary,
				Payload: map[string]any{
					"timeout_ms": dispatchTimeout.Milliseconds(),
				},
				CreatedAt: nowMillis(),
			})
		}
		_ = r.Publish(persistCtx, &protocol.TaskEvent{
			EventID: uuid.NewString(),
			TaskID:  task.TaskID,
			TraceID: task.TraceID,
			Type:    "task_failed",
			Agent:   task.Assignee,
			Message: failResult.Summary,
			Payload: map[string]any{
				"error_code":  failResult.Error.Code,
				"error_text":  failResult.Error.Message,
				"result_type": failResult.Status,
				"timeout_ms":  dispatchTimeout.Milliseconds(),
			},
			CreatedAt: nowMillis(),
		})
		metrics.ObserveAgentDispatch(task.Assignee, string(protocol.ResultStatusFailed), time.Since(dispatchStarted))
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
	result = normalizeTaskResult(task, result, startedAt)
	span.SetAttributes(
		attribute.String("task.status", string(taskStatusForResult(result.Status))),
		attribute.String("result.status", string(result.Status)),
	)
	if err := r.ledger.AppendResult(persistCtx, task.TaskID, result); err != nil {
		return nil, err
	}

	_ = r.ledger.UpdateTaskStatus(persistCtx, task.TaskID, taskStatusForResult(result.Status))
	payload := map[string]any{
		"status":          result.Status,
		"summary_length":  len(strings.TrimSpace(result.Summary)),
		"summary_omitted": shouldOmitSummary(result.Summary),
	}
	if strings.TrimSpace(result.DegradationReason) != "" {
		payload["degradation_reason"] = result.DegradationReason
	}
	_ = r.Publish(persistCtx, &protocol.TaskEvent{
		EventID:   uuid.NewString(),
		TaskID:    task.TaskID,
		TraceID:   task.TraceID,
		Type:      "task_completed",
		Agent:     task.Assignee,
		Message:   taskCompletionMessage(result),
		Payload:   payload,
		CreatedAt: nowMillis(),
	})
	metrics.ObserveAgentDispatch(task.Assignee, string(result.Status), time.Since(dispatchStarted))
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
	case "task_started", "task_info", "task_failed", "task_timeout", "task_completed":
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

func (r *Runtime) executeAgent(ctx context.Context, agent Agent, task *protocol.TaskEnvelope, timeout time.Duration) (*protocol.TaskResult, error) {
	if timeout <= 0 {
		return agent.Handle(ctx, task)
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type agentResult struct {
		result *protocol.TaskResult
		err    error
	}

	done := make(chan agentResult, 1)
	go func() {
		result, err := agent.Handle(runCtx, task)
		done <- agentResult{result: result, err: err}
	}()

	select {
	case out := <-done:
		return out.result, out.err
	case <-runCtx.Done():
		return nil, runCtx.Err()
	}
}

func agentDispatchTimeout(ctx context.Context, task *protocol.TaskEnvelope) time.Duration {
	if task != nil && task.Constraints != nil {
		if timeout, ok := parseTimeoutMillis(task.Constraints["timeout_ms"]); ok {
			return timeout
		}
	}

	v, err := g.Cfg().Get(ctx, "multi_agent.agent_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultAgentDispatchTimeout
}

func parseTimeoutMillis(raw any) (time.Duration, bool) {
	switch typed := raw.(type) {
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond, true
		}
	case int32:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond, true
		}
	case int64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond, true
		}
	case float32:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond, true
		}
	case float64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond, true
		}
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil && n > 0 {
			return time.Duration(n) * time.Millisecond, true
		}
	}
	return 0, false
}

func failureResultForError(task *protocol.TaskEnvelope, startedAt int64, err error, timeout time.Duration) *protocol.TaskResult {
	summary := err.Error()
	code := "dispatch_failed"
	if isDispatchTimeout(err) {
		summary = fmt.Sprintf("%s timed out after %dms", task.Assignee, timeout.Milliseconds())
		code = "timeout"
	} else if errors.Is(err, context.Canceled) {
		summary = fmt.Sprintf("%s was cancelled", task.Assignee)
		code = "cancelled"
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      task.Assignee,
		Status:     protocol.ResultStatusFailed,
		Summary:    summary,
		Confidence: 0,
		Error: &protocol.TaskError{
			Code:    code,
			Message: summary,
		},
		StartedAt:  startedAt,
		FinishedAt: nowMillis(),
	}
}

func normalizeTaskResult(task *protocol.TaskEnvelope, result *protocol.TaskResult, startedAt int64) *protocol.TaskResult {
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
	if result.Status == protocol.ResultStatusDegraded && strings.TrimSpace(result.DegradationReason) == "" {
		result.DegradationReason = deriveDegradationReason(result)
	}
	return result
}

func deriveDegradationReason(result *protocol.TaskResult) string {
	if result == nil {
		return ""
	}
	if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
		return strings.TrimSpace(result.Error.Message)
	}
	if result.Metadata != nil {
		if reason, ok := result.Metadata["degradation_reason"].(string); ok && strings.TrimSpace(reason) != "" {
			return strings.TrimSpace(reason)
		}
	}
	return previewSummary(result.Summary, 200)
}

func taskStatusForResult(status protocol.ResultStatus) protocol.TaskStatus {
	if status == protocol.ResultStatusFailed {
		return protocol.TaskStatusFailed
	}
	return protocol.TaskStatusSucceeded
}

func isDispatchTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
