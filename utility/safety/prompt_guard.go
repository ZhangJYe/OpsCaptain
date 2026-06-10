package safety

import (
	"context"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

type PromptGuardDecision struct {
	Allowed   bool
	Reason    string
	Pattern   string
	RiskScore float64 // 0.0 = safe, 1.0 = definitely injection
	RiskLevel string  // "safe" | "suspicious" | "dangerous"
}

type promptPattern struct {
	name  string
	regex *regexp.Regexp
}

var promptPatterns = []promptPattern{
	{name: "ignore_previous_instructions", regex: regexp.MustCompile(`(?i)\bignore\s+(all\s+)?previous\s+instructions?\b`)},
	{name: "forget_instructions", regex: regexp.MustCompile(`(?i)\bforget\s+(all\s+)?(your\s+)?(previous\s+)?instructions?\b`)},
	{name: "new_instructions", regex: regexp.MustCompile(`(?i)\bnew\s+instructions?\s*:`)},
	{name: "you_are_now", regex: regexp.MustCompile(`(?i)\byou\s+are\s+now\b`)},
	{name: "system_prefix", regex: regexp.MustCompile(`(?i)\bsystem\s*:`)},
	{name: "inst_block", regex: regexp.MustCompile(`(?i)\[inst\]|<<\s*sys\s*>>`)},
	{name: "markdown_system", regex: regexp.MustCompile("(?i)```\\s*system")},
	{name: "dan_jailbreak", regex: regexp.MustCompile(`(?i)\bdan\s+mode\b|\bdo\s+anything\s+now\b`)},
	{name: "chinese_ignore", regex: regexp.MustCompile(`忽略(之前|以上|前面)的?指令`)},
	{name: "chinese_role_override", regex: regexp.MustCompile(`你现在是`)},
	{name: "chinese_new_instructions", regex: regexp.MustCompile(`新的指令[：:]`)},
}

func CheckPrompt(ctx context.Context, input string) PromptGuardDecision {
	if !promptGuardEnabled(ctx) {
		return PromptGuardDecision{Allowed: true, RiskLevel: "safe"}
	}

	decision := evaluatePrompt(input)

	// Regex matched — definite injection
	if !decision.Allowed {
		decision.RiskScore = 1.0
		decision.RiskLevel = "dangerous"
		return decision
	}

	// Regex passed — run LLM classifier if enabled
	if ClassifierEnabled(ctx) {
		verdict := ClassifyInjection(ctx, input)
		decision.RiskScore = verdict.Score
		decision.Reason = verdict.Reason
		threshold := ClassifierThreshold(ctx)
		if verdict.Score >= threshold {
			decision.RiskLevel = "suspicious"
		} else {
			decision.RiskLevel = "safe"
		}
	} else {
		decision.RiskLevel = "safe"
	}

	return decision
}

func evaluatePrompt(input string) PromptGuardDecision {
	normalized := normalizePrompt(input)
	if normalized == "" {
		return PromptGuardDecision{Allowed: true}
	}
	for _, pattern := range promptPatterns {
		if pattern.regex.MatchString(normalized) {
			return PromptGuardDecision{
				Allowed: false,
				Reason:  "request blocked by prompt guard",
				Pattern: pattern.name,
			}
		}
	}
	return PromptGuardDecision{Allowed: true}
}

func normalizePrompt(input string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(input)), " "))
}

func promptGuardEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "safety.prompt_guard.enabled")
	if err != nil || v.String() == "" {
		return true
	}
	return v.Bool()
}

// SanitizeForLLMContext 对将被拼入 LLM prompt 的外部文本做注入清洗。
// 匹配到注入模式的片段会被替换为 [已过滤]。
// 用于变更事件、外部 webhook 内容等非直接用户输入但会被注入到 LLM 上下文的场景。
func SanitizeForLLMContext(input string) string {
	if input == "" {
		return input
	}
	result := input
	for _, pattern := range promptPatterns {
		result = pattern.regex.ReplaceAllString(result, "[已过滤]")
	}
	return result
}
