package chat_pipeline

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizePromptConfigValueDropsUnresolvedEnvPlaceholders(t *testing.T) {
	if got, ok := normalizePromptConfigValue("${LOG_TOPIC_REGION}"); ok || got != "" {
		t.Fatalf("expected unresolved env placeholder to be dropped, got value=%q ok=%t", got, ok)
	}
}

func TestNormalizePromptConfigValueResolvesConcreteValue(t *testing.T) {
	if got, ok := normalizePromptConfigValue("ap-shanghai"); !ok || got != "ap-shanghai" {
		t.Fatalf("expected concrete value to pass through, got value=%q ok=%t", got, ok)
	}
}

func TestBuildSystemPromptIncludesDefaultChineseRule(t *testing.T) {
	prompt := buildSystemPrompt(context.Background())
	if !strings.Contains(prompt, "默认使用中文回答") {
		t.Fatalf("expected prompt to require Chinese by default, got %q", prompt)
	}
	if !strings.Contains(prompt, "用户明确要求英文或其他语言") {
		t.Fatalf("expected prompt to allow explicit language override, got %q", prompt)
	}
}
