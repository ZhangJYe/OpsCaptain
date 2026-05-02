package events

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// SchemaField 输出 schema 中的字段定义
type SchemaField struct {
	Name        string
	Required    bool
	Validator   func(string) (bool, string)
	Description string
}

// SchemaGate 输出 schema 校验引擎
type SchemaGate struct {
	mu     sync.Mutex
	fields []SchemaField
}

func NewSchemaGate() *SchemaGate {
	return NewSchemaGateWithConfig(nil)
}

func NewSchemaGateWithConfig(hc *HallucinationConfig) *SchemaGate {
	g := &SchemaGate{}
	g.registerDefaults(hc)
	return g
}

func (g *SchemaGate) registerDefaults(hc *HallucinationConfig) {
	contradictions := [][2]string{
		{"正常", "异常"},
		{"健康", "故障"},
		{"无告警", "告警"},
		{"没有问题", "问题"},
	}
	problemWords := []string{"异常", "故障", "错误", "告警", "升高", "下降", "超时", "失败"}
	actionWords := []string{"建议", "推荐", "可以", "需要", "排查", "检查", "尝试", "应该"}

	if hc != nil {
		if len(hc.Contradictions) > 0 {
			contradictions = hc.Contradictions
		}
		if len(hc.ProblemWords) > 0 {
			problemWords = hc.ProblemWords
		}
		if len(hc.ActionWords) > 0 {
			actionWords = hc.ActionWords
		}
	}

	localContradictions := contradictions
	localProblemWords := problemWords
	localActionWords := actionWords

	g.fields = []SchemaField{
		{
			Name:     "has_answer",
			Required: true,
			Validator: func(output string) (bool, string) {
				if len(strings.TrimSpace(output)) < 10 {
					return false, "输出过短，可能未提供有效回答"
				}
				return true, ""
			},
			Description: "输出必须包含实质性回答",
		},
		{
			Name:     "no_contradiction",
			Required: false,
			Validator: func(output string) (bool, string) {
				lower := strings.ToLower(output)
				for _, c := range localContradictions {
					if strings.Contains(lower, c[0]) && strings.Contains(lower, c[1]) {
						return false, fmt.Sprintf("输出可能自相矛盾：同时包含'%s'和'%s'", c[0], c[1])
					}
				}
				return true, ""
			},
			Description: "输出不应自相矛盾",
		},
		{
			Name:     "actionable",
			Required: false,
			Validator: func(output string) (bool, string) {
				hasProblem := false
				hasAction := false
				for _, w := range localProblemWords {
					if strings.Contains(output, w) {
						hasProblem = true
						break
					}
				}
				for _, w := range localActionWords {
					if strings.Contains(output, w) {
						hasAction = true
						break
					}
				}
				if hasProblem && !hasAction {
					return false, "输出描述了问题但未提供可操作建议"
				}
				return true, ""
			},
			Description: "描述问题时应提供可操作建议",
		},
	}
}

// Validate 对输出执行 schema 校验
func (g *SchemaGate) Validate(output string) *SchemaResult {
	g.mu.Lock()
	defer g.mu.Unlock()

	result := &SchemaResult{
		Passed: true,
		Checks: make([]SchemaCheck, 0, len(g.fields)),
	}

	for _, field := range g.fields {
		check := SchemaCheck{
			Field:       field.Name,
			Description: field.Description,
		}
		if field.Validator != nil {
			passed, detail := field.Validator(output)
			check.Passed = passed
			check.Detail = detail
			if !passed && field.Required {
				result.Passed = false
			}
		} else {
			check.Passed = true
		}
		result.Checks = append(result.Checks, check)
	}

	return result
}

// SchemaCheck 单项校验结果
type SchemaCheck struct {
	Field       string
	Description string
	Passed      bool
	Detail      string
}

// SchemaResult schema 校验总结果
type SchemaResult struct {
	Passed bool
	Checks []SchemaCheck
}

func (r *SchemaResult) Summary() string {
	var sb strings.Builder
	if r.Passed {
		sb.WriteString("schema: passed\n")
	} else {
		sb.WriteString("schema: FAILED\n")
	}
	for _, c := range r.Checks {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s", status, c.Field))
		if c.Detail != "" {
			sb.WriteString(": " + c.Detail)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// SchemaGateCollector 实现 Emitter，收集事件用于 Schema 校验
type SchemaGateCollector struct {
	mu            sync.Mutex
	toolCallCount int
}

func NewSchemaGateCollector() *SchemaGateCollector {
	return &SchemaGateCollector{}
}

func (c *SchemaGateCollector) Emit(_ context.Context, event AgentEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if event.Type == EventToolCallEnd {
		c.toolCallCount++
	}
}

func (c *SchemaGateCollector) ToolCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.toolCallCount
}
