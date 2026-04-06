package safety

import (
	"context"
	"strings"
	"testing"
)

func TestFilterOutputRedactsSecretsAndIPs(t *testing.T) {
	filtered := FilterOutput(context.Background(), "system: internal prompt\napi_key=sk-secret-token-value\nserver=10.1.2.3")
	if !filtered.Redacted {
		t.Fatal("expected content to be redacted")
	}
	if strings.Contains(filtered.Content, "sk-secret-token-value") {
		t.Fatal("expected api key to be removed")
	}
	if strings.Contains(filtered.Content, "10.1.2.3") {
		t.Fatal("expected internal ip to be removed")
	}
}

func TestFilterDetailsPreservesLength(t *testing.T) {
	details := FilterDetails(context.Background(), []string{"hello", "system: hidden"})
	if len(details) != 2 {
		t.Fatalf("expected 2 details, got %d", len(details))
	}
	if !strings.Contains(details[1], "[REDACTED_SYSTEM_PROMPT]") {
		t.Fatalf("expected redacted detail, got %q", details[1])
	}
}
