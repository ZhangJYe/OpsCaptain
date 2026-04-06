package service

import (
	"SuperBizAgent/internal/consts"
	"context"
	"testing"
)

func TestRequiresApproval(t *testing.T) {
	if !requiresApproval("delete production rows") {
		t.Fatal("expected destructive action to require approval")
	}
	if requiresApproval("summarize current alert status") {
		t.Fatal("expected read-only request to skip approval")
	}
}

func TestApprovalGateBypassContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), consts.CtxKeyApprovalBypass, true)
	decision := NewApprovalGate().Check(ctx, "delete production rows")
	if !decision.Approved {
		t.Fatalf("expected bypass context to approve request, got %#v", decision)
	}
}

func TestBuildExecutionPlanIncludesPreview(t *testing.T) {
	plan := buildExecutionPlan("restart payment service deployment")
	if len(plan) == 0 {
		t.Fatal("expected non-empty execution plan")
	}
}
