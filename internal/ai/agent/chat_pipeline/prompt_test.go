package chat_pipeline

import (
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
