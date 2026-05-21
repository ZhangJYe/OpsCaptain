package plan_execute_replan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/gogf/gf/v2/frame/g"
)

func BuildPlanAgent(ctx context.Context, query string) (string, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

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

	var lastAnalysis string
	var detail []string
	for {
		event, ok := iter.Next()
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
		detail = append(detail, msg.String())

		if isAnalysisMessage(msg) {
			lastAnalysis = msg.Content
		}
	}

	if lastAnalysis == "" {
		if fallback := fallbackAnalysisFromDetails(detail); fallback != "" {
			return fallback, detail, fmt.Errorf("no analysis conclusion found in event stream")
		}
		return "", detail, fmt.Errorf("no analysis conclusion found in event stream")
	}
	return lastAnalysis, detail, nil
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
	if msg.Role != "assistant" {
		return false
	}
	if len(msg.Content) < 20 {
		return false
	}
	if len(msg.ToolCalls) > 0 {
		return false
	}
	if isPlanJSON(msg.Content) {
		return false
	}
	return true
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
