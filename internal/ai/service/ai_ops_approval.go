package service

import (
	"SuperBizAgent/internal/consts"
	"context"
	"strings"
)

func ListApprovalRequests(ctx context.Context, status string) ([]ApprovalRequest, error) {
	return listApprovalRequests(ctx, parseApprovalStatus(status))
}

func RejectQueuedAIOpsRequest(ctx context.Context, requestID, reviewReason string) (*ApprovalRequest, error) {
	request, err := rejectApprovalRequest(ctx, requestID, reviewerIdentity(ctx), reviewReason)
	if err != nil {
		return nil, err
	}
	_ = RecordIncidentApprovalRejection(ctx, requestID, reviewReason)
	return request, nil
}

func ApproveQueuedAIOpsRequest(ctx context.Context, requestID string) (ExecutionResponse, error) {
	request, err := approveApprovalRequest(ctx, requestID, reviewerIdentity(ctx))
	if err != nil {
		return ExecutionResponse{}, err
	}

	runCtx := context.WithValue(ctx, consts.CtxKeyApprovalBypass, true)
	runCtx = context.WithValue(runCtx, consts.CtxKeyApprovalRequestID, requestID)
	if request.SessionID != "" {
		runCtx = context.WithValue(runCtx, consts.CtxKeySessionID, request.SessionID)
	}
	if request.UserID != "" {
		runCtx = context.WithValue(runCtx, consts.CtxKeyUserID, request.UserID)
	}
	runCtx = withIncidentApprovalRun(runCtx, requestID)

	response, err := RunAIOpsMultiAgent(runCtx, request.Query)
	if err != nil {
		return response, err
	}
	response.ApprovalRequestID = requestID
	response.ApprovalStatus = string(ApprovalStatusApproved)
	if response.TraceID != "" {
		if markErr := markApprovalRequestExecuted(ctx, requestID, response.TraceID); markErr == nil {
			response.ApprovalStatus = string(ApprovalStatusExecuted)
		}
	}
	_ = RecordIncidentApprovalExecution(runCtx, requestID, response)
	return response, nil
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