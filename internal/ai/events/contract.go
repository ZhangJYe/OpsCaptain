package events

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// ContractLevel 违规等级
type ContractLevel int

const (
	ContractLevelWarn ContractLevel = iota
	ContractLevelViolation
)

// ContractViolation 单条违规
type ContractViolation struct {
	Rule   string
	Level  ContractLevel
	Detail string
}

func (v ContractViolation) String() string {
	prefix := "WARN"
	if v.Level == ContractLevelViolation {
		prefix = "VIOLATION"
	}
	return fmt.Sprintf("[%s][%s] %s", prefix, v.Rule, v.Detail)
}

// ContractResult 校验结果
type ContractResult struct {
	Violations []ContractViolation
	Passed     bool
}

func (r *ContractResult) AddViolation(rule, detail string) {
	r.Violations = append(r.Violations, ContractViolation{
		Rule:   rule,
		Level:  ContractLevelViolation,
		Detail: detail,
	})
	r.Passed = false
}

func (r *ContractResult) AddWarn(rule, detail string) {
	r.Violations = append(r.Violations, ContractViolation{
		Rule:   rule,
		Level:  ContractLevelWarn,
		Detail: detail,
	})
}

func (r *ContractResult) Summary() string {
	if r.Passed && len(r.Violations) == 0 {
		return "contract: passed"
	}
	var sb strings.Builder
	if r.Passed {
		sb.WriteString("contract: passed with warnings\n")
	} else {
		sb.WriteString("contract: FAILED\n")
	}
	for _, v := range r.Violations {
		sb.WriteString("  " + v.String() + "\n")
	}
	return sb.String()
}

// ValidateContract 对 ReAct 路径输出做轻量级 Contract 校验
func ValidateContract(output string, toolResults []string, failedTools []string, hasToolCalls bool) *ContractResult {
	return ValidateContractWithConfig(nil, output, toolResults, failedTools, hasToolCalls)
}

// ValidateContractWithConfig 带配置的 Contract 校验
func ValidateContractWithConfig(hc *HallucinationConfig, output string, toolResults []string, failedTools []string, hasToolCalls bool) *ContractResult {
	result := &ContractResult{Passed: true}

	// 规则 1: 运维问题必须有工具调用
	if !hasToolCalls {
		result.AddViolation("must_use_tools", "运维相关问题未调用任何工具，输出可能为幻觉")
	}

	// 规则 2: 工具失败必须告知用户
	if len(failedTools) > 0 {
		for _, ft := range failedTools {
			if !strings.Contains(output, ft) && !strings.Contains(output, "失败") && !strings.Contains(output, "错误") {
				result.AddViolation("must_report_failure", fmt.Sprintf("工具 %s 调用失败，但输出中未提及", ft))
			}
		}
	}

	// 规则 3: 有工具结果时，输出必须引用数据
	if len(toolResults) > 0 {
		combined := strings.Join(toolResults, " ")
		hasReference := false
		for _, tr := range toolResults {
			snippet := tr
			if len(snippet) > 50 {
				snippet = snippet[:50]
			}
			words := strings.Fields(snippet)
			matchCount := 0
			for _, w := range words {
				if len(w) >= 2 && strings.Contains(output, w) {
					matchCount++
				}
			}
			if matchCount >= 2 {
				hasReference = true
				break
			}
		}
		_ = combined
		if !hasReference {
			result.AddWarn("should_reference_data", "有工具返回数据，但输出中未明显引用工具结果")
		}
	}

	// 规则 4: 编造告警检测
	fabricatedAlertPattern := regexp.MustCompile(`(?i)(告警|alert)\s*[:：]?\s*[^\n]{5,50}(触发|firing|resolved|pending)`)
	if fabricatedAlertPattern.MatchString(output) && len(toolResults) == 0 {
		result.AddViolation("no_fabricated_alerts", "输出中包含告警描述，但无工具返回告警数据")
	}

	// 规则 5: 置信度标注（软约束）
	hedgeWords := []string{"推测", "可能", "大概", "不确定", "待确认", "需要进一步", "建议确认", "仅供参考"}
	if hc != nil && len(hc.HedgeWords) > 0 {
		hedgeWords = hc.HedgeWords
	}
	hasHedge := false
	for _, hw := range hedgeWords {
		if strings.Contains(output, hw) {
			hasHedge = true
			break
		}
	}
	if len(toolResults) == 0 && !hasHedge && hasToolCalls {
		result.AddWarn("should_hedge", "无工具数据支持且未使用置信度标注词")
	}

	return result
}

// ContractCollector 实现 Emitter，收集事件用于 Contract 校验
type ContractCollector struct {
	mu           sync.Mutex
	toolResults  []string
	failedTools  []string
	hasToolCalls bool
}

func NewContractCollector() *ContractCollector {
	return &ContractCollector{}
}

func (c *ContractCollector) Emit(_ context.Context, event AgentEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event.Type {
	case EventToolCallEnd:
		c.hasToolCalls = true
		toolName, _ := event.Payload["tool_name"].(string)
		summary, _ := event.Payload["summary"].(string)
		errMsg, _ := event.Payload["error"].(string)
		if toolName == "" {
			return
		}
		if errMsg != "" {
			c.failedTools = append(c.failedTools, toolName)
		} else if summary != "" {
			c.toolResults = append(c.toolResults, summary)
		}
	}
}

func (c *ContractCollector) ToolResults() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := make([]string, len(c.toolResults))
	copy(r, c.toolResults)
	return r
}

func (c *ContractCollector) FailedTools() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := make([]string, len(c.failedTools))
	copy(r, c.failedTools)
	return r
}

func (c *ContractCollector) HasToolCalls() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasToolCalls
}
