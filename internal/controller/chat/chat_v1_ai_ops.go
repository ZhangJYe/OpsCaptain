package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/consts"
	"context"
	"errors"

	"github.com/gogf/gf/v2/util/guid"
)

var runAIOpsMultiAgent = service.RunAIOpsMultiAgent

func (c *ControllerV1) AIOps(ctx context.Context, req *v1.AIOpsReq) (res *v1.AIOpsRes, err error) {
	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = enrichRequestContext(ctx, "", requestID)

	if err := rejectSuspiciousPrompt(ctx, req.Query); err != nil {
		return nil, err
	}
	if decision := getDegradationDecision(ctx, "ai_ops"); decision.Enabled {
		return &v1.AIOpsRes{
			Result:            decision.Message,
			Detail:            []string{decision.Reason},
			Degraded:          true,
			DegradationReason: decision.Reason,
		}, nil
	}

	query := req.Query
	if query == "" {
		query = `You are an AIOps incident assistant. Follow this order:
1. Query active Prometheus alerts.
2. For each alert, look up the matching internal docs or runbook.
3. Use only tool results and internal docs for analysis.
4. If a tool fails, skip that step and call it out once in the report.
5. Produce a markdown report with sections: Active Alerts, Root Cause Analysis, Mitigation, Conclusion.`
	}

	response, err := runAIOpsMultiAgent(ctx, query)
	if err != nil {
		if fallback := userFacingAIOpsError(ctx, err); fallback != nil {
			return fallback, nil
		}
		return nil, err
	}

	result := response.Content
	if result == "" {
		if len(response.Detail) > 0 && response.Detail[0] != "" {
			result = response.Detail[0]
		} else {
			return nil, errors.New("internal error")
		}
	}
	result, detail := filterAssistantPayload(ctx, result, response.Detail)
	executionPlan := make([]string, 0, len(response.ExecutionPlan))
	for _, step := range response.ExecutionPlan {
		filtered, _ := filterAssistantPayload(ctx, step, nil)
		executionPlan = append(executionPlan, filtered)
	}

	return &v1.AIOpsRes{
		TraceID:           response.TraceID,
		Result:            result,
		Detail:            detail,
		ApprovalRequired:  response.ApprovalRequired,
		ApprovalRequestID: response.ApprovalRequestID,
		ApprovalStatus:    response.ApprovalStatus,
		ExecutionPlan:     executionPlan,
		Degraded:          response.Degraded(),
		DegradationReason: response.DegradationReason,
	}, nil
}

func (c *ControllerV1) AIOpsTrace(ctx context.Context, req *v1.AIOpsTraceReq) (res *v1.AIOpsTraceRes, err error) {
	events, detail, err := service.GetAIOpsTrace(ctx, req.TraceID)
	if err != nil {
		return nil, err
	}

	out := make([]v1.AIOpsTraceEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		out = append(out, v1.AIOpsTraceEvent{
			EventID:   event.EventID,
			TaskID:    event.TaskID,
			TraceID:   event.TraceID,
			Type:      event.Type,
			Agent:     event.Agent,
			Message:   event.Message,
			Payload:   event.Payload,
			CreatedAt: event.CreatedAt,
		})
	}

	return &v1.AIOpsTraceRes{
		TraceID: req.TraceID,
		Detail:  detail,
		Events:  out,
	}, nil
}
