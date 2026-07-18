package app

import (
	"context"
	"errors"
	"testing"

	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/ai/protocol"
	aiservice "SuperBizAgent/internal/ai/service"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func newEnabledAgentApp(chatApp *ChatApp, aiopsApp *AIOpsApp) *AgentApp {
	agentApp := NewAgentApp(chatApp, aiopsApp)
	agentApp.SetEnabledCheck(func(context.Context) bool { return true })
	return agentApp
}

func TestAgentAppExplicitModesUseFixedPolicies(t *testing.T) {
	chatApp := newTestChatApp(
		func(context.Context, string) (compose.Runnable[*chat_pipeline.UserMessage, *schema.Message], error) {
			return &fakeRunnable{answer: "chat answer"}, nil
		},
		func(context.Context, string) aiservice.DegradationDecision { return aiservice.DegradationDecision{} },
	)
	aiopsApp := NewAIOpsApp()
	aiopsApp.SetDegradationCheck(func(context.Context, string) aiservice.DegradationDecision { return aiservice.DegradationDecision{} })
	aiopsApp.SetRunMultiAgent(func(context.Context, string) (aiservice.ExecutionResponse, error) {
		return aiservice.ExecutionResponse{Content: "diagnosis", TraceID: "trace-aiops"}, nil
	})
	agentApp := newEnabledAgentApp(chatApp, aiopsApp)
	sessionID := memory.GenerateSessionID()

	chatResult, err := agentApp.HandleAgent(context.Background(), &AgentInput{SessionID: sessionID, Query: "解释 Redis", Mode: AgentModeChat})
	if err != nil {
		t.Fatalf("chat HandleAgent() error = %v", err)
	}
	if chatResult.Mode != AgentModeChat || chatResult.Chat == nil || chatResult.Diagnosis != nil {
		t.Fatalf("unexpected chat result: %#v", chatResult)
	}

	diagnosisResult, err := agentApp.HandleAgent(context.Background(), &AgentInput{SessionID: sessionID, Query: "诊断 Redis", Mode: AgentModeAIOpsDiagnosis})
	if err != nil {
		t.Fatalf("aiops HandleAgent() error = %v", err)
	}
	if diagnosisResult.Mode != AgentModeAIOpsDiagnosis || diagnosisResult.Diagnosis == nil || diagnosisResult.Chat != nil {
		t.Fatalf("unexpected diagnosis result: %#v", diagnosisResult)
	}
}

func TestAgentAppAutoReturnsChatWhenDiagnosisToolIsNotCalled(t *testing.T) {
	chatApp := newTestChatApp(nil, nil)
	aiopsApp := NewAIOpsApp()
	agentApp := newEnabledAgentApp(chatApp, aiopsApp)
	agentApp.SetRunAutoChat(func(_ context.Context, input *ChatInput, _ tool.BaseTool) (*ChatResult, error) {
		if !input.DisableCache {
			t.Fatal("auto mode must bypass live-response cache")
		}
		return &ChatResult{Answer: "Redis 是一个内存数据存储", Mode: "chat"}, nil
	})

	result, err := agentApp.HandleAgent(context.Background(), &AgentInput{
		SessionID: memory.GenerateSessionID(),
		Query:     "Redis 是什么？",
		Mode:      AgentModeAuto,
	})
	if err != nil {
		t.Fatalf("HandleAgent() error = %v", err)
	}
	if result.Mode != AgentModeChat || result.Chat == nil || result.Diagnosis != nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentAppAutoDiagnosisUsesOriginalQueryAndServerEngine(t *testing.T) {
	chatApp := newTestChatApp(nil, nil)
	aiopsApp := NewAIOpsApp()
	aiopsApp.SetDegradationCheck(func(context.Context, string) aiservice.DegradationDecision { return aiservice.DegradationDecision{} })
	var gotQuery string
	aiopsApp.SetRunMultiAgent(func(_ context.Context, query string) (aiservice.ExecutionResponse, error) {
		gotQuery = query
		return aiservice.ExecutionResponse{
			Content:    "CPU 根因诊断",
			TraceID:    "trace-auto",
			Engine:     "aiops_plan_execute_replan",
			Confidence: 0.9,
			Evidence: []protocol.EvidenceItem{{
				SourceType: "metrics",
				SourceID:   "metric-1",
				Title:      "CPU",
			}},
		}, nil
	})
	agentApp := newEnabledAgentApp(chatApp, aiopsApp)
	agentApp.SetRunAutoChat(func(ctx context.Context, _ *ChatInput, diagnosisTool tool.BaseTool) (*ChatResult, error) {
		invokable, ok := diagnosisTool.(tool.InvokableTool)
		if !ok {
			t.Fatal("diagnosis tool must be invokable")
		}
		output, err := invokable.InvokableRun(ctx, `{}`)
		if err != nil {
			return nil, err
		}
		return &ChatResult{Answer: output, Mode: "chat"}, nil
	})

	query := "检查生产 order-service 最近十分钟 CPU 持续升高的原因"
	result, err := agentApp.HandleAgent(context.Background(), &AgentInput{
		SessionID: memory.GenerateSessionID(),
		Query:     query,
		Mode:      AgentModeAuto,
	})
	if err != nil {
		t.Fatalf("HandleAgent() error = %v", err)
	}
	if gotQuery != query {
		t.Fatalf("diagnosis must use original query, got %q", gotQuery)
	}
	if result.Mode != AgentModeAIOpsDiagnosis || result.Diagnosis == nil || result.Chat != nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Diagnosis.TraceID != "trace-auto" || len(result.Diagnosis.Evidence) != 1 {
		t.Fatalf("diagnosis payload was not preserved: %#v", result.Diagnosis)
	}
}

func TestAgentAppDisabledReturnsVisibleDegradation(t *testing.T) {
	agentApp := NewAgentApp(NewChatApp(), NewAIOpsApp())
	agentApp.SetEnabledCheck(func(context.Context) bool { return false })
	result, err := agentApp.HandleAgent(context.Background(), &AgentInput{
		SessionID: memory.GenerateSessionID(),
		Query:     "hello",
	})
	if err != nil {
		t.Fatalf("HandleAgent() error = %v", err)
	}
	if !result.Degraded || result.HTTPStatus != 503 || result.DegradationReason != "agent_gateway_disabled" {
		t.Fatalf("unexpected disabled result: %#v", result)
	}
}

func TestAgentAppAutoDiagnosisFailureReturnsDegradedPayload(t *testing.T) {
	chatApp := newTestChatApp(nil, nil)
	aiopsApp := NewAIOpsApp()
	aiopsApp.SetDegradationCheck(func(context.Context, string) aiservice.DegradationDecision { return aiservice.DegradationDecision{} })
	aiopsApp.SetRunMultiAgent(func(context.Context, string) (aiservice.ExecutionResponse, error) {
		return aiservice.ExecutionResponse{}, errors.New("backend unavailable")
	})
	agentApp := newEnabledAgentApp(chatApp, aiopsApp)
	agentApp.SetRunAutoChat(func(ctx context.Context, _ *ChatInput, diagnosisTool tool.BaseTool) (*ChatResult, error) {
		output, err := diagnosisTool.(tool.InvokableTool).InvokableRun(ctx, `{}`)
		return &ChatResult{Answer: output, Mode: "chat"}, err
	})

	result, err := agentApp.HandleAgent(context.Background(), &AgentInput{
		SessionID: memory.GenerateSessionID(),
		Query:     "诊断生产服务",
		Mode:      AgentModeAuto,
	})
	if err != nil {
		t.Fatalf("HandleAgent() error = %v", err)
	}
	if result.Mode != AgentModeAIOpsDiagnosis || result.Diagnosis == nil || !result.Degraded {
		t.Fatalf("unexpected degraded diagnosis: %#v", result)
	}
	if result.DegradationReason != "diagnose_incident_failed" {
		t.Fatalf("unexpected degradation reason: %q", result.DegradationReason)
	}
}
