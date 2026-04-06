package service

import (
	"SuperBizAgent/internal/consts"
	"context"
	"strings"
)

type ApprovalDecision struct {
	Approved        bool
	Queued          bool
	Reason          string
	ApprovalRequest *ApprovalRequest
}

type ApprovalGate interface {
	Check(ctx context.Context, action string) ApprovalDecision
}

type StaticApprovalGate struct {
	queue *ApprovalQueue
}

func NewApprovalGate() *StaticApprovalGate {
	return &StaticApprovalGate{queue: NewApprovalQueue()}
}

var highRiskApprovalKeywords = []string{
	"delete", "drop", "update", "insert", "truncate", "alter", "rollback", "restart",
	"执行", "删除", "修改", "回滚", "重启", "写入", "变更",
}

func (g *StaticApprovalGate) Check(ctx context.Context, action string) ApprovalDecision {
	if bypass, _ := ctx.Value(consts.CtxKeyApprovalBypass).(bool); bypass {
		return ApprovalDecision{Approved: true}
	}

	lower := strings.ToLower(strings.TrimSpace(action))
	if !requiresApproval(lower) {
		return ApprovalDecision{Approved: true}
	}

	reason := "high-risk action queued for human approval"
	request, err := g.queue.Enqueue(ctx, action, reason, buildExecutionPlan(action))
	if err != nil {
		return ApprovalDecision{
			Approved: false,
			Reason:   "high-risk action detected but approval queue is unavailable",
		}
	}

	return ApprovalDecision{
		Approved:        false,
		Queued:          true,
		Reason:          reason,
		ApprovalRequest: request,
	}
}

func requiresApproval(action string) bool {
	for _, keyword := range highRiskApprovalKeywords {
		if strings.Contains(action, keyword) {
			return true
		}
	}
	return false
}

func buildExecutionPlan(action string) []string {
	preview := strings.TrimSpace(action)
	if len(preview) > 160 {
		preview = preview[:160] + "..."
	}
	return []string{
		"Review the requested operation scope and affected resources.",
		"Validate safety constraints and available rollback options.",
		"After approval, execute the original request: " + preview,
	}
}
