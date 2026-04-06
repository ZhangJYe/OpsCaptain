package service

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

func TestShouldUseMultiAgentForChat(t *testing.T) {
	if !ShouldUseMultiAgentForChat(context.Background(), "analyze current Prometheus alerts") {
		t.Fatal("expected ops query to route to multi-agent")
	}
	if ShouldUseMultiAgentForChat(context.Background(), "hello, introduce yourself") {
		t.Fatal("expected generic chat to stay on legacy route")
	}
}

func TestRunChatMultiAgentUsesChatMode(t *testing.T) {
	oldFactory := newPersistentRuntime
	oldRegister := registerAIOpsAgentsFn
	oldMemoryFactory := newMemoryService
	oldRuntimes := aiOpsRuntimes
	oldCfgBool := degradationConfigBool
	oldCfgString := degradationConfigString
	defer func() {
		newPersistentRuntime = oldFactory
		registerAIOpsAgentsFn = oldRegister
		newMemoryService = oldMemoryFactory
		aiOpsRuntimes = oldRuntimes
		degradationConfigBool = oldCfgBool
		degradationConfigString = oldCfgString
	}()

	degradationConfigBool = func(context.Context, string) bool { return false }
	degradationConfigString = func(context.Context, string) string { return "" }
	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	supervisorAgent := &captureSupervisorAgent{}
	memorySvc := &stubAIOpsMemory{
		sessionID:     "chat-session",
		memoryContext: "- [fact] payment service had a recent timeout",
		contextDetail: []string{"context profile=aiops-default"},
		refs:          []protocol.MemoryRef{{ID: "mem-1", Type: "fact"}},
	}

	newPersistentRuntime = func(string) (*runtime.Runtime, error) { return runtime.New(), nil }
	registerAIOpsAgentsFn = func(rt *runtime.Runtime) error { return rt.Register(supervisorAgent) }
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
