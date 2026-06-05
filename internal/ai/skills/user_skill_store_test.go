package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileUserSkillStore_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewFileUserSkillStore(filepath.Join(dir, "nonexistent", "registry.json"))

	data, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(data.Tools) != 0 || len(data.Skills) != 0 {
		t.Fatalf("expected empty data, got tools=%d skills=%d", len(data.Tools), len(data.Skills))
	}
}

func TestFileUserSkillStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	store := NewFileUserSkillStore(path)

	now := time.Now().Truncate(time.Second)
	input := &UserRegistryData{
		Tools: []UserMCPTool{
			{
				ID:          "tool-1",
				Name:        "test-tool",
				Description: "A test tool",
				Transport:   TransportSSE,
				EndpointURL: "http://localhost:8080/sse",
				ToolName:    "my_tool",
				InputSchema: map[string]any{"type": "object"},
				TimeoutMs:   5000,
				Status:      StatusPending,
				CreatedAt:   now,
				CreatedBy:   "user-1",
			},
		},
		Skills: []UserSkill{
			{
				ID:           "skill-1",
				Name:         "test-skill",
				Description:  "A test skill",
				Domain:       DomainMetrics,
				ToolRefID:    "tool-1",
				Focus:        "latency",
				OutputParser: ParserJSONArray,
				Keywords:     []string{"latency", "p99"},
				Tier:         2,
				Status:       StatusApproved,
				CreatedAt:    now,
				CreatedBy:    "user-1",
				ApprovedAt:   &now,
				ApprovedBy:   "admin",
			},
		},
	}

	if err := store.Save(context.Background(), input); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(loaded.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(loaded.Tools))
	}
	if len(loaded.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(loaded.Skills))
	}

	tool := loaded.Tools[0]
	if tool.ID != "tool-1" || tool.Name != "test-tool" || tool.Transport != TransportSSE {
		t.Errorf("tool mismatch: %+v", tool)
	}

	skill := loaded.Skills[0]
	if skill.ID != "skill-1" || skill.Domain != DomainMetrics || skill.Tier != 2 {
		t.Errorf("skill mismatch: %+v", skill)
	}
	if len(skill.Keywords) != 2 || skill.Keywords[0] != "latency" {
		t.Errorf("keywords mismatch: %v", skill.Keywords)
	}
	if skill.ApprovedAt == nil || skill.ApprovedBy != "admin" {
		t.Errorf("approval fields mismatch: approved_at=%v approved_by=%s", skill.ApprovedAt, skill.ApprovedBy)
	}
}

func TestFileUserSkillStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	store := NewFileUserSkillStore(path)

	data := &UserRegistryData{
		Tools:  []UserMCPTool{{ID: "t1", Name: "t1", Status: StatusPending}},
		Skills: []UserSkill{},
	}

	if err := store.Save(context.Background(), data); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	// Verify the main file exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("registry file not found: %v", err)
	}

	// Verify no .tmp file is left behind.
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf(".tmp file should not exist after save, got err=%v", err)
	}
}

func TestFileUserSkillStore_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	store := NewFileUserSkillStore(filepath.Join(dir, "registry.json"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Load(ctx); err == nil {
		t.Fatal("expected error on cancelled context for Load")
	}
	if err := store.Save(ctx, &UserRegistryData{}); err == nil {
		t.Fatal("expected error on cancelled context for Save")
	}
}
