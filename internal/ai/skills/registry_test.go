package skills

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

type fakeSkill struct {
	name  string
	match bool
}

func (f *fakeSkill) Name() string {
	return f.name
}

func (f *fakeSkill) Description() string {
	return "fake skill"
}

func (f *fakeSkill) Match(*protocol.TaskEnvelope) bool {
	return f.match
}

func (f *fakeSkill) Run(context.Context, *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return &protocol.TaskResult{Summary: f.name}, nil
}

func TestRegistryPrefersFirstMatchingSkill(t *testing.T) {
	registry, err := NewRegistry("knowledge",
		&fakeSkill{name: "default", match: false},
		&fakeSkill{name: "specific", match: true},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	exec, err := registry.Execute(context.Background(), protocol.NewRootTask("sess", "query", "knowledge"))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if exec.Skill.Name() != "specific" {
		t.Fatalf("expected specific skill, got %q", exec.Skill.Name())
	}
	if exec.Result.Metadata["skill_name"] != "specific" {
		t.Fatalf("expected skill metadata, got %#v", exec.Result.Metadata)
	}
}

func TestRegistryFallsBackToFirstSkill(t *testing.T) {
	registry, err := NewRegistry("logs",
		&fakeSkill{name: "default", match: false},
		&fakeSkill{name: "secondary", match: false},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	exec, err := registry.Execute(context.Background(), protocol.NewRootTask("sess", "query", "logs"))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if exec.Skill.Name() != "default" {
		t.Fatalf("expected fallback skill default, got %q", exec.Skill.Name())
	}
}
