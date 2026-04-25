package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/service"
	"SuperBizAgent/utility/mem"
	"context"
	"errors"
	"fmt"
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

func (c *ControllerV1) MemoryList(ctx context.Context, req *v1.MemoryListReq) (res *v1.MemoryListRes, err error) {
	items := service.NewMemoryService().ListMemories(ctx, service.MemoryListOptions{
		SessionID:      req.SessionID,
		UserID:         req.UserID,
		ProjectID:      req.ProjectID,
		IncludeExpired: req.IncludeExpired,
	})
	result := make([]v1.MemoryItem, 0, len(items))
	for _, item := range items {
		result = append(result, toMemoryItem(item))
	}
	return &v1.MemoryListRes{Items: result}, nil
}

func (c *ControllerV1) MemoryAction(ctx context.Context, req *v1.MemoryActionReq) (res *v1.MemoryActionRes, err error) {
	svc := service.NewMemoryService()
	switch req.Action {
	case "delete":
		return &v1.MemoryActionRes{Success: svc.DeleteMemory(ctx, req.ID)}, nil
	case "disable":
		return &v1.MemoryActionRes{Success: svc.DisableMemory(ctx, req.ID)}, nil
	default:
		return nil, fmt.Errorf("unsupported memory action: %s", req.Action)
	}
}

func (c *ControllerV1) MemoryPromote(ctx context.Context, req *v1.MemoryPromoteReq) (res *v1.MemoryActionRes, err error) {
	success := service.NewMemoryService().PromoteMemory(ctx, req.ID, service.MemoryPromoteOptions{
		Scope:      mem.MemoryScope(req.Scope),
		ScopeID:    req.ScopeID,
		Confidence: req.Confidence,
	})
	return &v1.MemoryActionRes{Success: success}, nil
}

func toMemoryItem(item *mem.MemoryEntry) v1.MemoryItem {
	if item == nil {
		return v1.MemoryItem{}
	}
	return v1.MemoryItem{
		ID:            item.ID,
		SessionID:     item.SessionID,
		Type:          string(item.Type),
		Content:       item.Content,
		Source:        item.Source,
		Scope:         string(item.Scope),
		ScopeID:       item.ScopeID,
		Confidence:    item.Confidence,
		SafetyLabel:   item.SafetyLabel,
		Provenance:    item.Provenance,
		UpdatePolicy:  item.UpdatePolicy,
		ConflictGroup: item.ConflictGroup,
		ExpiresAt:     item.ExpiresAt,
		CreatedAt:     item.CreatedAt.UnixMilli(),
		UpdatedAt:     item.UpdatedAt.UnixMilli(),
		LastUsed:      item.LastUsed.UnixMilli(),
	}
}
