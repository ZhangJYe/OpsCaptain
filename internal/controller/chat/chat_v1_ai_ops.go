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
		query = `你是一个 AIOps 事故分析助手，请严格按以下顺序执行：
1. 查询当前活跃的 Prometheus 告警。
2. 对每条告警查询匹配的内部文档或 runbook。
3. 只能基于工具结果和内部文档进行分析。
4. 如果某个工具失败，跳过该步骤，并在报告中明确说明一次。
5. 默认使用中文输出报告，除非用户明确要求其他语言。
6. 报告使用 Markdown，包含这些章节：活跃告警、根因分析、缓解建议、结论。`
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
