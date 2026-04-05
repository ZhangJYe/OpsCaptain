package reporter

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"SuperBizAgent/internal/ai/contextengine"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

const AgentName = "reporter"

type Agent struct{}

func New() *Agent {
	return &Agent{}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return []string{"aggregation", "reporting"}
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	raw, _ := task.Input["results"].([]*protocol.TaskResult)
	query, _ := task.Input["query"].(string)
	intent, _ := task.Input["intent"].(string)
	responseMode, _ := task.Input["response_mode"].(string)
	contextPkg, contextDetail := buildReporterContext(ctx, task, raw, query)

	if responseMode == "chat" {
		result := buildChatResponse(task, raw, query, intent, contextPkg)
		result.Metadata = mergeMetadata(result.Metadata, map[string]any{
			"context_detail":  contextDetail,
			"tool_item_count": len(contextPkg.ToolItems),
		})
		return result, nil
	}

	sections := make([]string, 0, len(raw)+4)
	sections = append(sections, "# 告警分析报告")
	if query != "" {
		sections = append(sections, "## 用户目标\n"+query)
	}
	sections = append(sections, "## 任务类型\n"+fallback(intent, "alert_analysis"))
	sections = append(sections, "## 执行摘要")

	conclusionLines := make([]string, 0, len(raw))
	evidence := make([]protocol.EvidenceItem, 0)
	for _, result := range raw {
		if result == nil {
			continue
		}
		label := displayAgentName(result.Agent)
		sections = append(sections, fmt.Sprintf("### %s\n%s", label, fallback(result.Summary, "无结果")))
		if len(result.Evidence) > 0 {
			evidence = append(evidence, result.Evidence...)
		}
		statusText := fmt.Sprintf("%s: %s", result.Agent, result.Summary)
		if result.Status == protocol.ResultStatusDegraded {
			statusText += "（降级）"
		}
		conclusionLines = append(conclusionLines, statusText)
	}

	sections = append(sections, "## 结论")
	if len(conclusionLines) == 0 {
		sections = append(sections, "当前没有足够的子任务结果，建议检查工具配置与依赖服务。")
	} else {
		sections = append(sections, strings.Join(prefixEach(conclusionLines, "- "), "\n"))
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    strings.Join(sections, "\n\n"),
		Confidence: deriveConfidence(raw),
		Evidence:   evidence,
		Metadata: map[string]any{
			"context_detail":  contextDetail,
			"tool_item_count": len(contextPkg.ToolItems),
		},
	}, nil
}

func buildChatResponse(task *protocol.TaskEnvelope, raw []*protocol.TaskResult, query, intent string, contextPkg *contextengine.ContextPackage) *protocol.TaskResult {
	evidence := make([]protocol.EvidenceItem, 0)
	lines := make([]string, 0, len(raw))

	for _, result := range raw {
		if result == nil {
			continue
		}
		if len(result.Evidence) > 0 {
			evidence = append(evidence, result.Evidence...)
		}
		label := displayAgentName(result.Agent)
		line := fmt.Sprintf("%s：%s", label, fallback(result.Summary, "无结果"))
		if result.Status == protocol.ResultStatusDegraded {
			line += "（降级）"
		}
		lines = append(lines, line)
	}

	parts := make([]string, 0, 4)
	switch intent {
	case "kb_qa":
		parts = append(parts, "我查询了当前可用的知识与工具结果：")
	case "incident_analysis":
		parts = append(parts, "我结合当前可用的排障结果做了分析：")
	default:
		parts = append(parts, "我结合当前可用的多智能体结果做了分析：")
	}
	if strings.TrimSpace(query) != "" {
		parts = append(parts, "问题："+query)
	}
	if len(lines) == 0 {
		parts = append(parts, "当前没有拿到足够的子任务结果，建议检查工具配置或稍后重试。")
	} else {
		parts = append(parts, strings.Join(prefixEach(lines, "- "), "\n"))
	}
	toolSnippets := contextengine.ToolItemSnippets(contextPkg.ToolItems, 3)
	if len(toolSnippets) > 0 {
		parts = append(parts, "可参考证据：\n"+strings.Join(prefixEach(toolSnippets, "- "), "\n"))
	} else if len(evidence) > 0 {
		parts = append(parts, "可参考证据：\n"+strings.Join(prefixEach(chatEvidenceSnippets(evidence, 3), "- "), "\n"))
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      AgentName,
		Status:     protocol.ResultStatusSucceeded,
		Summary:    strings.Join(parts, "\n\n"),
		Confidence: deriveConfidence(raw),
		Evidence:   evidence,
	}
}

func buildReporterContext(ctx context.Context, task *protocol.TaskEnvelope, raw []*protocol.TaskResult, query string) (*contextengine.ContextPackage, []string) {
	items := contextengine.ToolItemsFromResults(raw)
	assembler := contextengine.NewAssembler()
	pkg, err := assembler.Assemble(ctx, contextengine.ContextRequest{
		Mode:      "reporter",
		Query:     query,
		ToolItems: items,
	}, nil)
	if err != nil {
		return &contextengine.ContextPackage{}, []string{fmt.Sprintf("reporter context assemble failed: %v", err)}
	}

	detail := contextengine.TraceDetails(pkg.Trace)
	if rt, ok := runtime.FromContext(ctx); ok {
		for _, line := range detail {
			rt.EmitInfo(ctx, task, AgentName, line, map[string]any{"component": "reporter_context"})
		}
	}
	return pkg, detail
}

func mergeMetadata(left, right map[string]any) map[string]any {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	out := make(map[string]any, len(left)+len(right))
	for k, v := range left {
		out[k] = v
	}
	for k, v := range right {
		out[k] = v
	}
	return out
}

func displayAgentName(name string) string {
	if name == "" {
		return "Unknown"
	}
	r, size := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError {
		return name
	}
	return string(unicode.ToUpper(r)) + name[size:]
}

func fallback(value, alt string) string {
	if strings.TrimSpace(value) == "" {
		return alt
	}
	return value
}

func prefixEach(items []string, prefix string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, prefix+item)
	}
	return out
}

func deriveConfidence(results []*protocol.TaskResult) float64 {
	if len(results) == 0 {
		return 0.2
	}
	total := 0.0
	count := 0.0
	for _, result := range results {
		if result == nil {
			continue
		}
		total += result.Confidence
		count++
	}
	if count == 0 {
		return 0.2
	}
	return total / count
}

func chatEvidenceSnippets(items []protocol.EvidenceItem, limit int) []string {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	out := make([]string, 0, min(limit, len(items)))
	for idx, item := range items {
		if idx >= limit {
			break
		}
		label := fallback(item.Title, item.SourceID)
		snippet := fallback(item.Snippet, "无摘要")
		out = append(out, fmt.Sprintf("%s：%s", label, snippet))
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
