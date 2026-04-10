package service

import (
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
	"SuperBizAgent/internal/consts"
	"context"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/agent/plan_execute_replan"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/skills"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

var (
	newApprovalQueue = func() *ApprovalQueue {
		return NewApprovalQueue()
	}
	newMemoryService = func() aiOpsMemory {
		return NewMemoryService()
	}
	buildPlanAgent = plan_execute_replan.BuildPlanAgent

	skillFocusCollector = skills.NewFocusCollector(
		logs.SkillRegistry(),
		metrics.SkillRegistry(),
		knowledge.SkillRegistry(),
	)
)

type aiOpsMemory interface {
	ResolveSessionID(ctx context.Context) string
	BuildContextPlan(ctx context.Context, mode, sessionID, query string) (string, []protocol.MemoryRef, []string)
	PersistOutcome(ctx context.Context, sessionID, query, summary string)
}

func RunAIOpsMultiAgent(ctx context.Context, query string) (ExecutionResponse, error) {
	approval := NewApprovalGate()
	if decision := approval.Check(ctx, query); !decision.Approved {
		if decision.Queued && decision.ApprovalRequest != nil {
			return ExecutionResponse{
				Content:           decision.Reason,
				Detail:            []string{decision.Reason},
				Status:            protocol.ResultStatusSucceeded,
				ApprovalRequired:  true,
				ApprovalRequestID: decision.ApprovalRequest.ID,
				ApprovalStatus:    string(decision.ApprovalRequest.Status),
				ExecutionPlan:     append([]string{}, decision.ApprovalRequest.ExecutionPlan...),
			}, nil
		}
		return ExecutionResponse{
			Content: decision.Reason,
			Detail:  []string{decision.Reason},
			Status:  protocol.ResultStatusSucceeded,
		}, nil
	}

	if decision := GetDegradationDecision(ctx, "ai_ops"); decision.Enabled {
		return NewDegradedExecutionResponse(decision), nil
	}

	traceID := uuid.NewString()

	memorySvc := newMemoryService()
	sessionID := memorySvc.ResolveSessionID(ctx)
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, sessionID)
	memoryContext, _, contextDetail := memorySvc.BuildContextPlan(ctx, "aiops", sessionID, query)

	enrichedQuery := query
	if strings.TrimSpace(memoryContext) != "" {
		enrichedQuery = query + "\n\n可参考的历史上下文：\n" + memoryContext
	}

	if hints := skillFocusCollector.Collect(query); len(hints) > 0 {
		enrichedQuery = enrichedQuery + "\n\n场景分析方向（基于 Skill 匹配）：\n" + skills.FormatFocusHints(hints)
		g.Log().Infof(ctx, "[AIOps] skill focus injected: %d hints", len(hints))
	}

	g.Log().Infof(ctx, "[AIOps] plan-execute-replan started, trace_id=%s", traceID)
	content, planDetail, err := buildPlanAgent(ctx, enrichedQuery)
	if err != nil {
		g.Log().Errorf(ctx, "[AIOps] plan-execute-replan failed: %v", err)
		return ExecutionResponse{
			Detail:            append(contextDetail, fmt.Sprintf("plan-execute-replan error: %v", err)),
			TraceID:           traceID,
			Status:            protocol.ResultStatusFailed,
			DegradationReason: err.Error(),
		}, err
	}

	detail := append([]string{}, contextDetail...)
	detail = append(detail, planDetail...)

	memorySvc.PersistOutcome(ctx, sessionID, query, content)
	g.Log().Infof(ctx, "[AIOps] plan-execute-replan completed, trace_id=%s, steps=%d", traceID, len(planDetail))

	return ExecutionResponse{
		Content: content,
		Detail:  detail,
		TraceID: traceID,
		Status:  protocol.ResultStatusSucceeded,
	}, nil
}

func ListApprovalRequests(ctx context.Context, status string) ([]ApprovalRequest, error) {
	return newApprovalQueue().List(ctx, parseApprovalStatus(status))
}

func RejectQueuedAIOpsRequest(ctx context.Context, requestID, reviewReason string) (*ApprovalRequest, error) {
	return newApprovalQueue().Reject(ctx, requestID, reviewerIdentity(ctx), reviewReason)
}

func ApproveQueuedAIOpsRequest(ctx context.Context, requestID string) (ExecutionResponse, error) {
	queue := newApprovalQueue()
	request, err := queue.Approve(ctx, requestID, reviewerIdentity(ctx))
	if err != nil {
		return ExecutionResponse{}, err
	}

	runCtx := context.WithValue(ctx, consts.CtxKeyApprovalBypass, true)
	runCtx = context.WithValue(runCtx, consts.CtxKeyApprovalRequestID, requestID)
	if request.SessionID != "" {
		runCtx = context.WithValue(runCtx, consts.CtxKeySessionID, request.SessionID)
	}

	response, err := RunAIOpsMultiAgent(runCtx, request.Query)
	if err != nil {
		return response, err
	}
	response.ApprovalRequestID = requestID
	response.ApprovalStatus = string(ApprovalStatusApproved)
	if response.TraceID != "" {
		if markErr := queue.MarkExecuted(ctx, requestID, response.TraceID); markErr == nil {
			response.ApprovalStatus = string(ApprovalStatusExecuted)
		}
	}
	return response, nil
}

func GetAIOpsTrace(_ context.Context, traceID string) ([]*protocol.TaskEvent, []string, error) {
	if traceID == "" {
		return nil, nil, fmt.Errorf("traceID is empty")
	}
	return nil, nil, fmt.Errorf("trace %s not found: plan-execute-replan traces are recorded in application logs", traceID)
}

func reviewerIdentity(ctx context.Context) string {
	if userID, ok := ctx.Value(consts.CtxKeyUserID).(string); ok && userID != "" {
		return userID
	}
	return "system"
}

func parseApprovalStatus(status string) ApprovalStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(ApprovalStatusApproved):
		return ApprovalStatusApproved
	case string(ApprovalStatusRejected):
		return ApprovalStatusRejected
	case string(ApprovalStatusExecuted):
		return ApprovalStatusExecuted
	default:
		return ApprovalStatusPending
	}
}
