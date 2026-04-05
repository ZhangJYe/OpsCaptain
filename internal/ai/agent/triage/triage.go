package triage

import (
	"context"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

const AgentName = "triage"

type Agent struct{}

type rule struct {
	intent   string
	domains  []string
	priority string
	keywords []string
	summary  string
}

var triageRules = []rule{
	{
		intent:   "alert_analysis",
		domains:  []string{"metrics", "logs", "knowledge"},
		priority: "high",
		keywords: []string{"告警", "alert", "prometheus"},
		summary:  "已识别为告警分析任务，优先查询告警、日志和知识库。",
	},
	{
		intent:   "kb_qa",
		domains:  []string{"knowledge"},
		priority: "medium",
		keywords: []string{"文档", "知识库", "runbook", "sop"},
		summary:  "已识别为知识检索任务，优先查询内部文档。",
	},
	{
		intent:   "data_query",
		domains:  []string{"knowledge"},
		priority: "medium",
		keywords: []string{"sql", "mysql", "数据库"},
		summary:  "已识别为数据查询任务，当前优先返回知识和操作建议。",
	},
	{
		intent:   "incident_analysis",
		domains:  []string{"logs", "knowledge"},
		priority: "medium",
		keywords: []string{"日志", "log"},
		summary:  "已识别为故障排查任务，优先查询日志和知识库。",
	},
}

func New() *Agent {
	return &Agent{}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return []string{"intent-classification", "routing"}
}

func (a *Agent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	query := strings.TrimSpace(task.Goal)
	lower := strings.ToLower(query)
	selected := rule{
		intent:   "alert_analysis",
		domains:  []string{"metrics", "logs", "knowledge"},
		priority: "medium",
		summary:  "已识别为告警分析任务，优先查询告警、日志和知识库。",
	}

	for _, candidate := range triageRules {
		if matchesRule(lower, candidate.keywords) {
			selected = candidate
			break
		}
	}

	if strings.Contains(lower, "严重") || strings.Contains(lower, "sev1") || strings.Contains(lower, "p0") {
		selected.priority = "high"
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    selected.summary,
		Confidence: 0.76,
		Metadata: map[string]any{
			"intent":   selected.intent,
			"domains":  selected.domains,
			"priority": selected.priority,
		},
	}, nil
}

func matchesRule(query string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(query, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
