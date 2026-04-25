package triage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentcontracts "SuperBizAgent/internal/ai/agent/contracts"
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/protocol"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "triage"

const (
	defaultTriageLLMTimeout = time.Second
	triagePrompt            = `你是一个运维查询分类器。根据用户输入，输出 JSON。

输入：
%s

输出格式：
{
  "intent": "alert_analysis | incident_analysis | kb_qa | data_query",
  "domains": ["metrics", "logs", "knowledge"],
  "priority": "high | medium | low",
  "use_multi_agent": true
}

规则：
- alert_analysis: 涉及告警、指标异常、容量、性能、延迟、错误率、资源使用率，通常需要 metrics + logs + knowledge
- incident_analysis: 涉及故障排查、失败、超时、重启、CrashLoopBackOff、connection refused、5xx、504，通常需要 logs + knowledge，必要时加 metrics
- kb_qa: 纯知识、配置、概念、SOP、错误码解释，通常只需要 knowledge
- data_query: 明确 SQL、数据库查询、表数据查询，通常只需要 knowledge
- 简单问候、闲聊、与运维无关的问题，domains 为空，use_multi_agent 为 false

只输出 JSON，不要输出 Markdown 代码块或额外解释。`
)

type Agent struct{}

type rule struct {
	intent   string
	domains  []string
	priority string
	keywords []string
	summary  string
}

type decision struct {
	intent        string
	domains       []string
	priority      string
	useMultiAgent bool
	summary       string
	source        string
	confidence    float64
}

type llmDecision struct {
	Intent        string   `json:"intent"`
	Domains       []string `json:"domains"`
	Priority      string   `json:"priority"`
	UseMultiAgent *bool    `json:"use_multi_agent"`
}

var (
	triageMode = func(ctx context.Context) string {
		v, err := g.Cfg().Get(ctx, "multi_agent.triage_mode")
		if err == nil && strings.TrimSpace(v.String()) != "" {
			return strings.ToLower(strings.TrimSpace(v.String()))
		}
		return "rule"
	}
	triageLLMTimeout = func(ctx context.Context) time.Duration {
		v, err := g.Cfg().Get(ctx, "multi_agent.triage_llm_timeout_ms")
		if err == nil && v.Int64() > 0 {
			return time.Duration(v.Int64()) * time.Millisecond
		}
		return defaultTriageLLMTimeout
	}
	classifyTriageWithLLM = defaultClassifyTriageWithLLM
)

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
	return []string{"intent-classification", "routing", agentcontracts.Capability(AgentName)}
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	query := strings.TrimSpace(task.Goal)
	selected, matchedRule := classifyByRule(query)
	route := decisionFromRule(selected, matchedRule)
	mode := triageMode(ctx)
	llmErr := ""

	switch mode {
	case "llm":
		if llmRoute, err := classifyTriageWithLLM(ctx, query); err == nil {
			route = llmRoute
		} else {
			llmErr = err.Error()
			route = decisionFromRule(defaultRule(), false)
			route.source = "fallback"
			route.confidence = 0.45
		}
	case "hybrid":
		if !matchedRule {
			if llmRoute, err := classifyTriageWithLLM(ctx, query); err == nil {
				route = llmRoute
			} else {
				llmErr = err.Error()
				route = decisionFromRule(defaultRule(), false)
				route.source = "fallback"
				route.confidence = 0.45
			}
		}
	default:
		mode = "rule"
	}

	if isHighPriority(query) {
		route.priority = "high"
	}

	metadata := map[string]any{
		"intent":          route.intent,
		"domains":         route.domains,
		"priority":        route.priority,
		"use_multi_agent": route.useMultiAgent,
		"matched_rule":    matchedRule,
		"triage_fallback": route.source == "fallback",
		"triage_mode":     mode,
		"triage_source":   route.source,
	}
	if llmErr != "" {
		metadata["llm_error"] = llmErr
	}

	return agentcontracts.AttachMetadata(&protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    route.summary,
		Confidence: route.confidence,
		Metadata:   metadata,
	}, AgentName), nil
}

func classifyByRule(query string) (rule, bool) {
	lower := strings.ToLower(strings.TrimSpace(query))
	selected := defaultRule()
	for _, candidate := range triageRules {
		if matchesRule(lower, candidate.keywords) {
			return candidate, true
		}
	}
	return selected, false
}

func defaultRule() rule {
	return rule{
		intent:   "alert_analysis",
		domains:  []string{"metrics", "logs", "knowledge"},
		priority: "medium",
		summary:  "已识别为告警分析任务，优先查询告警、日志和知识库。",
	}
}

func decisionFromRule(selected rule, matched bool) decision {
	source := "rule"
	confidence := 0.76
	if !matched {
		source = "fallback"
		confidence = 0.45
	}
	return decision{
		intent:        selected.intent,
		domains:       append([]string(nil), selected.domains...),
		priority:      selected.priority,
		useMultiAgent: len(selected.domains) > 0,
		summary:       selected.summary,
		source:        source,
		confidence:    confidence,
	}
}

func defaultClassifyTriageWithLLM(ctx context.Context, query string) (decision, error) {
	callCtx, cancel := context.WithTimeout(ctx, triageLLMTimeout(ctx))
	defer cancel()

	chatModel, err := models.OpenAIForDeepSeekV3Quick(callCtx)
	if err != nil {
		return decision{}, err
	}
	resp, err := chatModel.Generate(callCtx, []*schema.Message{
		{Role: schema.System, Content: "你只输出合法 JSON。"},
		{Role: schema.User, Content: fmt.Sprintf(triagePrompt, query)},
	})
	if err != nil {
		return decision{}, err
	}
	return parseLLMDecision(resp.Content)
}

func parseLLMDecision(raw string) (decision, error) {
	payload := extractJSONObject(raw)
	if payload == "" {
		return decision{}, fmt.Errorf("triage llm response does not contain JSON")
	}
	var parsed llmDecision
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return decision{}, err
	}
	intent := normalizeIntent(parsed.Intent)
	domains := normalizeDomains(parsed.Domains)
	priority := normalizePriority(parsed.Priority)
	if intent == "" {
		return decision{}, fmt.Errorf("triage llm returned unsupported intent %q", parsed.Intent)
	}
	useMultiAgent := len(domains) > 0
	if parsed.UseMultiAgent != nil {
		useMultiAgent = *parsed.UseMultiAgent
	}
	if !useMultiAgent {
		domains = nil
	}
	return decision{
		intent:        intent,
		domains:       domains,
		priority:      priority,
		useMultiAgent: useMultiAgent,
		summary:       summaryForLLMDecision(intent, domains, useMultiAgent),
		source:        "llm",
		confidence:    0.82,
	}, nil
}

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return ""
	}
	return raw[start : end+1]
}

func normalizeIntent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "alert_analysis", "alert":
		return "alert_analysis"
	case "incident_analysis", "incident":
		return "incident_analysis"
	case "kb_qa", "knowledge", "qa":
		return "kb_qa"
	case "data_query", "data":
		return "data_query"
	default:
		return ""
	}
}

func normalizeDomains(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "metrics", "metric":
			if _, ok := seen["metrics"]; !ok {
				out = append(out, "metrics")
				seen["metrics"] = struct{}{}
			}
		case "logs", "log":
			if _, ok := seen["logs"]; !ok {
				out = append(out, "logs")
				seen["logs"] = struct{}{}
			}
		case "knowledge", "kb", "docs":
			if _, ok := seen["knowledge"]; !ok {
				out = append(out, "knowledge")
				seen["knowledge"] = struct{}{}
			}
		}
	}
	return out
}

func normalizePriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func summaryForLLMDecision(intent string, domains []string, useMultiAgent bool) string {
	if !useMultiAgent {
		return "已识别为非多智能体任务，不需要调用专业代理。"
	}
	return fmt.Sprintf("LLM 已识别为 %s，建议调用领域：%s。", intent, strings.Join(domains, ","))
}

func isHighPriority(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	return strings.Contains(lower, "严重") || strings.Contains(lower, "sev1") || strings.Contains(lower, "p0")
}

func matchesRule(query string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(query, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
