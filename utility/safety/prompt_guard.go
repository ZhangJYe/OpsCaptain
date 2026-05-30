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
	{name: "you_are_now", regex: regexp.MustCompile(`(?i)\byou\s+are\s+now\b`)},
	{name: "system_prefix", regex: regexp.MustCompile(`(?i)\bsystem\s*:`)},
	{name: "inst_block", regex: regexp.MustCompile(`(?i)\[inst\]|<<\s*sys\s*>>`)},
	{name: "chinese_ignore", regex: regexp.MustCompile(`忽略(之前|以上|前面)的?指令`)},
	{name: "chinese_role_override", regex: regexp.MustCompile(`你现在是`)},
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
