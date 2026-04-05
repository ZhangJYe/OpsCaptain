package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
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

type Agent struct{}

func New() *Agent {
	return &Agent{}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return []string{"log-mcp-query", "log-evidence-extraction"}
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	toolList, err := discoverLogTools()
	if err != nil {
		return degradedLogResult(task, a.Name(), fmt.Sprintf("日志 MCP 能力初始化失败：%v", err), nil, []string{err.Error()}), nil
	}
	if len(toolList) == 0 {
		return degradedLogResult(task, a.Name(), "日志查询能力未配置，已跳过该步骤。", nil, nil), nil
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
			summary := fmt.Sprintf("已通过日志工具 %s 提取到 %d 条相关日志证据。", name, len(evidence))
			if len(toolErrors) > 0 {
				summary += " 其他工具调用失败，已自动降级处理。"
			}
			return &protocol.TaskResult{
				TaskID:     task.TaskID,
				Agent:      a.Name(),
				Status:     protocol.ResultStatusSucceeded,
				Summary:    summary,
				Confidence: 0.74,
				Evidence:   evidence,
				Metadata: map[string]any{
					"tool_names":       toolNames,
					"tool_errors":      toolErrors,
					"successful_tool":  name,
					"tool_description": desc,
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
			Agent:      a.Name(),
			Status:     protocol.ResultStatusDegraded,
			Summary:    "日志工具已执行，但只拿到原始输出，建议检查 MCP 工具的结构化返回格式。",
			Confidence: 0.42,
			Evidence: []protocol.EvidenceItem{
				{
					SourceType: "log-raw",
					SourceID:   "raw-output",
					Title:      "日志原始输出",
					Snippet:    shorten(strings.Join(rawOutputs, " | "), 240),
					Score:      0.44,
				},
			},
			Metadata: map[string]any{
				"tool_names":  toolNames,
				"tool_errors": toolErrors,
			},
		}, nil
	}

	summary := fmt.Sprintf("发现 %d 个日志相关 MCP 工具，但没有获取到可用日志证据。", len(toolNames))
	if len(toolErrors) > 0 {
		summary += " 工具错误：" + strings.Join(toolErrors, " ; ")
	}
	return degradedLogResult(task, a.Name(), summary, toolNames, toolErrors), nil
}

func degradedLogResult(task *protocol.TaskEnvelope, agentName, summary string, toolNames []string, toolErrors []string) *protocol.TaskResult {
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      agentName,
		Status:     protocol.ResultStatusDegraded,
		Summary:    summary,
		Confidence: 0.28,
		Metadata: map[string]any{
			"tool_names":  toolNames,
			"tool_errors": toolErrors,
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
			Title:      fmt.Sprintf("%s 日志证据 %d", sourceName, idx+1),
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
