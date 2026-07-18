package service

import (
	"SuperBizAgent/internal/consts"
	"context"
	"strings"
	"testing"
)

func TestRequiresApproval(t *testing.T) {
	if !requiresApproval("delete production rows") {
		t.Fatal("expected destructive action to require approval")
	}
	if requiresApproval("summarize current alert status") {
		t.Fatal("expected read-only request to skip approval")
	}
	if requiresApproval("请给出回滚、限流和验证步骤") {
		t.Fatal("expected advisory rollback plan to skip approval")
	}
	if !requiresApproval("请立即回滚 paymentservice") {
		t.Fatal("expected explicit rollback operation to require approval")
	}
}

func TestRequiresApprovalUnderstandsNegatedExecution(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   bool
	}{
		{name: "chinese do not execute changes", action: "只读查询当前告警，不要执行任何变更", want: false},
		{name: "chinese negation spans enumerated operations", action: "只读检查当前活跃的 Prometheus 告警，并基于可用证据简要判断是否存在真实故障；不要执行重启、部署或配置变更。", want: false},
		{name: "chinese must not restart", action: "请排查延迟问题，不得重启服务", want: false},
		{name: "english do not restart", action: "analyze current alerts, do not restart the service", want: false},
		{name: "affirmative restart", action: "分析完成后立即重启 paymentservice", want: true},
		{name: "contrast keeps affirmative action", action: "不要重启旧实例，而是立即重启新实例", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiresApproval(strings.ToLower(tt.action)); got != tt.want {
				t.Fatalf("requiresApproval(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
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
