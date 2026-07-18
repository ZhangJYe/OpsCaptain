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
	"time"

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
	if d := aiOpsAppTimeout(ctx); d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}

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
		message, detail := filterOutput(ctx, d.Message, []string{d.Reason})
		return &AIOpsResult{
			Result:            message,
			Detail:            detail,
			Degraded:          true,
			DegradationReason: filterAIOpsText(ctx, d.Reason),
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

	return &AIOpsResult{
		TraceID:           response.TraceID,
		Result:            result,
		Detail:            detail,
		Engine:            response.Engine,
		ApprovalRequired:  response.ApprovalRequired,
		ApprovalRequestID: response.ApprovalRequestID,
		ApprovalStatus:    response.ApprovalStatus,
		ExecutionPlan:     filterAIOpsStrings(ctx, response.ExecutionPlan),
		Degraded:          response.Degraded(),
		DegradationReason: filterAIOpsText(ctx, response.DegradationReason),
		Confidence:        response.Confidence,
		Evidence:          filterAIOpsEvidence(ctx, response.Evidence),
		NextActions:       filterAIOpsStrings(ctx, response.NextActions),
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
		DegradationReason: filterAIOpsText(ctx, runs.DegradationReason),
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
	return &AIOpsTraceResult{
		Events: filterAIOpsTraceEvents(ctx, events),
		Detail: filterAIOpsStrings(ctx, detail),
	}, nil
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
		Result:            filterAIOpsText(ctx, result.Content),
		Detail:            filterAIOpsStrings(ctx, result.Detail),
		Engine:            result.Engine,
		Confidence:        result.Confidence,
		Evidence:          filterAIOpsEvidence(ctx, result.Evidence),
		NextActions:       filterAIOpsStrings(ctx, result.NextActions),
		Degraded:          result.Degraded(),
		DegradationReason: filterAIOpsText(ctx, result.DegradationReason),
		StartedAt:         result.StartedAt,
		FinishedAt:        result.FinishedAt,
	}, nil
}

func filterAIOpsText(ctx context.Context, value string) string {
	return safety.FilterOutput(ctx, value).Content
}

func filterAIOpsStrings(ctx context.Context, values []string) []string {
	return safety.FilterDetails(ctx, values)
}

func filterAIOpsEvidence(ctx context.Context, evidence []protocol.EvidenceItem) []protocol.EvidenceItem {
	if len(evidence) == 0 {
		return nil
	}
	out := make([]protocol.EvidenceItem, len(evidence))
	for i, item := range evidence {
		item.SourceType = filterAIOpsText(ctx, item.SourceType)
		item.SourceID = filterAIOpsText(ctx, item.SourceID)
		item.Title = filterAIOpsText(ctx, item.Title)
		item.Snippet = filterAIOpsText(ctx, item.Snippet)
		item.URI = filterAIOpsText(ctx, item.URI)
		out[i] = item
	}
	return out
}

func filterAIOpsTraceEvents(ctx context.Context, events []*protocol.TaskEvent) []*protocol.TaskEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]*protocol.TaskEvent, len(events))
	for i, event := range events {
		if event == nil {
			continue
		}
		cloned := *event
		cloned.Message = filterAIOpsText(ctx, event.Message)
		cloned.Payload = filterAIOpsPayload(ctx, event.Payload)
		out[i] = &cloned
	}
	return out
}

func filterAIOpsPayload(ctx context.Context, payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = filterAIOpsPayloadValue(ctx, value)
	}
	return out
}

func filterAIOpsPayloadValue(ctx context.Context, value any) any {
	switch typed := value.(type) {
	case string:
		return filterAIOpsText(ctx, typed)
	case []string:
		return filterAIOpsStrings(ctx, typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = filterAIOpsPayloadValue(ctx, item)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, item := range typed {
			out[key] = filterAIOpsText(ctx, item)
		}
		return out
	case map[string]any:
		return filterAIOpsPayload(ctx, typed)
	default:
		return value
	}
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

func aiOpsAppTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "aiops.timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return 0
}
