package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	agentcontracts "SuperBizAgent/internal/ai/agent/contracts"
	"SuperBizAgent/internal/ai/contextengine"
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "reporter"

const (
	defaultReporterLLMTimeout = 10 * time.Second
	reporterModeTemplate      = "template"
	reporterModeLLM           = "llm"
	reporterModeAuto          = "auto"
	reporterPrompt            = `你是 AIOps 多智能体诊断报告生成器。请基于用户问题和各专业代理结果，输出中文 Markdown 报告。

要求：
- 先给诊断结论，再列关键依据，最后给建议动作
- 只基于输入中的结果，不要编造不存在的实时证据
- 如果证据不足，明确写出证据不足和下一步需要补充什么

输入：
%s`
)

type Agent struct{}

var (
	reporterMode = func(ctx context.Context) string {
		v, err := g.Cfg().Get(ctx, "multi_agent.reporter_mode")
		if err == nil && strings.TrimSpace(v.String()) != "" {
			return strings.ToLower(strings.TrimSpace(v.String()))
		}
		return reporterModeTemplate
	}
	reporterLLMTimeout = func(ctx context.Context) time.Duration {
		v, err := g.Cfg().Get(ctx, "multi_agent.reporter_llm_timeout_ms")
		if err == nil && v.Int64() > 0 {
			return time.Duration(v.Int64()) * time.Millisecond
		}
		return defaultReporterLLMTimeout
	}
	synthesizeReportWithLLM = defaultSynthesizeReportWithLLM
)

func New() *Agent {
	return &Agent{}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return []string{"aggregation", "reporting", agentcontracts.Capability(AgentName)}
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	raw, _ := task.Input["results"].([]*protocol.TaskResult)
	query, _ := task.Input["query"].(string)
	intent, _ := task.Input["intent"].(string)
	responseMode, _ := task.Input["response_mode"].(string)
	english := prefersEnglishResponse(query)
	contextPkg, contextDetail := buildReporterContext(ctx, task, raw, query)
	degradationReasons := collectDegradationReasons(raw)
	status := aggregateResultStatus(raw)
	degradationReason := strings.Join(degradationReasons, " ; ")

	if responseMode == "chat" {
		result := buildChatResponse(task, raw, query, intent, contextPkg, english)
		result.Metadata = mergeMetadata(result.Metadata, map[string]any{
			"context_detail":      contextDetail,
			"tool_item_count":     len(contextPkg.ToolItems),
			"degradation_reasons": degradationReasons,
		})
		result.Status = status
		result.DegradationReason = degradationReason
		return agentcontracts.AttachMetadata(result, AgentName), nil
	}

	result := buildTemplateReportResponse(task, raw, query, intent, contextDetail, contextPkg, degradationReasons, status, degradationReason, english)
	mode := normalizeReporterMode(reporterMode(ctx), raw)
	if mode == reporterModeLLM {
		if summary, err := synthesizeReportWithLLM(ctx, query, intent, raw); err == nil && strings.TrimSpace(summary) != "" {
			result.Summary = strings.TrimSpace(summary)
			result.Metadata = mergeMetadata(result.Metadata, map[string]any{
				"reporter_mode":   mode,
				"reporter_source": "llm",
			})
		} else {
			llmErr := ""
			if err != nil {
				llmErr = err.Error()
			}
			result.Metadata = mergeMetadata(result.Metadata, map[string]any{
				"reporter_mode":      mode,
				"reporter_source":    "template_fallback",
				"reporter_llm_error": llmErr,
			})
		}
	} else {
		result.Metadata = mergeMetadata(result.Metadata, map[string]any{
			"reporter_mode":   mode,
			"reporter_source": "template",
		})
	}
	return agentcontracts.AttachMetadata(result, AgentName), nil
}

func buildTemplateReportResponse(task *protocol.TaskEnvelope, raw []*protocol.TaskResult, query, intent string, contextDetail []string, contextPkg *contextengine.ContextPackage, degradationReasons []string, status protocol.ResultStatus, degradationReason string, english bool) *protocol.TaskResult {
	text := reporterText(english)
	sections := make([]string, 0, len(raw)+6)
	sections = append(sections, text.reportTitle)
	if query != "" {
		sections = append(sections, text.userGoalHeading+"\n"+query)
	}
	sections = append(sections, text.intentHeading+"\n"+fallback(intent, "alert_analysis"))
	sections = append(sections, text.executionSummaryHeading)

	conclusionLines := make([]string, 0, len(raw))
	evidence := make([]protocol.EvidenceItem, 0)
	for _, result := range raw {
		if result == nil {
			continue
		}
		label := displayAgentName(result.Agent, english)
		sections = append(sections, fmt.Sprintf("### %s\n%s", label, fallback(result.Summary, text.noResult)))
		if len(result.Evidence) > 0 {
			evidence = append(evidence, result.Evidence...)
		}
		statusText := fmt.Sprintf("%s%s%s", displayAgentName(result.Agent, english), text.labelSeparator, fallback(result.Summary, text.noResult))
		if result.Status != protocol.ResultStatusSucceeded {
			statusText += text.degradedSuffix
		}
		conclusionLines = append(conclusionLines, statusText)
	}

	if len(degradationReasons) > 0 {
		sections = append(sections, text.degradationHeading)
		sections = append(sections, strings.Join(prefixEach(degradationReasons, "- "), "\n"))
	}

	sections = append(sections, text.conclusionHeading)
	if len(conclusionLines) == 0 {
		sections = append(sections, text.noSpecialistResults)
	} else {
		sections = append(sections, strings.Join(prefixEach(conclusionLines, "- "), "\n"))
	}

	return &protocol.TaskResult{
		TaskID:            task.TaskID,
		Agent:             AgentName,
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
	}
}

func buildChatResponse(task *protocol.TaskEnvelope, raw []*protocol.TaskResult, query, intent string, contextPkg *contextengine.ContextPackage, english bool) *protocol.TaskResult {
	evidence := make([]protocol.EvidenceItem, 0)
	lines := make([]string, 0, len(raw))
	degradationReasons := collectDegradationReasons(raw)
	text := reporterText(english)

	for _, result := range raw {
		if result == nil {
			continue
		}
		if len(result.Evidence) > 0 {
			evidence = append(evidence, result.Evidence...)
		}
		label := displayAgentName(result.Agent, english)
		line := fmt.Sprintf("%s%s%s", label, text.labelSeparator, fallback(result.Summary, text.noResult))
		if result.Status != protocol.ResultStatusSucceeded {
			line += text.degradedSuffix
		}
		lines = append(lines, line)
	}

	parts := make([]string, 0, 5)
	switch intent {
	case "kb_qa":
		parts = append(parts, text.kbIntro)
	case "incident_analysis":
		parts = append(parts, text.incidentIntro)
	default:
		parts = append(parts, text.defaultIntro)
	}
	if strings.TrimSpace(query) != "" {
		parts = append(parts, text.questionPrefix+query)
	}
	if len(lines) == 0 {
		parts = append(parts, text.noSpecialistResults)
	} else {
		parts = append(parts, strings.Join(prefixEach(lines, "- "), "\n"))
	}
	toolSnippets := contextengine.ToolItemSnippets(contextPkg.ToolItems, 3)
	if len(toolSnippets) > 0 {
		parts = append(parts, text.evidenceHeading+"\n"+strings.Join(prefixEach(toolSnippets, "- "), "\n"))
	} else if len(evidence) > 0 {
		parts = append(parts, text.evidenceHeading+"\n"+strings.Join(prefixEach(chatEvidenceSnippets(evidence, 3), "- "), "\n"))
	}
	if len(degradationReasons) > 0 {
		parts = append(parts, text.partialDegradationHeading+"\n"+strings.Join(prefixEach(degradationReasons, "- "), "\n"))
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

func defaultSynthesizeReportWithLLM(ctx context.Context, query, intent string, raw []*protocol.TaskResult) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, reporterLLMTimeout(ctx))
	defer cancel()

	payload, err := json.Marshal(map[string]any{
		"query":   query,
		"intent":  intent,
		"results": compactReportResults(raw),
	})
	if err != nil {
		return "", err
	}
	chatModel, err := models.OpenAIForDeepSeekV3Quick(callCtx)
	if err != nil {
		return "", err
	}
	resp, err := chatModel.Generate(callCtx, []*schema.Message{
		{Role: schema.System, Content: "你只输出诊断报告正文，不输出额外解释。"},
		{Role: schema.User, Content: fmt.Sprintf(reporterPrompt, string(payload))},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func compactReportResults(raw []*protocol.TaskResult) []map[string]any {
	out := make([]map[string]any, 0, len(raw))
	for _, result := range raw {
		if result == nil {
			continue
		}
		out = append(out, map[string]any{
			"agent":              result.Agent,
			"status":             result.Status,
			"summary":            result.Summary,
			"confidence":         result.Confidence,
			"degradation_reason": result.DegradationReason,
			"evidence":           chatEvidenceSnippets(result.Evidence, 3),
		})
	}
	return out
}

func normalizeReporterMode(mode string, raw []*protocol.TaskResult) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case reporterModeLLM:
		return reporterModeLLM
	case reporterModeAuto:
		if len(raw) >= 2 {
			return reporterModeLLM
		}
		return reporterModeTemplate
	default:
		return reporterModeTemplate
	}
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

func displayAgentName(name string, english bool) string {
	if !english {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "metrics":
			return "指标"
		case "logs":
			return "日志"
		case "knowledge":
			return "知识库"
		case "triage":
			return "分诊"
		case "supervisor":
			return "调度"
		case "reporter":
			return "汇总"
		}
	}
	if name == "" {
		if english {
			return "Unknown"
		}
		return "未知"
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

type reporterLocalization struct {
	reportTitle               string
	userGoalHeading           string
	intentHeading             string
	executionSummaryHeading   string
	degradationHeading        string
	conclusionHeading         string
	noSpecialistResults       string
	kbIntro                   string
	incidentIntro             string
	defaultIntro              string
	questionPrefix            string
	evidenceHeading           string
	partialDegradationHeading string
	labelSeparator            string
	degradedSuffix            string
	noResult                  string
}

func reporterText(english bool) reporterLocalization {
	if english {
		return reporterLocalization{
			reportTitle:               "# Alert Analysis Report",
			userGoalHeading:           "## User Goal",
			intentHeading:             "## Intent",
			executionSummaryHeading:   "## Execution Summary",
			degradationHeading:        "## Degradation",
			conclusionHeading:         "## Conclusion",
			noSpecialistResults:       "No specialist results were available. Check tool configuration or retry later.",
			kbIntro:                   "I checked the currently available knowledge and tool results.",
			incidentIntro:             "I combined the currently available troubleshooting signals.",
			defaultIntro:              "I combined the currently available multi-agent results.",
			questionPrefix:            "Question: ",
			evidenceHeading:           "Evidence:",
			partialDegradationHeading: "Partial degradation:",
			labelSeparator:            ": ",
			degradedSuffix:            " (degraded)",
			noResult:                  "no result",
		}
	}
	return reporterLocalization{
		reportTitle:               "# 告警分析报告",
		userGoalHeading:           "## 用户目标",
		intentHeading:             "## 意图",
		executionSummaryHeading:   "## 执行摘要",
		degradationHeading:        "## 降级信息",
		conclusionHeading:         "## 结论",
		noSpecialistResults:       "当前没有可用的专业代理结果，请检查工具配置、依赖服务或稍后重试。",
		kbIntro:                   "我已结合当前可用的知识和工具结果。",
		incidentIntro:             "我已综合当前可用的排障信号。",
		defaultIntro:              "我已汇总当前可用的多智能体结果。",
		questionPrefix:            "问题：",
		evidenceHeading:           "依据：",
		partialDegradationHeading: "部分降级：",
		labelSeparator:            "：",
		degradedSuffix:            "（已降级）",
		noResult:                  "暂无结果",
	}
}

func prefersEnglishResponse(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return false
	}
	for _, token := range []string{
		"answer in english",
		"respond in english",
		"reply in english",
		"please use english",
		"please answer in english",
		"in english",
		"用英文",
		"请用英文",
		"英文回答",
		"英文回复",
		"英语回答",
		"英语回复",
		"英文输出",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
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
