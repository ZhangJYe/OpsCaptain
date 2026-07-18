package service

import (
	"SuperBizAgent/internal/consts"
	"context"
	"regexp"
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

var negatedExecutionPrefix = regexp.MustCompile(`(?:不要|不得|禁止|避免|无需|不能|不应|不允许|不需要|请勿|不)(?:再|直接|立即|实际)?(?:执行|进行|做)?(?:任何)?$|(?:do not|don't|must not|never|without|no)(?: directly| actually)?(?: execute| perform| apply| make)?(?: any)?\s*$`)
var negatedExecutionScope = regexp.MustCompile(`(?:不要|不得|禁止|避免|无需|不能|不应|不允许|不需要|请勿|不)(?:再|直接|立即|实际)?(?:执行|进行|做)|(?:do not|don't|must not|never|without)(?: directly| actually)?(?: execute| perform| apply| make)`)

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
	if !hasAnyUnnegatedKeyword(action, highRiskApprovalKeywords) {
		return false
	}
	hasExecution := hasAnyUnnegatedKeyword(action, explicitExecutionKeywords)
	if hasAnyKeyword(action, analysisOnlyKeywords) && !hasExecution {
		return false
	}
	return hasExecution
}

func hasAnyUnnegatedKeyword(action string, keywords []string) bool {
	for _, keyword := range keywords {
		remaining := action
		for {
			index := strings.Index(remaining, keyword)
			if index < 0 {
				break
			}
			if !keywordIsNegated(remaining[:index]) {
				return true
			}
			remaining = remaining[index+len(keyword):]
		}
	}
	return false
}

func keywordIsNegated(prefix string) bool {
	if negatedExecutionPrefix.MatchString(prefix) {
		return true
	}
	return negatedExecutionScope.MatchString(currentExecutionClause(prefix))
}

func currentExecutionClause(value string) string {
	start := 0
	for _, separator := range []string{"，", ",", "。", ";", "；", "\n", "而是", "但是", "但要", "however", "but "} {
		if index := strings.LastIndex(value, separator); index >= 0 && index+len(separator) > start {
			start = index + len(separator)
		}
	}
	return value[start:]
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
