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
	"删除", "修改", "回滚", "重启", "写入", "变更",
}

var explicitExecutionKeywords = []string{
	"delete", "drop", "update", "insert", "truncate", "alter", "restart",
	"execute", "apply", "perform", "run rollback", "rollback ",
	"执行", "立即", "删除", "修改", "写入", "变更", "重启", "执行回滚", "立即回滚", "实际回滚", "帮我回滚",
}

var analysisOnlyKeywords = []string{
	"分析", "排查", "诊断", "建议", "步骤", "方案", "如何", "查询", "查看", "说明", "生成", "评估", "给出", "列出", "总结", "报告",
	"analyze", "diagnose", "summarize", "suggest", "recommend", "how to", "steps", "plan",
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
	hasRisk := false
	for _, keyword := range highRiskApprovalKeywords {
		if strings.Contains(action, keyword) {
			hasRisk = true
			break
		}
	}
	if !hasRisk {
		return false
	}
	if hasAnyKeyword(action, analysisOnlyKeywords) && !hasAnyKeyword(action, explicitExecutionKeywords) {
		return false
	}
	return hasAnyKeyword(action, explicitExecutionKeywords)
}

func hasAnyKeyword(action string, keywords []string) bool {
	for _, keyword := range keywords {
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
