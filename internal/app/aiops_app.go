package app

import (
	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/ai/protocol"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/safety"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

const defaultAIOpsQuery = `你是一个 AIOps 事故分析助手，请严格按以下顺序执行：
1. 查询当前活跃的 Prometheus 告警。
2. 对每条告警查询匹配的内部文档或 runbook。
3. 只能基于工具结果和内部文档进行分析。
4. 如果某个工具失败，跳过该步骤，并在报告中明确说明一次。
5. 默认使用中文输出报告，除非用户明确要求其他语言。
6. 报告使用 Markdown，包含这些章节：活跃告警、根因分析、缓解建议、结论。`

type AIOpsApp struct {
	runMultiAgent    func(ctx context.Context, query string) (aiservice.ExecutionResponse, error)
	runAsync         func(ctx context.Context, query string) (*aiservice.AIOpsRunInfo, error)
	getResult        func(ctx context.Context, traceID string) (*aiservice.ExecutionResponse, error)
	getTrace         func(ctx context.Context, traceID string) ([]*protocol.TaskEvent, []string, error)
	degradationCheck func(ctx context.Context, entrypoint string) aiservice.DegradationDecision
}

func NewAIOpsApp() *AIOpsApp {
	return &AIOpsApp{
		runMultiAgent:    aiservice.RunAIOpsMultiAgent,
		runAsync:         aiservice.RunAIOpsAsync,
		getResult:        aiservice.GetAIOpsResult,
		getTrace:         aiservice.GetAIOpsTrace,
		degradationCheck: aiservice.GetDegradationDecision,
	}
}

func (a *AIOpsApp) SetRunMultiAgent(fn func(ctx context.Context, query string) (aiservice.ExecutionResponse, error)) {
	a.runMultiAgent = fn
}

func (a *AIOpsApp) SetDegradationCheck(fn func(ctx context.Context, entrypoint string) aiservice.DegradationDecision) {
	a.degradationCheck = fn
}

func (a *AIOpsApp) SetRunAsync(fn func(ctx context.Context, query string) (*aiservice.AIOpsRunInfo, error)) {
	a.runAsync = fn
}

func (a *AIOpsApp) SetGetResult(fn func(ctx context.Context, traceID string) (*aiservice.ExecutionResponse, error)) {
	a.getResult = fn
}

func (a *AIOpsApp) SetGetTrace(fn func(ctx context.Context, traceID string) ([]*protocol.TaskEvent, []string, error)) {
	a.getTrace = fn
}

type AIOpsInput struct {
	SessionID string
	Query     string
	Engine    string
}

type AIOpsResult struct {
	TraceID           string
	Result            string
	Detail            []string
	Engine            string
	ApprovalRequired  bool
	ApprovalRequestID string
	ApprovalStatus    string
	ExecutionPlan     []string
	Degraded          bool
	DegradationReason string
	Confidence        float64
	Evidence          []protocol.EvidenceItem
	NextActions       []string
	StartedAt         int64
	FinishedAt        int64
	HTTPStatus        int
}

type AIOpsRunsInput struct {
	Query  string
	Engine string
}

type AIOpsRunsResult struct {
	TraceID           string
	TaskID            string
	Engine            string
	Status            string
	Degraded          bool
	DegradationReason string
	ApprovalRequired  bool
	ApprovalRequestID string
	ApprovalStatus    string
}

type AIOpsTraceResult struct {
	Events []*protocol.TaskEvent
	Detail []string
}

type AIOpsLookupResult struct {
	Found             bool
	Status            string
	TraceID           string
	Result            string
	Detail            []string
	Engine            string
	Confidence        float64
	Evidence          []protocol.EvidenceItem
	NextActions       []string
	Degraded          bool
	DegradationReason string
	StartedAt         int64
	FinishedAt        int64
}

func (a *AIOpsApp) HandleAIOps(ctx context.Context, input *AIOpsInput) (*AIOpsResult, error) {
	requestID := guid.S()
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID != "" {
		if err := memory.ValidateSessionID(sessionID); err != nil {
			return nil, fmt.Errorf("invalid session ID: %w", err)
		}
		ctx = context.WithValue(ctx, consts.CtxKeySessionID, sessionID)
	}
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = enrichContext(ctx, sessionID, requestID)

	ctx, promptErr := validatePrompt(ctx, input.Query)
	if promptErr != nil {
		return nil, promptErr
	}
	if d := a.degradationCheck(ctx, "ai_ops"); d.Enabled {
		return &AIOpsResult{
			Result:            d.Message,
			Detail:            []string{d.Reason},
			Degraded:          true,
			DegradationReason: d.Reason,
		}, nil
	}

	query := input.Query
	ctx = aiservice.WithAIOpsEngine(ctx, input.Engine)
	if query == "" {
		query = defaultAIOpsQuery
	}

	response, err := a.runMultiAgent(ctx, query)
	if err != nil {
		if status, message := classifyError(err); status != 0 {
			return &AIOpsResult{
				Result:            message,
				Detail:            []string{message},
				Degraded:          true,
				DegradationReason: message,
				HTTPStatus:        status,
			}, nil
		}
		return nil, err
	}

	result := response.Content
	if result == "" {
		if len(response.Detail) > 0 && response.Detail[0] != "" {
			result = response.Detail[0]
		} else {
			return nil, errors.New("internal error")
		}
	}
	result, detail := filterOutput(ctx, result, response.Detail)
	executionPlan := make([]string, 0, len(response.ExecutionPlan))
	for _, step := range response.ExecutionPlan {
		filtered, _ := filterOutput(ctx, step, nil)
		executionPlan = append(executionPlan, filtered)
	}

	return &AIOpsResult{
		TraceID:           response.TraceID,
		Result:            result,
		Detail:            detail,
		Engine:            response.Engine,
		ApprovalRequired:  response.ApprovalRequired,
		ApprovalRequestID: response.ApprovalRequestID,
		ApprovalStatus:    response.ApprovalStatus,
		ExecutionPlan:     executionPlan,
		Degraded:          response.Degraded(),
		DegradationReason: response.DegradationReason,
		Confidence:        response.Confidence,
		Evidence:          response.Evidence,
		NextActions:       response.NextActions,
		StartedAt:         response.StartedAt,
		FinishedAt:        response.FinishedAt,
	}, nil
}

func (a *AIOpsApp) HandleAIOpsRuns(ctx context.Context, input *AIOpsRunsInput) (*AIOpsRunsResult, error) {
	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = enrichContext(ctx, "", requestID)

	ctx, promptErr := validatePrompt(ctx, input.Query)
	if promptErr != nil {
		return nil, promptErr
	}

	query := input.Query
	ctx = aiservice.WithAIOpsEngine(ctx, input.Engine)
	if query == "" {
		query = defaultAIOpsQuery
	}

	runs, err := a.runAsync(ctx, query)
	if err != nil {
		return nil, err
	}

	return &AIOpsRunsResult{
		TraceID:           runs.TraceID,
		TaskID:            runs.TaskID,
		Engine:            runs.Engine,
		Status:            runs.Status,
		Degraded:          runs.Degraded,
		DegradationReason: runs.DegradationReason,
		ApprovalRequired:  runs.ApprovalRequired,
		ApprovalRequestID: runs.ApprovalRequestID,
		ApprovalStatus:    runs.ApprovalStatus,
	}, nil
}

func (a *AIOpsApp) HandleAIOpsTrace(ctx context.Context, traceID string) (*AIOpsTraceResult, error) {
	events, detail, err := a.getTrace(ctx, traceID)
	if err != nil {
		return nil, err
	}
	return &AIOpsTraceResult{Events: events, Detail: detail}, nil
}

func (a *AIOpsApp) HandleAIOpsResult(ctx context.Context, traceID string) (*AIOpsLookupResult, error) {
	result, err := a.getResult(ctx, traceID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &AIOpsLookupResult{Found: false, TraceID: traceID}, nil
	}
	return &AIOpsLookupResult{
		Found:             true,
		Status:            string(result.Status),
		TraceID:           traceID,
		Result:            result.Content,
		Detail:            result.Detail,
		Engine:            result.Engine,
		Confidence:        result.Confidence,
		Evidence:          result.Evidence,
		NextActions:       result.NextActions,
		Degraded:          result.Degraded(),
		DegradationReason: result.DegradationReason,
		StartedAt:         result.StartedAt,
		FinishedAt:        result.FinishedAt,
	}, nil
}

func validatePrompt(ctx context.Context, input string) (context.Context, error) {
	decision := safety.CheckPrompt(ctx, input)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskScore, decision.RiskScore)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskLevel, decision.RiskLevel)
	if !decision.Allowed {
		return ctx, &PromptRejectedError{
			Reason:    decision.Reason,
			RiskScore: decision.RiskScore,
			RiskLevel: decision.RiskLevel,
			Pattern:   decision.Pattern,
		}
	}
	if decision.RiskLevel == "suspicious" {
		g.Log().Warningf(ctx, "[prompt_guard] suspicious input detected, risk_score=%.2f reason=%q", decision.RiskScore, decision.Reason)
	}
	return ctx, nil
}
