package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/service"
	"context"
	"time"
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
	requests, err := service.ListApprovalRequests(ctx, req.Status)
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
	response, err := service.ApproveQueuedAIOpsRequest(ctx, req.RequestID)
	if err != nil {
		if fallback := userFacingAIOpsError(ctx, err); fallback != nil {
			return fallback, nil
		}
		return nil, err
	}

	result, detail := filterAssistantPayload(ctx, response.Content, response.Detail)
	return &v1.AIOpsRes{
		TraceID:           response.TraceID,
		Result:            result,
		Detail:            detail,
		ApprovalRequired:  response.ApprovalRequired,
		ApprovalRequestID: response.ApprovalRequestID,
		ApprovalStatus:    response.ApprovalStatus,
		ExecutionPlan:     append([]string{}, response.ExecutionPlan...),
		Degraded:          response.Degraded(),
		DegradationReason: response.DegradationReason,
	}, nil
}

func (c *ControllerV1) RejectApprovalRequest(ctx context.Context, req *v1.ApprovalRejectReq) (res *v1.ApprovalRequestItem, err error) {
	request, err := service.RejectQueuedAIOpsRequest(ctx, req.RequestID, req.Reason)
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
