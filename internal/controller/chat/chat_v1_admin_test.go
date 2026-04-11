package chat

import (
	"context"
	"strings"
	"testing"

	v1 "SuperBizAgent/api/chat/v1"
	aiService "SuperBizAgent/internal/ai/service"
)

func TestApproveApprovalRequestUsesDetailWhenResultIsEmpty(t *testing.T) {
	oldApprove := approveQueuedAIOpsRequest
	defer func() {
		approveQueuedAIOpsRequest = oldApprove
	}()

	approveQueuedAIOpsRequest = func(context.Context, string) (aiService.ExecutionResponse, error) {
		return aiService.ExecutionResponse{
			Detail: []string{"approval denied"},
		}, nil
	}

	ctrl := &ControllerV1{}
	res, err := ctrl.ApproveApprovalRequest(context.Background(), &v1.ApprovalActionReq{RequestID: "approval-1"})
	if err != nil {
		t.Fatalf("approve approval request returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected response")
	}
	if res.Result != "approval denied" {
		t.Fatalf("unexpected result: %q", res.Result)
	}
	if len(res.Detail) != 1 || res.Detail[0] != res.Result {
		t.Fatalf("unexpected detail: %v", res.Detail)
	}
}

func TestApproveApprovalRequestReturnsInternalErrorWhenResultAndDetailAreEmpty(t *testing.T) {
	oldApprove := approveQueuedAIOpsRequest
	defer func() {
		approveQueuedAIOpsRequest = oldApprove
	}()

	approveQueuedAIOpsRequest = func(context.Context, string) (aiService.ExecutionResponse, error) {
		return aiService.ExecutionResponse{}, nil
	}

	ctrl := &ControllerV1{}
	res, err := ctrl.ApproveApprovalRequest(context.Background(), &v1.ApprovalActionReq{RequestID: "approval-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "internal error") {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response, got %#v", res)
	}
}

func TestApproveApprovalRequestFiltersExecutionPlan(t *testing.T) {
	oldApprove := approveQueuedAIOpsRequest
	defer func() {
		approveQueuedAIOpsRequest = oldApprove
	}()

	approveQueuedAIOpsRequest = func(context.Context, string) (aiService.ExecutionResponse, error) {
		return aiService.ExecutionResponse{
			Content:       "done",
			ExecutionPlan: []string{"set api_key: supersecretkey12345 before retry"},
		}, nil
	}

	ctrl := &ControllerV1{}
	res, err := ctrl.ApproveApprovalRequest(context.Background(), &v1.ApprovalActionReq{RequestID: "approval-1"})
	if err != nil {
		t.Fatalf("approve approval request returned error: %v", err)
	}
	if len(res.ExecutionPlan) != 1 {
		t.Fatalf("expected one execution step, got %v", res.ExecutionPlan)
	}
	if strings.Contains(res.ExecutionPlan[0], "supersecretkey12345") {
		t.Fatalf("expected execution plan to be filtered, got %q", res.ExecutionPlan[0])
	}
}
