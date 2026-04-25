package eval

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

type fakeDispatcher struct {
	task *protocol.TaskEnvelope
}

func (d *fakeDispatcher) Dispatch(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	d.task = task
	return &protocol.TaskResult{
		TaskID:  task.TaskID,
		Agent:   task.Assignee,
		Status:  protocol.ResultStatusSucceeded,
		Summary: "ok",
		Metadata: map[string]any{
			"intent":      "alert_analysis",
			"domains":     []any{"metrics", "logs"},
			"tokens_used": int64(42),
			"llm_calls":   2,
		},
	}, nil
}

func TestRuntimeRunnerBuildsEvalTask(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	runner := NewDispatcherRunner(dispatcher, "custom-supervisor").WithSessionID("session-1").WithResponseMode("chat")

	result, err := runner.Run(context.Background(), "paymentservice CPU 高")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if dispatcher.task == nil || dispatcher.task.Assignee != "custom-supervisor" {
		t.Fatalf("unexpected task: %#v", dispatcher.task)
	}
	if dispatcher.task.Input["response_mode"] != "chat" || dispatcher.task.Input["entrypoint"] != "eval" {
		t.Fatalf("unexpected task input: %#v", dispatcher.task.Input)
	}
	if result.Intent != "alert_analysis" || !containsString(result.Domains, "metrics") || result.TokensUsed != 42 || result.LLMCalls != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}
