package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/service"
	"context"
	"errors"
	"time"
)

var (
	listApprovalRequests      = service.ListApprovalRequests
	approveQueuedAIOpsRequest = service.ApproveQueuedAIOpsRequest
	rejectQueuedAIOpsRequest  = service.RejectQueuedAIOpsRequest
)

func (c *ControllerV1) TokenAudit(ctx context.Context, req *v1.TokenAuditReq) (res *v1.TokenAuditRes, err error) {
	queryDate := time.Now()
	if req.Date != "" {
		queryDate, err = time.Parse("2006-01-02", req.Date)
		if err != nil {
			return nil, err
		}
	}

	audit, err := service.GetSessionTokenAudit(ctx, req.SessionID, queryDate)
	if err != nil {
		return nil, err
	}
	return &v1.TokenAuditRes{
		Date:             audit.Date,
		SessionID:        audit.SessionID,
		UserID:           audit.UserID,
		PromptTokens:     audit.PromptTokens,
		CompletionTokens: audit.CompletionTokens,
		TotalTokens:      audit.TotalTokens,
		Calls:            audit.Calls,
		LastModel:        audit.LastModel,
		UpdatedAt:        audit.UpdatedAt,
	}, nil
}

func (c *ControllerV1) ApprovalRequests(ctx context.Context, req *v1.ApprovalRequestsReq) (res *v1.ApprovalRequestsRes, err error) {
	requests, err := listApprovalRequests(ctx, req.Status)
	if err != nil {
		return nil, err
	}
	items := make([]v1.ApprovalRequestItem, 0, len(requests))
	for _, request := range requests {
		items = append(items, toApprovalRequestItem(request))
	}
	return &v1.ApprovalRequestsRes{Items: items}, nil
}

func (c *ControllerV1) ApproveApprovalRequest(ctx context.Context, req *v1.ApprovalActionReq) (res *v1.AIOpsRes, err error) {
	response, err := approveQueuedAIOpsRequest(ctx, req.RequestID)
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

func (c *ControllerV1) RejectApprovalRequest(ctx context.Context, req *v1.ApprovalRejectReq) (res *v1.ApprovalRequestItem, err error) {
	request, err := rejectQueuedAIOpsRequest(ctx, req.RequestID, req.Reason)
	if err != nil {
		return nil, err
	}
	item := toApprovalRequestItem(*request)
	return &item, nil
}

func toApprovalRequestItem(request service.ApprovalRequest) v1.ApprovalRequestItem {
	return v1.ApprovalRequestItem{
		ID:            request.ID,
		Query:         request.Query,
		Reason:        request.Reason,
		Status:        string(request.Status),
		SessionID:     request.SessionID,
		UserID:        request.UserID,
		RequestedBy:   request.RequestedBy,
		ReviewedBy:    request.ReviewedBy,
		ReviewReason:  request.ReviewReason,
		ExecutionPlan: append([]string{}, request.ExecutionPlan...),
		ResultTraceID: request.ResultTraceID,
		CreatedAt:     request.CreatedAt,
		UpdatedAt:     request.UpdatedAt,
		ApprovedAt:    request.ApprovedAt,
		RejectedAt:    request.RejectedAt,
		ExecutionAt:   request.ExecutionAt,
	}
}
