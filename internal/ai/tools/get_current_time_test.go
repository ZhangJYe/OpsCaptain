package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGetCurrentTimeTool_Info(t *testing.T) {
	tool := NewGetCurrentTimeTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("failed to get tool info: %v", err)
	}
	if info.Name != "get_current_time" {
		t.Fatalf("expected tool name 'get_current_time', got '%s'", info.Name)
	}
}

func TestGetCurrentTimeTool_Invoke(t *testing.T) {
	tool := NewGetCurrentTimeTool()
	result, err := tool.InvokableRun(context.Background(), "{}")
	if err != nil {
		t.Fatalf("failed to invoke tool: %v", err)
	}

	var output GetCurrentTimeOutput
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if !output.Success {
		t.Fatal("expected success to be true")
	}
	if output.Seconds <= 0 {
		t.Fatal("expected positive seconds")
	}
	if output.Milliseconds <= 0 {
		t.Fatal("expected positive milliseconds")
	}
	if output.Microseconds <= 0 {
		t.Fatal("expected positive microseconds")
	}
	if output.Timestamp == "" {
		t.Fatal("expected non-empty timestamp")
	}
	if output.Message == "" {
		t.Fatal("expected non-empty message")
	}
}
