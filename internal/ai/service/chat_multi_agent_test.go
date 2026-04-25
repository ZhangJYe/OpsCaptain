package service

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/agent/supervisor"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

func enableMultiAgentForTest(t *testing.T) {
	t.Helper()
	oldConfigBool := multiAgentConfigBool
	multiAgentConfigBool = func(context.Context, string) (bool, bool) {
		return true, true
	}
	t.Cleanup(func() {
		multiAgentConfigBool = oldConfigBool
	})
}

type captureSupervisorAgent struct {
	lastTask *protocol.TaskEnvelope
}

func (a *captureSupervisorAgent) Name() string           { return supervisor.AgentName }
func (a *captureSupervisorAgent) Capabilities() []string { return []string{"test"} }
func (a *captureSupervisorAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	a.lastTask = task
	return &protocol.TaskResult{TaskID: task.TaskID, Agent: a.Name(), Status: protocol.ResultStatusSucceeded, Summary: "ok", Confidence: 1}, nil
}

func TestShouldUseMultiAgentForChat(t *testing.T) {
	enableMultiAgentForTest(t)
	if !ShouldUseMultiAgentForChat(context.Background(), "analyze current Prometheus alerts") {
		t.Fatal("expected ops query to route to multi-agent")
	}
	if ShouldUseMultiAgentForChat(context.Background(), "hello, introduce yourself") {
		t.Fatal("expected generic chat to stay on legacy route")
	}
}

func TestShouldUseMultiAgentForChatDisabledByConfig(t *testing.T) {
	oldConfigBool := multiAgentConfigBool
	multiAgentConfigBool = func(context.Context, string) (bool, bool) {
		return false, true
	}
	t.Cleanup(func() {
		multiAgentConfigBool = oldConfigBool
	})

	if ShouldUseMultiAgentForChat(context.Background(), "analyze current Prometheus alerts") {
		t.Fatal("expected multi-agent route to stay disabled")
	}
}

func TestRunChatMultiAgentUsesChatMode(t *testing.T) {
	enableMultiAgentForTest(t)
	oldFactory := newPersistentRuntime
	oldRegister := registerChatAgentsFn
	oldMemoryFactory := newMemoryService
	oldRuntimes := chatRuntimes
	oldCfgBool := degradationConfigBool
	oldCfgString := degradationConfigString
	defer func() {
		newPersistentRuntime = oldFactory
		registerChatAgentsFn = oldRegister
		newMemoryService = oldMemoryFactory
		chatRuntimes = oldRuntimes
		degradationConfigBool = oldCfgBool
		degradationConfigString = oldCfgString
	}()

	degradationConfigBool = func(context.Context, string) bool { return false }
	degradationConfigString = func(context.Context, string) string { return "" }
	chatRuntimes = make(map[string]*runtime.Runtime)
	supervisorAgent := &captureSupervisorAgent{}
	memorySvc := &stubAIOpsMemory{
		sessionID:     "chat-session",
		memoryContext: "- [fact] payment service had a recent timeout",
		contextDetail: []string{"context profile=aiops-default"},
		refs:          []protocol.MemoryRef{{ID: "mem-1", Type: "fact"}},
	}

	newPersistentRuntime = func(string) (*runtime.Runtime, error) { return runtime.New(), nil }
	registerChatAgentsFn = func(rt *runtime.Runtime) error { return rt.Register(supervisorAgent) }
	newMemoryService = func() aiOpsMemory { return memorySvc }

	response, err := RunChatMultiAgent(context.Background(), "chat-session", "check payment service log errors")
	if err != nil {
		t.Fatalf("run chat multi-agent: %v", err)
	}
	if response.Content != "ok" {
		t.Fatalf("unexpected result: %q", response.Content)
	}
	if response.TraceID == "" {
		t.Fatal("expected trace id")
	}
	if len(response.Detail) == 0 {
		t.Fatal("expected detail messages")
	}
	if response.Detail[0] != "context profile=aiops-default" {
		t.Fatalf("expected context detail first, got %v", response.Detail)
	}
	if supervisorAgent.lastTask == nil {
		t.Fatal("expected supervisor root task")
	}
	if supervisorAgent.lastTask.Goal != "check payment service log errors" {
		t.Fatalf("expected raw query goal, got %q", supervisorAgent.lastTask.Goal)
	}
	if got, _ := supervisorAgent.lastTask.Input["response_mode"].(string); got != "chat" {
		t.Fatalf("expected chat response mode, got %q", got)
	}
	if got, _ := supervisorAgent.lastTask.Input["entrypoint"].(string); got != "chat" {
		t.Fatalf("expected chat entrypoint, got %q", got)
	}
	if got, _ := supervisorAgent.lastTask.Input["memory_context"].(string); got != memorySvc.memoryContext {
		t.Fatalf("unexpected memory context: %q", got)
	}
}
