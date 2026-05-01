package skills

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/protocol"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeDisclosureSkill struct {
	name        string
	description string
	match       bool
}

func (s *fakeDisclosureSkill) Name() string {
	return s.name
}

func (s *fakeDisclosureSkill) Description() string {
	return s.description
}

func (s *fakeDisclosureSkill) Match(*protocol.TaskEnvelope) bool {
	return s.match
}

func (s *fakeDisclosureSkill) Run(context.Context, *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return nil, nil
}

type fakeDisclosureTool struct {
	name string
}

func (t *fakeDisclosureTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name}, nil
}

func TestProgressiveDisclosureHonorsExplicitSkillSelection(t *testing.T) {
	logsRegistry, err := NewRegistry("logs",
		&fakeDisclosureSkill{name: "logs_evidence_extract", description: "Inspect log evidence", match: false},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	knowledgeRegistry, err := NewRegistry("knowledge",
		&fakeDisclosureSkill{name: "knowledge_sop_lookup", description: "Inspect SOPs", match: false},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	pd := NewProgressiveDisclosure(
		[]*Registry{logsRegistry, knowledgeRegistry},
		[]TieredTool{
			{Tool: &fakeDisclosureTool{name: "always_on"}, Tier: TierAlwaysOn},
			{Tool: &fakeDisclosureTool{name: "logs_tool"}, Tier: TierSkillGate, Domains: []string{"logs"}},
			{Tool: &fakeDisclosureTool{name: "knowledge_tool"}, Tier: TierSkillGate, Domains: []string{"knowledge"}},
		},
	)

	result := pd.Disclose("先帮我看一下", []string{"logs_evidence_extract"})

	if len(result.SelectedSkills) != 1 || result.SelectedSkills[0].Name != "logs_evidence_extract" {
		t.Fatalf("expected selected logs skill to be resolved, got %#v", result.SelectedSkills)
	}
	if !containsToolNamed(result.Tools, "always_on") || !containsToolNamed(result.Tools, "logs_tool") {
		t.Fatalf("expected always-on and logs tools to be disclosed, got %#v", result.Tools)
	}
	if containsToolNamed(result.Tools, "knowledge_tool") {
		t.Fatalf("expected knowledge tool to stay hidden when only logs skill was selected")
	}
}

func TestProgressiveDisclosureIgnoresUnknownSkillSelection(t *testing.T) {
	logsRegistry, err := NewRegistry("logs",
		&fakeDisclosureSkill{name: "logs_evidence_extract", description: "Inspect log evidence", match: false},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	pd := NewProgressiveDisclosure(
		[]*Registry{logsRegistry},
		[]TieredTool{
			{Tool: &fakeDisclosureTool{name: "always_on"}, Tier: TierAlwaysOn},
			{Tool: &fakeDisclosureTool{name: "logs_tool"}, Tier: TierSkillGate, Domains: []string{"logs"}},
		},
	)

	result := pd.Disclose("先帮我看一下", []string{"not_exists"})
	if len(result.SelectedSkills) != 0 {
		t.Fatalf("expected unknown skills to be ignored, got %#v", result.SelectedSkills)
	}
	if containsToolNamed(result.Tools, "logs_tool") {
		t.Fatalf("expected unknown skills to avoid unlocking gated tools")
	}
}

func containsToolNamed(tools []tool.BaseTool, target string) bool {
	for _, current := range tools {
		info, err := current.Info(context.Background())
		if err != nil {
			continue
		}
		if info != nil && info.Name == target {
			return true
		}
	}
	return false
}
