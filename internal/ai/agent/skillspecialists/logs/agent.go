package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/tools"

	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "logs"

const (
	defaultLogQueryTimeout  = 3 * time.Second
	defaultLogEvidenceLimit = 3
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
	return skills.PrefixedCapabilities([]string{"log-mcp-query", "log-evidence-extraction"}, a.registry.SkillNames())
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
	return execution.Result, nil
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
		return true
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
				"鎶ラ敊", "寮傚父", "閿欒", "瓒呮椂", "澶辫触", "鍫嗘爤", "鏃ュ織璇佹嵁",
			},
		},
		&logSkill{
			name:        "logs_raw_review",
			description: "Fallback log review skill that still returns raw snippets when structured evidence is unavailable.",
			mode:        "raw_review",
			focus:       "Focus on broad log review and retain raw output when structure is unavailable.",
		},
	)
	if err != nil {
		panic(fmt.Sprintf("failed to build log skills registry: %v", err))
	}
	return registry
}

func runLogSkillWithFocus(ctx context.Context, task *protocol.TaskEnvelope, mode string, focus string) (*protocol.TaskResult, error) {
	toolList, err := discoverLogTools()
	if err != nil {
		return degradedLogResult(task, AgentName, fmt.Sprintf("log MCP bootstrap failed: %v", err), nil, []string{err.Error()}, mode, focus), nil
	}
	if len(toolList) == 0 {
		return degradedLogResult(task, AgentName, "log query capability is not configured", nil, nil, mode, focus), nil
	}

	limit := logEvidenceLimit(ctx)
	toolNames := make([]string, 0, len(toolList))
	toolErrors := make([]string, 0)
	rawOutputs := make([]string, 0)

	for _, baseTool := range toolList {
		name, desc := describeTool(ctx, baseTool)
		toolNames = append(toolNames, name)

		invokable, ok := baseTool.(toolapi.InvokableTool)
		if !ok {
			continue
		}

		output, invokeErr := invokeFocusedLogTool(ctx, invokable, task.Goal, limit, mode, focus)
		if invokeErr != nil {
			toolErrors = append(toolErrors, fmt.Sprintf("%s: %v", name, invokeErr))
			continue
		}

		evidence := buildLogEvidence(name, output, limit)
		if len(evidence) > 0 {
			summary := fmt.Sprintf("log skill %s extracted %d evidence items with %s", mode, len(evidence), name)
			if len(toolErrors) > 0 {
				summary += "; other tools degraded automatically"
			}
			return &protocol.TaskResult{
				TaskID:      task.TaskID,
				Agent:       AgentName,
				Status:      protocol.ResultStatusSucceeded,
				Summary:     summary,
				Confidence:  0.74,
				Evidence:    evidence,
				NextActions: buildLogNextActions(mode),
				Metadata: map[string]any{
					"tool_names":       toolNames,
					"tool_errors":      toolErrors,
					"successful_tool":  name,
					"tool_description": desc,
					"log_mode":         mode,
					"log_focus":        focus,
				},
			}, nil
		}

		if snippet := fallbackSnippet(output, desc); snippet != "" {
			rawOutputs = append(rawOutputs, fmt.Sprintf("%s: %s", name, snippet))
		}
	}

	if len(rawOutputs) > 0 {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    "log tools ran, but only raw outputs were available",
			Confidence: 0.42,
			Evidence: []protocol.EvidenceItem{
				{
					SourceType: "log-raw",
					SourceID:   "raw-output",
					Title:      "raw log output",
					Snippet:    shorten(strings.Join(rawOutputs, " | "), 240),
					Score:      0.44,
				},
			},
			NextActions: buildLogNextActions(mode),
			Metadata: map[string]any{
				"tool_names":  toolNames,
				"tool_errors": toolErrors,
				"log_mode":    mode,
				"log_focus":   focus,
			},
		}, nil
	}

	summary := fmt.Sprintf("found %d log MCP tools but no reusable log evidence", len(toolNames))
	if len(toolErrors) > 0 {
		summary += "; tool errors: " + strings.Join(toolErrors, " ; ")
	}
	return degradedLogResult(task, AgentName, summary, toolNames, toolErrors, mode, focus), nil
}

func invokeFocusedLogTool(ctx context.Context, tool toolapi.InvokableTool, query string, limit int, mode string, focus string) (string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, logQueryTimeout(ctx))
	defer cancel()
	return tool.InvokableRun(queryCtx, buildLogQueryPayloadWithFocus(query, limit, mode, focus))
}

func buildLogQueryPayloadWithFocus(query string, limit int, mode string, focus string) string {
	focusedQuery := buildFocusedLogQuery(query, focus)
	payload, err := json.Marshal(map[string]any{
		"query":      focusedQuery,
		"limit":      limit,
		"skill_mode": mode,
		"focus":      focus,
	})
	if err != nil {
		return fmt.Sprintf(`{"query":%q,"limit":%d,"skill_mode":%q}`, focusedQuery, limit, mode)
	}
	return string(payload)
}

func buildFocusedLogQuery(query string, focus string) string {
	query = strings.TrimSpace(query)
	focus = strings.TrimSpace(focus)
	if focus == "" {
		return query
	}
	if query == "" {
		return focus
	}
	return query + "\nFocus: " + focus
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

func buildLogNextActions(mode string) []string {
	switch mode {
	case "service_offline_panic_trace":
		return []string{
			"compare the panic timestamp with the latest deploy or config change",
			"check pod restart count, crashloop reason, and the failing stack frame owner",
		}
	case "api_failure_rate_investigation":
		return []string{
			"separate client errors from server errors and confirm the dominant status code family",
			"check whether the failure spike correlates with an upstream or downstream dependency change",
		}
	default:
		return nil
	}
}

func mustNewSkillRegistry() *skills.Registry {
	registry, err := skills.NewRegistry(
		AgentName,
		&logSkill{
			name:        "logs_evidence_extract",
			description: "Extract structured log evidence for error, timeout, and exception focused queries.",
			mode:        "evidence_extract",
			keywords: []string{
				"error", "errors", "exception", "timeout", "fail", "failed", "panic", "stack",
				"报错", "异常", "错误", "超时", "失败", "堆栈", "日志证据",
			},
		},
		&logSkill{
			name:        "logs_raw_review",
			description: "Fallback log review skill that still returns raw snippets when structured evidence is unavailable.",
			mode:        "raw_review",
		},
	)
	if err != nil {
		panic(fmt.Sprintf("failed to build log skills registry: %v", err))
	}
	return registry
}

func runLogSkill(ctx context.Context, task *protocol.TaskEnvelope, mode string) (*protocol.TaskResult, error) {
	toolList, err := discoverLogTools()
	if err != nil {
		return degradedLogResult(task, AgentName, fmt.Sprintf("log MCP bootstrap failed: %v", err), nil, []string{err.Error()}, mode, ""), nil
	}
	if len(toolList) == 0 {
		return degradedLogResult(task, AgentName, "log query capability is not configured", nil, nil, mode, ""), nil
	}

	limit := logEvidenceLimit(ctx)
	toolNames := make([]string, 0, len(toolList))
	toolErrors := make([]string, 0)
	rawOutputs := make([]string, 0)

	for _, baseTool := range toolList {
		name, desc := describeTool(ctx, baseTool)
		toolNames = append(toolNames, name)

		invokable, ok := baseTool.(toolapi.InvokableTool)
		if !ok {
			continue
		}

		output, invokeErr := invokeLogTool(ctx, invokable, task.Goal, limit)
		if invokeErr != nil {
			toolErrors = append(toolErrors, fmt.Sprintf("%s: %v", name, invokeErr))
			continue
		}

		evidence := buildLogEvidence(name, output, limit)
		if len(evidence) > 0 {
			summary := fmt.Sprintf("log skill %s extracted %d evidence items with %s", mode, len(evidence), name)
			if len(toolErrors) > 0 {
				summary += "; other tools degraded automatically"
			}
			return &protocol.TaskResult{
				TaskID:     task.TaskID,
				Agent:      AgentName,
				Status:     protocol.ResultStatusSucceeded,
				Summary:    summary,
				Confidence: 0.74,
				Evidence:   evidence,
				Metadata: map[string]any{
					"tool_names":       toolNames,
					"tool_errors":      toolErrors,
					"successful_tool":  name,
					"tool_description": desc,
					"log_mode":         mode,
				},
			}, nil
		}

		if snippet := fallbackSnippet(output, desc); snippet != "" {
			rawOutputs = append(rawOutputs, fmt.Sprintf("%s: %s", name, snippet))
		}
	}

	if len(rawOutputs) > 0 {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    "log tools ran, but only raw outputs were available",
			Confidence: 0.42,
			Evidence: []protocol.EvidenceItem{
				{
					SourceType: "log-raw",
					SourceID:   "raw-output",
					Title:      "raw log output",
					Snippet:    shorten(strings.Join(rawOutputs, " | "), 240),
					Score:      0.44,
				},
			},
			Metadata: map[string]any{
				"tool_names":  toolNames,
				"tool_errors": toolErrors,
				"log_mode":    mode,
			},
		}, nil
	}

	summary := fmt.Sprintf("found %d log MCP tools but no reusable log evidence", len(toolNames))
	if len(toolErrors) > 0 {
		summary += "; tool errors: " + strings.Join(toolErrors, " ; ")
	}
	return degradedLogResult(task, AgentName, summary, toolNames, toolErrors, mode, ""), nil
}

func degradedLogResult(task *protocol.TaskEnvelope, agentName, summary string, toolNames []string, toolErrors []string, mode string, focus string) *protocol.TaskResult {
	return &protocol.TaskResult{
		TaskID:      task.TaskID,
		Agent:       agentName,
		Status:      protocol.ResultStatusDegraded,
		Summary:     summary,
		Confidence:  0.28,
		NextActions: buildLogNextActions(mode),
		Metadata: map[string]any{
			"tool_names":  toolNames,
			"tool_errors": toolErrors,
			"log_mode":    mode,
			"log_focus":   focus,
		},
	}
}

func describeTool(ctx context.Context, baseTool toolapi.BaseTool) (string, string) {
	info, err := baseTool.Info(ctx)
	if err != nil || info == nil {
		return "unknown-log-tool", ""
	}
	return fallback(info.Name, "unknown-log-tool"), strings.TrimSpace(info.Desc)
}

func invokeLogTool(ctx context.Context, tool toolapi.InvokableTool, query string, limit int) (string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, logQueryTimeout(ctx))
	defer cancel()
	return tool.InvokableRun(queryCtx, buildLogQueryPayload(query, limit))
}

func buildLogQueryPayload(query string, limit int) string {
	payload, err := json.Marshal(map[string]any{
		"query": query,
		"limit": limit,
	})
	if err != nil {
		return fmt.Sprintf(`{"query":%q,"limit":%d}`, query, limit)
	}
	return string(payload)
}

func buildLogEvidence(sourceName, output string, limit int) []protocol.EvidenceItem {
	snippets := collectLogSnippets(strings.TrimSpace(output), limit)
	if len(snippets) == 0 {
		return nil
	}
	evidence := make([]protocol.EvidenceItem, 0, len(snippets))
	for idx, snippet := range snippets {
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "log",
			SourceID:   fmt.Sprintf("%s-%d", sourceName, idx+1),
			Title:      fmt.Sprintf("%s log evidence %d", sourceName, idx+1),
			Snippet:    snippet,
			Score:      0.72 - float64(idx)*0.06,
		})
	}
	return evidence
}

func collectLogSnippets(output string, limit int) []string {
	if output == "" || limit <= 0 {
		return nil
	}

	var payload any
	if err := json.Unmarshal([]byte(output), &payload); err == nil {
		snippets := collectFromValue(payload, limit)
		return dedupAndLimit(snippets, limit)
	}

	return dedupAndLimit(splitLogLines(output), limit)
}

func collectFromValue(value any, limit int) []string {
	if limit <= 0 || value == nil {
		return nil
	}

	switch typed := value.(type) {
	case map[string]any:
		var snippets []string
		for _, key := range []string{"logs", "items", "results", "data", "entries", "records"} {
			if nested, ok := typed[key]; ok {
				snippets = append(snippets, collectFromValue(nested, limit-len(snippets))...)
				if len(snippets) >= limit {
					return snippets[:limit]
				}
			}
		}
		if snippet := snippetFromMap(typed); snippet != "" {
			snippets = append(snippets, snippet)
		}
		if len(snippets) == 0 {
			if raw, err := json.Marshal(typed); err == nil {
				snippets = append(snippets, shorten(string(raw), 200))
			}
		}
		return snippets
	case []any:
		var snippets []string
		for _, item := range typed {
			snippets = append(snippets, collectFromValue(item, limit-len(snippets))...)
			if len(snippets) >= limit {
				return snippets[:limit]
			}
		}
		return snippets
	case string:
		return splitLogLines(typed)
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text == "" || text == "<nil>" {
			return nil
		}
		return []string{shorten(text, 200)}
	}
}

func snippetFromMap(item map[string]any) string {
	message := firstString(item, "message", "msg", "content", "text", "log", "line", "raw", "description")
	if message == "" {
		return ""
	}
	timestamp := firstString(item, "timestamp", "time", "ts")
	level := firstString(item, "level", "severity")
	source := firstString(item, "service", "app", "source", "host")

	parts := make([]string, 0, 4)
	if timestamp != "" {
		parts = append(parts, timestamp)
	}
	if level != "" {
		parts = append(parts, "["+level+"]")
	}
	if source != "" {
		parts = append(parts, "("+source+")")
	}
	parts = append(parts, message)
	return shorten(strings.Join(parts, " "), 200)
}

func splitLogLines(output string) []string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, shorten(line, 200))
	}
	return out
}

func dedupAndLimit(items []string, limit int) []string {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, min(limit, len(items)))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func firstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fallbackSnippet(output, desc string) string {
	if snippets := collectLogSnippets(output, 1); len(snippets) > 0 {
		return snippets[0]
	}
	return shorten(desc, 160)
}

func fallback(value, alt string) string {
	if strings.TrimSpace(value) == "" {
		return alt
	}
	return value
}

func shorten(input string, max int) string {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
	if len(input) <= max {
		return input
	}
	return input[:max] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func logQueryTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "multi_agent.log_query_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultLogQueryTimeout
}

func logEvidenceLimit(ctx context.Context) int {
	v, err := g.Cfg().Get(ctx, "multi_agent.log_evidence_limit")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return defaultLogEvidenceLimit
}
