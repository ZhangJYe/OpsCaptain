package chat

import (
	"context"
	"testing"

	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/utility/mem"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type fakeChatRunnable struct {
	answer string
}

func (f *fakeChatRunnable) Invoke(context.Context, *chat_pipeline.UserMessage, ...compose.Option) (*schema.Message, error) {
	return &schema.Message{Content: f.answer}, nil
}

func (f *fakeChatRunnable) Stream(context.Context, *chat_pipeline.UserMessage, ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (f *fakeChatRunnable) Collect(context.Context, *schema.StreamReader[*chat_pipeline.UserMessage], ...compose.Option) (*schema.Message, error) {
	return &schema.Message{Content: f.answer}, nil
}

func (f *fakeChatRunnable) Transform(context.Context, *schema.StreamReader[*chat_pipeline.UserMessage], ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func TestChatUsesMultiAgentRoute(t *testing.T) {
	oldShouldUse := shouldUseMultiAgentForChat
	oldRun := runChatMultiAgent
	oldBuild := buildChatAgent
	defer func() {
		shouldUseMultiAgentForChat = oldShouldUse
		runChatMultiAgent = oldRun
		buildChatAgent = oldBuild
	}()

	shouldUseMultiAgentForChat = func(context.Context, string) bool { return true }
	buildChatAgent = func(context.Context) (compose.Runnable[*chat_pipeline.UserMessage, *schema.Message], error) {
		t.Fatal("legacy chat agent should not be built for multi-agent route")
		return nil, nil
	}

	capturedSessionID := ""
	capturedQuery := ""
	runChatMultiAgent = func(ctx context.Context, sessionID, query string) (string, []string, string, error) {
		capturedSessionID = sessionID
		capturedQuery = query
		return "multi-agent answer", []string{"detail"}, "trace-chat-1", nil
	}

	ctrl := &ControllerV1{}
	sessionID := mem.GenerateSessionID()
	res, err := ctrl.Chat(context.Background(), &v1.ChatReq{
		Id:       sessionID,
		Question: "请分析当前 Prometheus 告警",
	})
	if err != nil {
		t.Fatalf("chat returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected response")
	}
	if res.Answer != "multi-agent answer" {
		t.Fatalf("unexpected answer: %q", res.Answer)
	}
	if res.Mode != "multi_agent" {
		t.Fatalf("expected multi_agent mode, got %q", res.Mode)
	}
	if res.TraceID != "trace-chat-1" {
		t.Fatalf("unexpected trace id: %q", res.TraceID)
	}
	if capturedSessionID != sessionID || capturedQuery != "请分析当前 Prometheus 告警" {
		t.Fatalf("unexpected multi-agent inputs: session=%q query=%q", capturedSessionID, capturedQuery)
	}
}

func TestChatFallsBackToLegacyRoute(t *testing.T) {
	oldShouldUse := shouldUseMultiAgentForChat
	oldRun := runChatMultiAgent
	oldBuild := buildChatAgent
	defer func() {
		shouldUseMultiAgentForChat = oldShouldUse
		runChatMultiAgent = oldRun
		buildChatAgent = oldBuild
	}()

	shouldUseMultiAgentForChat = func(context.Context, string) bool { return false }
	runChatMultiAgent = func(context.Context, string, string) (string, []string, string, error) {
		t.Fatal("multi-agent route should not be used for generic chat")
		return "", nil, "", nil
	}
	buildChatAgent = func(context.Context) (compose.Runnable[*chat_pipeline.UserMessage, *schema.Message], error) {
		return &fakeChatRunnable{answer: "legacy answer"}, nil
	}

	ctrl := &ControllerV1{}
	res, err := ctrl.Chat(context.Background(), &v1.ChatReq{
		Id:       mem.GenerateSessionID(),
		Question: "你好，介绍一下你自己",
	})
	if err != nil {
		t.Fatalf("chat returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected response")
	}
	if res.Answer != "legacy answer" {
		t.Fatalf("unexpected answer: %q", res.Answer)
	}
	if res.Mode != "legacy" {
		t.Fatalf("expected legacy mode, got %q", res.Mode)
	}
	if len(res.Detail) == 0 {
		t.Fatal("expected legacy route to include context detail")
	}
}
