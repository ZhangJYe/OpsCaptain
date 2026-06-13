package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"

	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/gogf/gf/v2/frame/g"
)

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
				Confidence:  confidenceLogsEvidence,
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
			Confidence: confidenceLogsDegraded,
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

func degradedLogResult(task *protocol.TaskEnvelope, agentName, summary string, toolNames []string, toolErrors []string, mode string, focus string) *protocol.TaskResult {
	return &protocol.TaskResult{
		TaskID:      task.TaskID,
		Agent:       agentName,
		Status:      protocol.ResultStatusDegraded,
		Summary:     summary,
		Confidence:  confidenceLogsFullyDegraded,
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