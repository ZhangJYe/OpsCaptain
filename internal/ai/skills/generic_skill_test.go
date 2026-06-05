package skills

import (
	"context"
	"fmt"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

// mockInvoker returns a preset output or error for testing.
type mockInvoker struct {
	output string
	err    error
}

func (m *mockInvoker) Invoke(_ context.Context, _, _ string) (string, error) {
	return m.output, m.err
}

func TestGenericSkill_Match(t *testing.T) {
	skill := NewGenericSkill(UserSkill{
		Name:     "test-skill",
		Keywords: []string{"cpu", "memory", "disk"},
	}, &mockInvoker{})

	tests := []struct {
		name string
		goal string
		want bool
	}{
		{"match cpu", "check cpu usage", true},
		{"match memory", "memory is high", true},
		{"match disk", "disk space low", true},
		{"case insensitive", "Check CPU Usage", true},
		{"no match", "deploy application", false},
		{"empty goal", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &protocol.TaskEnvelope{Goal: tt.goal}
			if got := skill.Match(task); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.goal, got, tt.want)
			}
		})
	}

	// Nil task
	if skill.Match(nil) {
		t.Error("Match(nil) should return false")
	}

	// Empty keywords
	emptySkill := NewGenericSkill(UserSkill{Name: "empty"}, &mockInvoker{})
	if emptySkill.Match(&protocol.TaskEnvelope{Goal: "test"}) {
		t.Error("Match with empty keywords should return false")
	}
}

func TestGenericSkill_Run_JSONArray(t *testing.T) {
	output := `[{"title":"High CPU","content":"CPU usage at 95%"},{"title":"High Memory","content":"Memory usage at 88%"}]`
	skill := NewGenericSkill(UserSkill{
		Name:         "metrics-skill",
		ToolRefID:    "tool-1",
		OutputParser: ParserJSONArray,
	}, &mockInvoker{output: output})

	task := &protocol.TaskEnvelope{TaskID: "t1", Goal: "check cpu"}
	result, err := skill.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Status != protocol.ResultStatusSucceeded {
		t.Errorf("Status = %v, want %v", result.Status, protocol.ResultStatusSucceeded)
	}
	if result.Confidence != 0.70 {
		t.Errorf("Confidence = %v, want 0.70", result.Confidence)
	}
	if len(result.Evidence) != 2 {
		t.Fatalf("Evidence len = %d, want 2", len(result.Evidence))
	}
	if result.Evidence[0].Title != "High CPU" {
		t.Errorf("Evidence[0].Title = %q, want %q", result.Evidence[0].Title, "High CPU")
	}
	if result.Evidence[0].Snippet != "CPU usage at 95%" {
		t.Errorf("Evidence[0].Snippet = %q, want %q", result.Evidence[0].Snippet, "CPU usage at 95%")
	}
}

func TestGenericSkill_Run_JSONNested(t *testing.T) {
	output := `{"data":{"results":[{"title":"Alert","content":"Disk full"}]}}`
	skill := NewGenericSkill(UserSkill{
		Name:         "nested-skill",
		ToolRefID:    "tool-2",
		OutputParser: ParserJSONNested,
		JSONPath:     "data.results",
	}, &mockInvoker{output: output})

	task := &protocol.TaskEnvelope{TaskID: "t2", Goal: "check disk"}
	result, err := skill.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Status != protocol.ResultStatusSucceeded {
		t.Errorf("Status = %v, want %v", result.Status, protocol.ResultStatusSucceeded)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("Evidence len = %d, want 1", len(result.Evidence))
	}
	if result.Evidence[0].Title != "Alert" {
		t.Errorf("Evidence[0].Title = %q, want %q", result.Evidence[0].Title, "Alert")
	}
}

func TestGenericSkill_Run_LogLines(t *testing.T) {
	output := "2024-01-01 ERROR disk full\n2024-01-01 WARN cpu high\n"
	skill := NewGenericSkill(UserSkill{
		Name:         "log-skill",
		ToolRefID:    "tool-3",
		OutputParser: ParserLogLines,
	}, &mockInvoker{output: output})

	task := &protocol.TaskEnvelope{TaskID: "t3", Goal: "check logs"}
	result, err := skill.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Status != protocol.ResultStatusSucceeded {
		t.Errorf("Status = %v, want %v", result.Status, protocol.ResultStatusSucceeded)
	}
	if len(result.Evidence) != 2 {
		t.Fatalf("Evidence len = %d, want 2", len(result.Evidence))
	}
	if result.Evidence[0].SourceType != "log_line" {
		t.Errorf("Evidence[0].SourceType = %q, want %q", result.Evidence[0].SourceType, "log_line")
	}
}

func TestGenericSkill_Run_EmptyOutput(t *testing.T) {
	skill := NewGenericSkill(UserSkill{
		Name:         "empty-skill",
		ToolRefID:    "tool-4",
		OutputParser: ParserJSONArray,
	}, &mockInvoker{output: ""})

	task := &protocol.TaskEnvelope{TaskID: "t4", Goal: "test"}
	result, err := skill.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Status != protocol.ResultStatusSucceeded {
		t.Errorf("Status = %v, want %v", result.Status, protocol.ResultStatusSucceeded)
	}
	if result.Confidence != 0.40 {
		t.Errorf("Confidence = %v, want 0.40", result.Confidence)
	}
	if len(result.Evidence) != 0 {
		t.Errorf("Evidence len = %d, want 0", len(result.Evidence))
	}
}

func TestGenericSkill_Run_InvokeError(t *testing.T) {
	skill := NewGenericSkill(UserSkill{
		Name:      "error-skill",
		ToolRefID: "tool-5",
	}, &mockInvoker{err: fmt.Errorf("connection refused")})

	task := &protocol.TaskEnvelope{TaskID: "t5", Goal: "test"}
	result, err := skill.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run() should not return error, got: %v", err)
	}

	if result.Status != protocol.ResultStatusDegraded {
		t.Errorf("Status = %v, want %v", result.Status, protocol.ResultStatusDegraded)
	}
	if result.Confidence != 0.25 {
		t.Errorf("Confidence = %v, want 0.25", result.Confidence)
	}
	if result.DegradationReason != "connection refused" {
		t.Errorf("DegradationReason = %q, want %q", result.DegradationReason, "connection refused")
	}
}

func TestGenericSkill_Focus(t *testing.T) {
	skill := NewGenericSkill(UserSkill{
		Name:   "focus-skill",
		Focus:  "Check system metrics and alerts",
	}, &mockInvoker{})

	if skill.Focus() != "Check system metrics and alerts" {
		t.Errorf("Focus() = %q, want %q", skill.Focus(), "Check system metrics and alerts")
	}
}

func TestGenericSkill_FocusProviderInterface(t *testing.T) {
	var _ FocusProvider = (*GenericSkill)(nil)
	var _ Skill = (*GenericSkill)(nil)
}
