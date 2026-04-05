package service

import (
	"context"
	"strings"
)

type ApprovalDecision struct {
	Approved bool
	Reason   string
}

type ApprovalGate interface {
	Check(ctx context.Context, action string) ApprovalDecision
}

type StaticApprovalGate struct{}

func NewApprovalGate() *StaticApprovalGate {
	return &StaticApprovalGate{}
}

func (g *StaticApprovalGate) Check(_ context.Context, action string) ApprovalDecision {
	lower := strings.ToLower(action)
	if strings.Contains(lower, "delete") || strings.Contains(lower, "drop") ||
		strings.Contains(lower, "update") || strings.Contains(lower, "insert") ||
		strings.Contains(lower, "执行") || strings.Contains(lower, "删除") || strings.Contains(lower, "修改") {
		return ApprovalDecision{
			Approved: false,
			Reason:   "检测到高风险动作，当前未获得审批。",
		}
	}
	return ApprovalDecision{Approved: true}
}
