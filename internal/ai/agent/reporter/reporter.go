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
	degradationReasons := collectDegradationReasons(raw)
	status := aggregateResultStatus(raw)
	degradationReason := strings.Join(degradationReasons, " ; ")

	if responseMode == "chat" {
		result := buildChatResponse(task, raw, query, intent, contextPkg)
		result.Metadata = mergeMetadata(result.Metadata, map[string]any{
			"context_detail":      contextDetail,
			"tool_item_count":     len(contextPkg.ToolItems),
			"degradation_reasons": degradationReasons,
		})
		result.Status = status
		result.DegradationReason = degradationReason
		return result, nil
	}

	sections := make([]string, 0, len(raw)+6)
	sections = append(sections, "# Alert Analysis Report")
	if query != "" {
		sections = append(sections, "## User Goal\n"+query)
	}
	sections = append(sections, "## Intent\n"+fallback(intent, "alert_analysis"))
	sections = append(sections, "## Execution Summary")

	conclusionLines := make([]string, 0, len(raw))
	evidence := make([]protocol.EvidenceItem, 0)
	for _, result := range raw {
		if result == nil {
			continue
		}
		label := displayAgentName(result.Agent)
		sections = append(sections, fmt.Sprintf("### %s\n%s", label, fallback(result.Summary, "no result")))
		if len(result.Evidence) > 0 {
			evidence = append(evidence, result.Evidence...)
		}
		statusText := fmt.Sprintf("%s: %s", result.Agent, fallback(result.Summary, "no result"))
		if result.Status != protocol.ResultStatusSucceeded {
			statusText += " (degraded)"
		}
		conclusionLines = append(conclusionLines, statusText)
	}

	if len(degradationReasons) > 0 {
		sections = append(sections, "## Degradation")
		sections = append(sections, strings.Join(prefixEach(degradationReasons, "- "), "\n"))
	}

	sections = append(sections, "## Conclusion")
	if len(conclusionLines) == 0 {
		sections = append(sections, "No specialist results were available. Check tool configuration and dependent services.")
	} else {
		sections = append(sections, strings.Join(prefixEach(conclusionLines, "- "), "\n"))
	}

	return &protocol.TaskResult{
		TaskID:            task.TaskID,
		Agent:             a.Name(),
		Status:            status,
		Summary:           strings.Join(sections, "\n\n"),
		Confidence:        deriveConfidence(raw),
		DegradationReason: degradationReason,
		Evidence:          evidence,
		Metadata: map[string]any{
			"context_detail":      contextDetail,
			"tool_item_count":     len(contextPkg.ToolItems),
			"degradation_reasons": degradationReasons,
		},
	}, nil
}

func buildChatResponse(task *protocol.TaskEnvelope, raw []*protocol.TaskResult, query, intent string, contextPkg *contextengine.ContextPackage) *protocol.TaskResult {
	evidence := make([]protocol.EvidenceItem, 0)
	lines := make([]string, 0, len(raw))
	degradationReasons := collectDegradationReasons(raw)

	for _, result := range raw {
		if result == nil {
			continue
		}
		if len(result.Evidence) > 0 {
			evidence = append(evidence, result.Evidence...)
		}
		label := displayAgentName(result.Agent)
		line := fmt.Sprintf("%s: %s", label, fallback(result.Summary, "no result"))
		if result.Status != protocol.ResultStatusSucceeded {
			line += " (degraded)"
		}
		lines = append(lines, line)
	}

	parts := make([]string, 0, 5)
	switch intent {
	case "kb_qa":
		parts = append(parts, "I checked the currently available knowledge and tool results.")
	case "incident_analysis":
		parts = append(parts, "I combined the currently available troubleshooting signals.")
	default:
		parts = append(parts, "I combined the currently available multi-agent results.")
	}
	if strings.TrimSpace(query) != "" {
		parts = append(parts, "Question: "+query)
	}
	if len(lines) == 0 {
		parts = append(parts, "No specialist results were available. Check tool configuration or retry later.")
	} else {
		parts = append(parts, strings.Join(prefixEach(lines, "- "), "\n"))
	}
	toolSnippets := contextengine.ToolItemSnippets(contextPkg.ToolItems, 3)
	if len(toolSnippets) > 0 {
		parts = append(parts, "Evidence:\n"+strings.Join(prefixEach(toolSnippets, "- "), "\n"))
	} else if len(evidence) > 0 {
		parts = append(parts, "Evidence:\n"+strings.Join(prefixEach(chatEvidenceSnippets(evidence, 3), "- "), "\n"))
	}
	if len(degradationReasons) > 0 {
		parts = append(parts, "Partial degradation:\n"+strings.Join(prefixEach(degradationReasons, "- "), "\n"))
	}

	return &protocol.TaskResult{
		TaskID:            task.TaskID,
		Agent:             AgentName,
		Status:            aggregateResultStatus(raw),
		Summary:           strings.Join(parts, "\n\n"),
		Confidence:        deriveConfidence(raw),
		DegradationReason: strings.Join(degradationReasons, " ; "),
		Evidence:          evidence,
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
		snippet := fallback(item.Snippet, "no snippet")
		out = append(out, fmt.Sprintf("%s: %s", label, snippet))
	}
	return out
}

func aggregateResultStatus(results []*protocol.TaskResult) protocol.ResultStatus {
	if len(results) == 0 {
		return protocol.ResultStatusDegraded
	}
	for _, result := range results {
		if result == nil || result.Status != protocol.ResultStatusSucceeded {
			return protocol.ResultStatusDegraded
		}
	}
	return protocol.ResultStatusSucceeded
}

func collectDegradationReasons(results []*protocol.TaskResult) []string {
	if len(results) == 0 {
		return []string{"no specialist results available"}
	}

	seen := make(map[string]struct{}, len(results))
	reasons := make([]string, 0, len(results))
	for _, result := range results {
		if result == nil || result.Status == protocol.ResultStatusSucceeded {
			continue
		}
		reason := strings.TrimSpace(result.DegradationReason)
		if reason == "" && result.Error != nil {
			reason = strings.TrimSpace(result.Error.Message)
		}
		if reason == "" {
			reason = strings.TrimSpace(result.Summary)
		}
		if reason == "" {
			continue
		}
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		reasons = append(reasons, reason)
	}
	return reasons
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
