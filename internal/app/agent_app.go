package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/memory"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

type AgentMode string

const (
	AgentModeAuto           AgentMode = "auto"
	AgentModeChat           AgentMode = "chat"
	AgentModeAIOpsDiagnosis AgentMode = "aiops_diagnosis"

	diagnoseIncidentToolName = "diagnose_incident"
)

type AgentInput struct {
	SessionID string
	Query     string
	Mode      AgentMode
	SkillIDs  []string
}

type AgentResult struct {
	TraceID           string
	Mode              AgentMode
	Chat              *ChatResult
	Diagnosis         *AIOpsResult
	Degraded          bool
	DegradationReason string
	HTTPStatus        int
}

type AgentApp struct {
	chatApp      *ChatApp
	aiopsApp     *AIOpsApp
	enabledCheck func(context.Context) bool
	runAutoChat  func(context.Context, *ChatInput, tool.BaseTool) (*ChatResult, error)
}

func NewAgentApp(chatApp *ChatApp, aiopsApp *AIOpsApp) *AgentApp {
	a := &AgentApp{
		chatApp:      chatApp,
		aiopsApp:     aiopsApp,
		enabledCheck: agentGatewayEnabled,
	}
	a.runAutoChat = func(ctx context.Context, input *ChatInput, diagnosisTool tool.BaseTool) (*ChatResult, error) {
		return chatApp.HandleChat(chat_pipeline.WithAutoDiagnosisTool(ctx, diagnosisTool), input)
	}
	return a
}

func (a *AgentApp) SetEnabledCheck(fn func(context.Context) bool) {
	if fn != nil {
		a.enabledCheck = fn
	}
}

func (a *AgentApp) SetRunAutoChat(fn func(context.Context, *ChatInput, tool.BaseTool) (*ChatResult, error)) {
	if fn != nil {
		a.runAutoChat = fn
	}
}

func (a *AgentApp) HandleAgent(ctx context.Context, input *AgentInput) (*AgentResult, error) {
	if input == nil {
		return nil, fmt.Errorf("agent input is required")
	}
	if err := memory.ValidateSessionID(input.SessionID); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}
	if strings.TrimSpace(input.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	mode, err := normalizeAgentMode(input.Mode)
	if err != nil {
		return nil, err
	}
	if !a.enabledCheck(ctx) {
		message := "统一 Agent 入口尚未启用，请暂时使用现有 Chat 或 AIOps 入口。"
		return &AgentResult{
			Mode:              AgentModeChat,
			Chat:              &ChatResult{Answer: message, Detail: []string{"agent_gateway_disabled"}, Mode: "degraded", Degraded: true, DegradationReason: "agent_gateway_disabled"},
			Degraded:          true,
			DegradationReason: "agent_gateway_disabled",
			HTTPStatus:        503,
		}, nil
	}

	switch mode {
	case AgentModeChat:
		chatResult, handleErr := a.chatApp.HandleChat(ctx, &ChatInput{
			SessionID: input.SessionID,
			Question:  input.Query,
			SkillIDs:  input.SkillIDs,
		})
		if handleErr != nil {
			return nil, handleErr
		}
		return agentResultFromChat(chatResult), nil

	case AgentModeAIOpsDiagnosis:
		diagnosis, handleErr := a.aiopsApp.HandleAIOps(ctx, &AIOpsInput{
			SessionID: input.SessionID,
			Query:     input.Query,
		})
		if handleErr != nil {
			return nil, handleErr
		}
		return agentResultFromDiagnosis(diagnosis), nil

	case AgentModeAuto:
		return a.handleAuto(ctx, input)
	default:
		return nil, fmt.Errorf("unsupported agent mode %q", mode)
	}
}

func (a *AgentApp) handleAuto(ctx context.Context, input *AgentInput) (*AgentResult, error) {
	capture := &diagnosisCapture{}
	diagnosisTool, err := a.newDiagnoseIncidentTool(input, capture)
	if err != nil {
		return nil, err
	}

	chatResult, err := a.runAutoChat(ctx, &ChatInput{
		SessionID:    input.SessionID,
		Question:     input.Query,
		SkillIDs:     input.SkillIDs,
		DisableCache: true,
	}, diagnosisTool)
	if err != nil {
		return nil, err
	}
	if diagnosis := capture.Result(); diagnosis != nil {
		return agentResultFromDiagnosis(diagnosis), nil
	}
	return agentResultFromChat(chatResult), nil
}

type DiagnoseIncidentInput struct{}

type diagnosisCapture struct {
	mu     sync.Mutex
	result *AIOpsResult
}

func (c *diagnosisCapture) Set(result *AIOpsResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result = result
}

func (c *diagnosisCapture) Result() *AIOpsResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.result
}

func (a *AgentApp) newDiagnoseIncidentTool(input *AgentInput, capture *diagnosisCapture) (tool.InvokableTool, error) {
	return utils.InferOptionableTool(
		diagnoseIncidentToolName,
		"当用户要求查询真实系统状态、日志、指标、告警，或定位当前实际故障时调用。概念解释、文档问答和普通对话不要调用。该工具只进行受权限和审批保护的证据化诊断，不能选择内部引擎，也不会直接执行重启、部署或配置修改。",
		func(ctx context.Context, _ *DiagnoseIncidentInput, _ ...tool.Option) (string, error) {
			result, err := a.aiopsApp.HandleAIOps(ctx, &AIOpsInput{
				SessionID: input.SessionID,
				Query:     input.Query,
			})
			if err != nil || result == nil {
				degraded := &AIOpsResult{
					Result:            "故障诊断暂时不可用，请稍后重试或使用显式 AIOps 入口。",
					Detail:            []string{"diagnose_incident_failed"},
					Degraded:          true,
					DegradationReason: "diagnose_incident_failed",
				}
				capture.Set(degraded)
				return degraded.Result, nil
			}
			capture.Set(result)
			return result.Result, nil
		},
	)
}

func normalizeAgentMode(mode AgentMode) (AgentMode, error) {
	switch AgentMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case "", AgentModeAuto:
		return AgentModeAuto, nil
	case AgentModeChat:
		return AgentModeChat, nil
	case AgentModeAIOpsDiagnosis:
		return AgentModeAIOpsDiagnosis, nil
	default:
		return "", fmt.Errorf("invalid agent mode %q", mode)
	}
}

func agentResultFromChat(result *ChatResult) *AgentResult {
	return &AgentResult{
		TraceID:           result.TraceID,
		Mode:              AgentModeChat,
		Chat:              result,
		Degraded:          result.Degraded,
		DegradationReason: result.DegradationReason,
		HTTPStatus:        result.HTTPStatus,
	}
}

func agentResultFromDiagnosis(result *AIOpsResult) *AgentResult {
	return &AgentResult{
		TraceID:           result.TraceID,
		Mode:              AgentModeAIOpsDiagnosis,
		Diagnosis:         result,
		Degraded:          result.Degraded,
		DegradationReason: result.DegradationReason,
		HTTPStatus:        result.HTTPStatus,
	}
}

func agentGatewayEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "agent_gateway.enabled")
	return err == nil && v.Bool()
}
