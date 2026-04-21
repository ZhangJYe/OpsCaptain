package contracts

import (
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

func TestCoreContractsAreGlobalAndRenderable(t *testing.T) {
	for _, agent := range []string{"triage", "metrics", "logs", "knowledge", "reporter"} {
		contract, ok := Get(agent)
		if !ok {
			t.Fatalf("expected contract for %s", agent)
		}
		if contract.CacheScope != CacheScopeGlobal {
			t.Fatalf("expected global cache scope for %s, got %q", agent, contract.CacheScope)
		}
		prompt, ok := PromptFor(agent)
		if !ok {
			t.Fatalf("expected renderable prompt for %s", agent)
		}
		if !strings.Contains(prompt, "<!-- scope: global -->") {
			t.Fatalf("expected global scope marker for %s, got %q", agent, prompt)
		}
		if !strings.Contains(prompt, "## Must Not") {
			t.Fatalf("expected guardrails in prompt for %s, got %q", agent, prompt)
		}
	}
}

func TestReporterContractForbidsNewFacts(t *testing.T) {
	prompt, ok := PromptFor("reporter")
	if !ok {
		t.Fatal("expected reporter contract")
	}
	if !strings.Contains(prompt, "不要新增 specialist 没有提供的新事实") {
		t.Fatalf("expected reporter contract to forbid new facts, got %q", prompt)
	}
}

func TestAttachMetadataAddsContractIdentity(t *testing.T) {
	result := AttachMetadata(&protocol.TaskResult{Agent: "knowledge"}, "knowledge")
	if result.Metadata["agent_contract_id"] != "knowledge:"+Version {
		t.Fatalf("unexpected contract id metadata: %#v", result.Metadata)
	}
	if result.Metadata["agent_contract_scope"] != CacheScopeGlobal {
		t.Fatalf("unexpected contract scope metadata: %#v", result.Metadata)
	}
}
