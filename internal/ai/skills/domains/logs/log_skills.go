package logs

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentcontracts "SuperBizAgent/internal/ai/agent/contracts"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/tools"
)

const AgentName = "logs"

const (
	defaultLogQueryTimeout  = 3 * time.Second
	defaultLogEvidenceLimit = 3
)

const (
	confidenceLogsEvidence      = 0.74
	confidenceLogsDegraded      = 0.42
	confidenceLogsFullyDegraded = 0.28
)

var discoverLogTools = tools.GetLogMcpTool

type Agent struct {
	registry *skills.Registry
}

type logSkill struct {
	name        string
	description string
	mode        string
	keywords    []string
	focus       string
	matcher     func(*protocol.TaskEnvelope) bool
}

func New() *Agent {
	return &Agent{registry: buildLogSkillRegistry()}
}

func SkillRegistry() *skills.Registry {
	return buildLogSkillRegistry()
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return skills.PrefixedCapabilities([]string{"log-mcp-query", "log-evidence-extraction", agentcontracts.Capability(AgentName)}, a.registry.SkillNames())
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	execution, err := a.registry.Execute(ctx, task)
	if err != nil {
		return nil, err
	}
	if rt, ok := runtime.FromContext(ctx); ok {
		rt.EmitInfo(ctx, task, a.Name(), fmt.Sprintf("selected skill=%s", execution.Skill.Name()), map[string]any{
			"skill_name":        execution.Skill.Name(),
			"skill_description": execution.Skill.Description(),
		})
	}
	return agentcontracts.AttachMetadata(execution.Result, AgentName), nil
}

func (s *logSkill) Name() string {
	return s.name
}

func (s *logSkill) Description() string {
	return s.description
}

func (s *logSkill) Match(task *protocol.TaskEnvelope) bool {
	if task == nil {
		return false
	}
	if s.matcher != nil {
		return s.matcher(task)
	}
	if len(s.keywords) == 0 {
		return false
	}
	return skills.ContainsAny(task.Goal, s.keywords...)
}

func (s *logSkill) Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return runLogSkillWithFocus(ctx, task, s.mode, s.focus)
}

func (s *logSkill) Focus() string {
	return s.focus
}

func buildLogSkillRegistry() *skills.Registry {
	registry, err := skills.NewRegistry(
		AgentName,
		[]skills.Skill{
			&logSkill{
				name:        "logs_service_offline_panic_trace",
				description: "Trace service offline, pod restart, crashloop, and panic evidence from logs.",
				mode:        "service_offline_panic_trace",
				focus:       "Focus on panic, stack trace, nil pointer, restart reason, crashloop, oom, pod restart count, and the latest release.",
				matcher:     matchesServiceOfflinePanicTask,
				keywords: []string{
					"service offline", "service down", "pod restart", "crashloop", "panic", "stack trace", "nil pointer", "oom", "restart",
				},
			},
			&logSkill{
				name:        "logs_api_failure_rate_investigation",
				description: "Trace API failure rate spikes, 5xx responses, and upstream or downstream failures from logs.",
				mode:        "api_failure_rate_investigation",
				focus:       "Focus on api name, route, status code, response payload, 4xx, 5xx, upstream, downstream, timeout, and dependency failures.",
				keywords: []string{
					"api failure rate", "failure rate", "5xx", "4xx", "response error", "error rate", "endpoint", "route", "upstream", "downstream",
				},
			},
			&logSkill{
				name:        "logs_payment_timeout_trace",
				description: "Trace payment, order, and checkout timeout evidence from logs.",
				mode:        "payment_timeout_trace",
				focus:       "Focus on payment, order, checkout, gateway timeout, retry, db timeout, and downstream latency.",
				matcher:     matchesPaymentTimeoutTask,
				keywords: []string{
					"payment timeout", "checkout timeout", "order timeout", "支付超时", "订单超时",
					"payment", "checkout", "order", "timeout", "504", "gateway timeout",
				},
			},
			&logSkill{
				name:        "logs_auth_failure_trace",
				description: "Trace login, token, and authorization failures from logs.",
				mode:        "auth_failure_trace",
				focus:       "Focus on login, token, jwt, forbidden, unauthorized, permission denied, and auth middleware.",
				keywords: []string{
					"login", "auth", "authentication", "authorization", "token", "jwt", "unauthorized", "forbidden",
					"登录", "鉴权", "令牌", "权限", "未授权",
				},
			},
			&logSkill{
				name:        "logs_evidence_extract",
				description: "Extract structured log evidence for error, timeout, and exception focused queries.",
				mode:        "evidence_extract",
				focus:       "Focus on error, timeout, exception, panic, and stack trace signals.",
				keywords: []string{
					"error", "errors", "exception", "timeout", "fail", "failed", "panic", "stack",
					"报错", "异常", "错误", "超时", "失败", "堆栈", "日志证据",
				},
			},
			&logSkill{
				name:        "logs_raw_review",
				description: "Fallback log review skill that still returns raw snippets when structured evidence is unavailable.",
				mode:        "raw_review",
				focus:       "Focus on broad log review and retain raw output when structure is unavailable.",
			},
		},
		skills.WithDefault("logs_raw_review"),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to build log skills registry: %v", err))
	}
	return registry
}

func matchesServiceOfflinePanicTask(task *protocol.TaskEnvelope) bool {
	if task == nil {
		return false
	}
	goal := strings.TrimSpace(task.Goal)
	if goal == "" {
		return false
	}
	hasPanicSignal := skills.ContainsAny(goal, "panic", "stack", "stack trace", "nil pointer", "oom", "fatal")
	hasOfflineSignal := skills.ContainsAny(goal, "offline", "down", "restart", "restarting", "crashloop", "pod", "service unavailable")
	return hasPanicSignal && hasOfflineSignal
}

func matchesPaymentTimeoutTask(task *protocol.TaskEnvelope) bool {
	if task == nil {
		return false
	}
	goal := strings.TrimSpace(task.Goal)
	if goal == "" {
		return false
	}
	primaryFlowSignal := skills.ContainsAny(goal, "payment", "checkout", "billing", "gateway")
	orderFlowSignal := skills.ContainsAny(goal, "order")
	hasIssueSignal := skills.ContainsAny(goal, "timeout", "latency", "retry", "504", "slow", "downstream", "error", "errors", "fail", "failed", "exception")
	if !(primaryFlowSignal || orderFlowSignal) {
		return false
	}
	if skills.ContainsAny(goal, "api failure rate", "failure rate", "error rate", "5xx", "4xx", "endpoint", "route") {
		return false
	}
	if primaryFlowSignal {
		return true
	}
	return hasIssueSignal
}