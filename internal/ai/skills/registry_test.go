package skills

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

type fakeSkill struct {
	name  string
	match bool
	focus string
}

func (f *fakeSkill) Name() string {
	return f.name
}

func (f *fakeSkill) Description() string {
	return "fake skill"
}

func (f *fakeSkill) Focus() string {
	return f.focus
}

func (f *fakeSkill) Match(*protocol.TaskEnvelope) bool {
	return f.match
}

func (f *fakeSkill) Run(context.Context, *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return &protocol.TaskResult{Summary: f.name}, nil
}

func TestRegistryPrefersFirstMatchingSkill(t *testing.T) {
	registry, err := NewRegistry("knowledge",
		[]Skill{
			&fakeSkill{name: "default", match: false},
			&fakeSkill{name: "specific", match: true},
		},
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
		[]Skill{
			&fakeSkill{name: "default", match: false},
			&fakeSkill{name: "secondary", match: false},
		},
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

func TestRegistryFallsBackToConfiguredDefault(t *testing.T) {
	registry, err := NewRegistry("logs",
		[]Skill{
			&fakeSkill{name: "first", match: false},
			&fakeSkill{name: "configured", match: false},
		},
		WithDefault("configured"),
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	task := protocol.NewRootTask("sess", "query", "logs")
	if matched := registry.Match(task); matched != nil {
		t.Fatalf("expected no explicit match, got %q", matched.Name())
	}

	exec, err := registry.Execute(context.Background(), task)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if exec.Skill.Name() != "configured" {
		t.Fatalf("expected configured default, got %q", exec.Skill.Name())
	}
}

func TestRegistryUnregister(t *testing.T) {
	registry, err := NewRegistry("test",
		[]Skill{
			&fakeSkill{name: "skill_a", match: false},
			&fakeSkill{name: "skill_b", match: true},
		},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	removed := registry.Unregister("skill_a")
	if !removed {
		t.Fatal("expected skill_a to be removed")
	}
	if len(registry.SkillNames()) != 1 || registry.SkillNames()[0] != "skill_b" {
		t.Fatalf("expected only skill_b, got %v", registry.SkillNames())
	}
	removed = registry.Unregister("nonexistent")
	if removed {
		t.Fatal("expected false for non-existent skill")
	}
}

func TestFocusCollectorSkipsConfiguredDefaultFallback(t *testing.T) {
	registry, err := NewRegistry("logs",
		[]Skill{
			&fakeSkill{name: "specific", match: false, focus: "specific focus"},
			&fakeSkill{name: "fallback", match: false, focus: "fallback focus"},
		},
		WithDefault("fallback"),
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	hints := NewFocusCollector(registry).Collect("generic query")
	if len(hints) != 0 {
		t.Fatalf("expected no fallback focus hints, got %#v", hints)
	}
}
