package plan_execute_replan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/gogf/gf/v2/frame/g"
)

type StageEmitter func(context.Context, string, map[string]any)

type stageEmitterContextKey struct{}

func WithStageEmitter(ctx context.Context, emit StageEmitter) context.Context {
	if ctx == nil || emit == nil {
		return ctx
	}
	return context.WithValue(ctx, stageEmitterContextKey{}, emit)
}

func BuildPlanAgent(ctx context.Context, query string) (string, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	emitPlanStage(ctx, "planning", "正在制定排障计划", nil)

	planAgent, err := NewPlanner(ctx)
	if err != nil {
		return "", []string{}, err
	}
	executeAgent, err := NewExecutor(ctx)
	if err != nil {
		return "", []string{}, err
	}
	replanAgent, err := NewRePlanAgent(ctx)
	if err != nil {
		return "", []string{}, err
	}
	planExecuteAgent, err := planexecute.New(ctx, &planexecute.Config{
		Planner:       planAgent,
		Executor:      executeAgent,
		Replanner:     replanAgent,
		MaxIterations: 5,
	})
	if err != nil {
		return "", []string{}, fmt.Errorf("build PlanExecuteAgent Error: %w", err)
	}
	r := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: planExecuteAgent,
	})
	iter := r.Query(ctx, queryWithFinalReportRequirement(query))
	return consumePlanEvents(ctx, iter.Next)
}

func consumePlanEvents(ctx context.Context, next func() (*adk.AgentEvent, bool)) (string, []string, error) {
	var detail []string
	for {
		event, ok := next()
		if !ok {
			break
		}
		if event.Output == nil {
			continue
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			continue
		}
		g.Log().Debugf(ctx, "[AIOps] step: %s", msg.String())
		if item := planDetailMessage(msg); item != "" {
			detail = append(detail, item)
			emitPlanMessageStage(ctx, msg)
		}

		if analysis := analysisMessageContent(msg); analysis != "" {
			return analysis, detail, nil
		}
	}

	if fallback := fallbackAnalysisFromDetails(detail); fallback != "" {
		emitPlanStage(ctx, "report_degraded", "Plan 已生成降级诊断报告", nil)
		return fallback, detail, fmt.Errorf("no analysis conclusion found in event stream")
	}
	emitPlanStage(ctx, "report_failed", "Plan 未生成诊断结论", nil)
	return "", detail, fmt.Errorf("no analysis conclusion found in event stream")
}

func emitPlanMessageStage(ctx context.Context, msg adk.Message) {
	stage, message, payload := planStageEvent(msg)
	if stage == "" || message == "" {
		return
	}
	emitPlanStage(ctx, stage, message, payload)
}

func emitPlanStage(ctx context.Context, stage, message string, payload map[string]any) {
	emit, ok := ctx.Value(stageEmitterContextKey{}).(StageEmitter)
	if !ok || emit == nil || strings.TrimSpace(stage) == "" || strings.TrimSpace(message) == "" {
		return
	}
	nextPayload := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		nextPayload[key] = value
	}
	nextPayload["stage"] = strings.TrimSpace(stage)
	emit(ctx, strings.TrimSpace(message), nextPayload)
}

func planStageEvent(msg adk.Message) (string, string, map[string]any) {
	if isPlanJSON(msg.Content) {
		count := planStepCount(msg.Content)
		if count > 0 {
			return "plan_ready", fmt.Sprintf("已生成 %d 个排障步骤", count), map[string]any{"step_count": count}
		}
		return "plan_ready", "已生成排障步骤", nil
	}
	if analysisMessageContent(msg) != "" {
		return "report_ready", "诊断报告已生成", nil
	}
	if len(msg.ToolCalls) > 0 {
		return "evidence_running", "按计划执行证据检查", map[string]any{"tool_count": len(msg.ToolCalls)}
	}
	if msg.Role == "tool" {
		return "evidence_ready", "证据检查已返回", nil
	}
	return "", "", nil
}

func queryWithFinalReportRequirement(query string) string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		trimmed = "请分析当前 AIOps 告警和系统现象。"
	}
	return trimmed + "\n\n输出要求：完成计划执行后，最后必须输出一份中文 Markdown 诊断报告，包含：现象、已检查证据、初步判断、下一步建议。如果证据不足，请明确说明缺口，不要只输出工具调用、计划 JSON 或空响应。"
}

func fallbackAnalysisFromDetails(detail []string) string {
	signals := compactPlanDetails(detail, 5)
	if len(signals) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## 诊断报告\n\n")
	b.WriteString("Plan 链路收集到了执行事件，但模型没有返回独立的最终结论。下面是根据执行过程整理的降级报告。\n\n")
	b.WriteString("### 已观察到的事件\n")
	for _, signal := range signals {
		b.WriteString("- ")
		b.WriteString(signal)
		b.WriteString("\n")
	}
	b.WriteString("\n### 初步判断\n")
	if detailsContain(detail, "llm_stream_failed") {
		b.WriteString("LLM 流式响应失败，当前无法完成完整的 Plan 总结。")
	} else {
		b.WriteString("当前执行事件不足以形成确定根因，需要补充更多可验证证据。")
	}
	b.WriteString("\n\n### 下一步建议\n")
	b.WriteString("- 补充服务名、告警时间窗、关键日志片段或指标截图。\n")
	b.WriteString("- 如果是发布后异常，请补充发布单、版本号和回滚窗口。\n")
	b.WriteString("- 重新发起 Plan 排障，或切换 GoS 用候选根因和证据置信度继续收敛。")
	return b.String()
}

func compactPlanDetails(detail []string, limit int) []string {
	out := make([]string, 0, limit)
	seen := make(map[string]bool)
	for _, item := range detail {
		cleaned := strings.Join(strings.Fields(strings.TrimSpace(item)), " ")
		if cleaned == "" || seen[cleaned] {
			continue
		}
		if len([]rune(cleaned)) > 180 {
			cleaned = string([]rune(cleaned)[:180]) + "..."
		}
		out = append(out, cleaned)
		seen[cleaned] = true
		if len(out) >= limit {
			break
		}
	}
	return out
}

func detailsContain(detail []string, needle string) bool {
	for _, item := range detail {
		if strings.Contains(strings.ToLower(item), needle) {
			return true
		}
	}
	return false
}

func isAnalysisMessage(msg adk.Message) bool {
	return analysisMessageContent(msg) != ""
}

func analysisMessageContent(msg adk.Message) string {
	if msg.Role != "assistant" {
		return ""
	}
	if len(msg.Content) < 20 {
		return ""
	}
	if len(msg.ToolCalls) > 0 {
		return ""
	}
	if isPlanJSON(msg.Content) {
		return ""
	}
	if response := planResponseContent(msg.Content); response != "" {
		return response
	}
	return strings.TrimSpace(msg.Content)
}

func isPlanJSON(content string) bool {
	s := strings.TrimSpace(content)
	if len(s) < 2 {
		return false
	}
	if s[0] != '{' {
		return false
	}
	return len(s) > 10 && (strings.Contains(s, `"steps"`) || strings.Contains(s, `"plan"`))
}

func planDetailMessage(msg adk.Message) string {
	if msg.Role != "assistant" {
		return strings.TrimSpace(msg.String())
	}
	if isPlanJSON(msg.Content) {
		if count := planStepCount(msg.Content); count > 0 {
			return fmt.Sprintf("Plan generated %d troubleshooting steps", count)
		}
		return "Plan generated troubleshooting steps"
	}
	if analysisMessageContent(msg) != "" {
		return "Plan generated final diagnostic report"
	}
	return strings.TrimSpace(msg.String())
}

func planResponseContent(content string) string {
	var payload map[string]json.RawMessage
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &payload) != nil {
		return ""
	}
	for _, key := range []string{"response", "answer", "report"} {
		var value string
		if raw, ok := payload[key]; ok && json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func planStepCount(content string) int {
	var payload map[string]json.RawMessage
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &payload) != nil {
		return 0
	}
	for _, key := range []string{"steps", "plan"} {
		var values []string
		if raw, ok := payload[key]; ok && json.Unmarshal(raw, &values) == nil {
			return len(values)
		}
	}
	return 0
}
