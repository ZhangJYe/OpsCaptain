package service

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

func TestShouldUseMultiAgentForChat(t *testing.T) {
	if !ShouldUseMultiAgentForChat(context.Background(), "请分析当前 Prometheus 告警") {
		t.Fatal("expected ops query to route to multi-agent")
	}
	if ShouldUseMultiAgentForChat(context.Background(), "你好，介绍一下你自己") {
		t.Fatal("expected generic chat to stay on legacy route")
	}
}

func TestRunChatMultiAgentUsesChatMode(t *testing.T) {
	oldFactory := newPersistentRuntime
	oldRegister := registerAIOpsAgentsFn
	oldMemoryFactory := newMemoryService
	oldRuntimes := aiOpsRuntimes
	defer func() {
		newPersistentRuntime = oldFactory
		registerAIOpsAgentsFn = oldRegister
		newMemoryService = oldMemoryFactory
		aiOpsRuntimes = oldRuntimes
	}()

	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	supervisorAgent := &captureSupervisorAgent{}
	memorySvc := &stubAIOpsMemory{
		sessionID:     "chat-session",
		memoryContext: "- [fact] 支付服务最近有一次超时历史",
		contextDetail: []string{"context profile=aiops-default"},
		refs: []protocol.MemoryRef{
			{ID: "mem-1", Type: "fact"},
		},
	}

	newPersistentRuntime = func(string) (*runtime.Runtime, error) {
		return runtime.New(), nil
	}
	registerAIOpsAgentsFn = func(rt *runtime.Runtime) error {
		return rt.Register(supervisorAgent)
	}
	newMemoryService = func() aiOpsMemory {
		return memorySvc
	}

	result, detail, traceID, err := RunChatMultiAgent(context.Background(), "chat-session", "请排查支付服务日志错误")
	if err != nil {
		t.Fatalf("run chat multi-agent: %v", err)
	}
	if result != "ok" {
		t.Fatalf("unexpected result: %q", result)
	}
	if traceID == "" {
		t.Fatal("expected trace id")
	}
	if len(detail) == 0 {
		t.Fatal("expected detail messages")
	}
	if detail[0] != "context profile=aiops-default" {
		t.Fatalf("expected context detail first, got %v", detail)
	}
	if supervisorAgent.lastTask == nil {
		t.Fatal("expected supervisor root task")
	}
	if supervisorAgent.lastTask.Goal != "请排查支付服务日志错误" {
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
