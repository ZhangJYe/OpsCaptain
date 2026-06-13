package service

import (
	"context"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/consts"

	"github.com/google/uuid"
)

func startIncidentTurn(ctx context.Context, incidentID, turnID string) {
	runCtx := context.WithoutCancel(ctx)
	startIncidentRun(func() {
		executeIncidentTurn(runCtx, incidentID, turnID)
	})
}

func executeIncidentTurn(ctx context.Context, incidentID, turnID string) {
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return
	}
	incident, err := store.Get(ctx, incidentID)
	if err != nil {
		return
	}
	turn, ok := incidentTurnByID(incident, turnID)
	if !ok {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, incidentTurnTimeout(ctx))
	defer cancel()
	runCtx = context.WithValue(runCtx, consts.CtxKeySessionID, incident.SessionID)
	runCtx = WithAIOpsEngine(runCtx, incident.EngineStrategy)
	runCtx = WithAIOpsIncidentContext(runCtx, incidentContext(ctx, incident, turnID))
	runCtx = withIncidentEventSink(runCtx, store, incidentID, turnID)
	response, runErr := runIncidentAIOps(runCtx, turn.UserQuery)
	_, _ = store.Update(context.Background(), incidentID, func(current *IncidentSession) error {
		currentTurn, ok := incidentTurnPointer(current, turnID)
		if !ok {
			return nil
		}
		status := incidentStatusForResponse(response, runErr)
		currentTurn.Status = status
		currentTurn.TraceID = firstNonEmptyIncident(response.TraceID, currentTurn.TraceID)
		currentTurn.Result = strings.TrimSpace(response.Content)
		currentTurn.Detail = append([]string{}, response.Detail...)
		currentTurn.Engine = strings.TrimSpace(response.Engine)
		currentTurn.ApprovalRequestID = strings.TrimSpace(response.ApprovalRequestID)
		currentTurn.ApprovalStatus = strings.TrimSpace(response.ApprovalStatus)
		currentTurn.DegradationReason = strings.TrimSpace(response.DegradationReason)
		if runErr != nil && currentTurn.Result == "" {
			currentTurn.Result = runErr.Error()
		}
		if status != IncidentStatusWaitingApproval {
			currentTurn.FinishedAt = time.Now().UnixMilli()
		}
		current.Status = status
		if currentTurn.Result != "" {
			current.LatestSummary = currentTurn.Result
		}
		current.Events = append(current.Events, incidentCompletionEvent(current.IncidentID, currentTurn, runErr))
		return nil
	})
}

func withIncidentApprovalRun(ctx context.Context, requestID string) context.Context {
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return ctx
	}
	incidents, err := store.List(ctx)
	if err != nil {
		return ctx
	}
	for idx := range incidents {
		incident := &incidents[idx]
		for _, turn := range incident.Turns {
			if turn.ApprovalRequestID != requestID {
				continue
			}
			runCtx := WithAIOpsEngine(ctx, incident.EngineStrategy)
			runCtx = WithAIOpsIncidentContext(runCtx, incidentContext(ctx, incident, turn.TurnID))
			return withIncidentEventSink(runCtx, store, incident.IncidentID, turn.TurnID)
		}
	}
	return ctx
}

func withIncidentEventSink(ctx context.Context, store IncidentStore, incidentID, turnID string) context.Context {
	return runtime.WithTaskEventSink(ctx, func(event *protocol.TaskEvent) {
		if event == nil {
			return
		}
		_, _ = store.Update(context.Background(), incidentID, func(current *IncidentSession) error {
			current.Events = append(current.Events, IncidentEvent{
				EventID:    event.EventID,
				IncidentID: current.IncidentID,
				TurnID:     turnID,
				TraceID:    event.TraceID,
				Type:       event.Type,
				Agent:      event.Agent,
				Message:    event.Message,
				Payload:    clonePayload(event.Payload),
				CreatedAt:  event.CreatedAt,
			})
			if existing, ok := incidentTurnPointer(current, turnID); ok && existing.TraceID == "" {
				existing.TraceID = strings.TrimSpace(event.TraceID)
			}
			return nil
		})
	})
}

func updateIncidentApproval(ctx context.Context, requestID string, update func(*IncidentSession, *IncidentTurn)) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return err
	}
	incidents, err := store.List(ctx)
	if err != nil {
		return err
	}
	for _, incident := range incidents {
		found := false
		for _, turn := range incident.Turns {
			if turn.ApprovalRequestID == requestID {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		_, err := store.Update(ctx, incident.IncidentID, func(current *IncidentSession) error {
			for idx := range current.Turns {
				if current.Turns[idx].ApprovalRequestID != requestID {
					continue
				}
				update(current, &current.Turns[idx])
				return nil
			}
			return nil
		})
		return err
	}
	return nil
}

func incidentCompletionEvent(incidentID string, turn *IncidentTurn, err error) IncidentEvent {
	eventType := "turn_completed"
	message := "排障轮次已完成"
	if turn.Status == IncidentStatusWaitingApproval {
		eventType = "approval_waiting"
		message = "排障已暂停，等待审批"
	} else if turn.Status == IncidentStatusDegraded {
		eventType = "turn_degraded"
		message = "排障轮次降级完成"
	} else if turn.Status == IncidentStatusFailed || err != nil {
		eventType = "turn_failed"
		message = "排障轮次执行失败"
	}
	return newIncidentEvent(incidentID, turn.TurnID, turn.TraceID, eventType, turn.Engine, message, map[string]any{
		"status":              turn.Status,
		"approval_request_id": turn.ApprovalRequestID,
		"degradation_reason":  turn.DegradationReason,
	})
}

func newIncidentEvent(incidentID, turnID, traceID, eventType, agent, message string, payload map[string]any) IncidentEvent {
	return IncidentEvent{
		EventID:    uuid.NewString(),
		IncidentID: incidentID,
		TurnID:     turnID,
		TraceID:    traceID,
		Type:       eventType,
		Agent:      agent,
		Message:    strings.TrimSpace(message),
		Payload:    clonePayload(payload),
		CreatedAt:  time.Now().UnixMilli(),
	}
}

func clonePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}
