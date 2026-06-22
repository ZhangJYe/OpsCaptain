package service

import (
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/skills/domains/knowledge"
	"SuperBizAgent/internal/ai/skills/domains/logs"
	"SuperBizAgent/internal/ai/skills/domains/metrics"
	"SuperBizAgent/internal/consts"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

var (
	newApprovalQueue = func() *ApprovalQueue {
		return NewApprovalQueue()
	}
	newMemoryService = func() aiOpsMemory {
		return NewMemoryService()
	}
	listApprovalRequests = func(ctx context.Context, status ApprovalStatus) ([]ApprovalRequest, error) {
		return newApprovalQueue().List(ctx, status)
	}
	rejectApprovalRequest = func(ctx context.Context, requestID, reviewer, reviewReason string) (*ApprovalRequest, error) {
		return newApprovalQueue().Reject(ctx, requestID, reviewer, reviewReason)
	}
	approveApprovalRequest = func(ctx context.Context, requestID, reviewer string) (*ApprovalRequest, error) {
		return newApprovalQueue().Approve(ctx, requestID, reviewer)
	}
	markApprovalRequestExecuted = func(ctx context.Context, requestID, traceID string) error {
		return newApprovalQueue().MarkExecuted(ctx, requestID, traceID)
	}

	sharedAIOpsRegistries []*skills.Registry
	multiAgentConfigBool = func(ctx context.Context, key string) (bool, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || strings.TrimSpace(v.String()) == "" {
			return false, false
		}
		return v.Bool(), true
	}
)

// SetSharedAIOpsRegistries sets shared skill registries for the AIOps skillFocusCollector.
// Must be called before any RunAIOps* functions.
func SetSharedAIOpsRegistries(registries []*skills.Registry) {
	sharedAIOpsRegistries = registries
}

var (
	skillFocusCollectorOnce sync.Once
	skillFocusCollectorIns  *skills.FocusCollector
)

func getSkillFocusCollector() *skills.FocusCollector {
	skillFocusCollectorOnce.Do(func() {
		registries := sharedAIOpsRegistries
		if len(registries) == 0 {
			registries = []*skills.Registry{
				logs.SkillRegistry(),
				metrics.SkillRegistry(),
				knowledge.SkillRegistry(),
			}
		}
		skillFocusCollectorIns = skills.NewFocusCollector(registries...)
	})
	return skillFocusCollectorIns
}

func RunAIOpsMultiAgent(ctx context.Context, query string) (ExecutionResponse, error) {
	agentName, agentAvailable, unavailableReason := resolveAIOpsAgentName(ctx)
	if !aiOpsEnabled(ctx) {
		response := aiOpsDisabledResponse()
		response.Engine = agentName
		return response, nil
	}
	if !agentAvailable {
		return aiOpsEngineUnavailableResponse(agentName, unavailableReason), nil
	}
	approval := NewApprovalGate()
	if decision := approval.Check(ctx, query); !decision.Approved {
		if decision.Queued && decision.ApprovalRequest != nil {
			return ExecutionResponse{
				Content:           decision.Reason,
				Detail:            []string{decision.Reason},
				Status:            protocol.ResultStatusSucceeded,
				Engine:            agentName,
				ApprovalRequired:  true,
				ApprovalRequestID: decision.ApprovalRequest.ID,
				ApprovalStatus:    string(decision.ApprovalRequest.Status),
				ExecutionPlan:     append([]string{}, decision.ApprovalRequest.ExecutionPlan...),
			}, nil
		}
		return ExecutionResponse{
			Content: decision.Reason,
			Detail:  []string{decision.Reason},
			Status:  protocol.ResultStatusSucceeded,
			Engine:  agentName,
		}, nil
	}

	if decision := GetDegradationDecision(ctx, "ai_ops"); decision.Enabled {
		response := NewDegradedExecutionResponse(decision)
		response.Engine = agentName
		return response, nil
	}

	memorySvc := newMemoryService()
	sessionID := memorySvc.ResolveSessionID(ctx)
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, sessionID)
	memoryContext, _, contextDetail := memorySvc.BuildContextPlan(ctx, "aiops", sessionID, query)

	enrichedQuery := query
	if strings.TrimSpace(memoryContext) != "" {
		enrichedQuery = query + "\n\n可参考的历史上下文：\n" + memoryContext
	}
	// 注入最近变更事件上下文（结构化查询优先）
	if changeCtx := buildChangeContext(ctx, query); changeCtx != "" {
		enrichedQuery += changeCtx
	}
	if incidentContext := aiOpsIncidentContext(ctx); incidentContext != "" {
		enrichedQuery = enrichedQuery + "\n\n当前事故排障会话：\n" + incidentContext
	}

	if hints := getSkillFocusCollector().Collect(query); len(hints) > 0 {
		enrichedQuery = enrichedQuery + "\n\n场景分析方向（基于 Skill 匹配）：\n" + skills.FormatFocusHints(hints)
		g.Log().Infof(ctx, "[AIOps] skill focus injected: %d hints", len(hints))
	}

	// 用户显式选择的 skills focus hints
	if selectedSkillIDs := skills.SelectedSkillIDsFromContext(ctx); len(selectedSkillIDs) > 0 {
		if selectedHints := getSkillFocusCollector().ResolveSelected(selectedSkillIDs); len(selectedHints) > 0 {
			enrichedQuery += "\n\n用户指定的分析方向：\n" + skills.FormatFocusHints(selectedHints)
			g.Log().Infof(ctx, "[AIOps] selected skills focus injected: %d hints", len(selectedHints))
		}
	}

	rt, err := getOrCreateAIOpsRuntime(ctx)
	if err != nil {
		return ExecutionResponse{
			Detail:            append(contextDetail, fmt.Sprintf("aiops runtime unavailable: %v", err)),
			Status:            protocol.ResultStatusFailed,
			DegradationReason: err.Error(),
		}, err
	}

	rootTask := protocol.NewRootTask(sessionID, query, agentName)
	rootTask.Input = map[string]any{
		"raw_query":        query,
		"executable_query": enrichedQuery,
		"context_detail":   append([]string{}, contextDetail...),
		"response_mode":    "ai_ops",
		"entrypoint":       "ai_ops",
		"engine":           agentName,
	}

	g.Log().Infof(ctx, "[AIOps] runtime dispatch started, trace_id=%s", rootTask.TraceID)
	result, err := rt.Dispatch(ctx, rootTask)
	detail := append([]string{}, contextDetail...)
	detail = append(detail, rt.DetailMessages(ctx, rootTask.TraceID)...)
	if result == nil {
		return ExecutionResponse{
			Detail:            detail,
			TraceID:           rootTask.TraceID,
			Status:            protocol.ResultStatusFailed,
			DegradationReason: "aiops execution returned no result",
		}, err
	}

	memorySvc.PersistOutcome(ctx, sessionID, query, result.Summary)
	g.Log().Infof(ctx, "[AIOps] runtime dispatch completed, trace_id=%s", rootTask.TraceID)

	return ExecutionResponseFromResult(result, detail, rootTask.TraceID), err
}

type AIOpsRunInfo struct {
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

func RunAIOpsAsync(ctx context.Context, query string) (*AIOpsRunInfo, error) {
	agentName, agentAvailable, unavailableReason := resolveAIOpsAgentName(ctx)
	if !aiOpsEnabled(ctx) {
		return &AIOpsRunInfo{
			Engine:            agentName,
			Status:            "degraded",
			Degraded:          true,
			DegradationReason: "aiops_disabled",
		}, nil
	}
	if !agentAvailable {
		return aiOpsEngineUnavailableRunInfo(agentName, unavailableReason), nil
	}

	approval := NewApprovalGate()
	if decision := approval.Check(ctx, query); !decision.Approved {
		if decision.Queued && decision.ApprovalRequest != nil {
			return &AIOpsRunInfo{
				Engine:            agentName,
				Status:            "approval_required",
				ApprovalRequired:  true,
				ApprovalRequestID: decision.ApprovalRequest.ID,
				ApprovalStatus:    string(decision.ApprovalRequest.Status),
			}, nil
		}
		return &AIOpsRunInfo{
			Engine:            agentName,
			Status:            "degraded",
			Degraded:          true,
			DegradationReason: decision.Reason,
		}, nil
	}

	if decision := GetDegradationDecision(ctx, "ai_ops"); decision.Enabled {
		return &AIOpsRunInfo{
			Engine:            agentName,
			Status:            "degraded",
			Degraded:          true,
			DegradationReason: decision.Reason,
		}, nil
	}

	rt, err := getOrCreateAIOpsRuntime(ctx)
	if err != nil {
		return nil, fmt.Errorf("aiops runtime unavailable: %w", err)
	}

	memorySvc := newMemoryService()
	sessionID := memorySvc.ResolveSessionID(ctx)
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, sessionID)
	memoryContext, _, _ := memorySvc.BuildContextPlan(ctx, "aiops", sessionID, query)

	enrichedQuery := query
	if strings.TrimSpace(memoryContext) != "" {
		enrichedQuery = query + "\n\n可参考的历史上下文：\n" + memoryContext
	}
	// 注入最近变更事件上下文（结构化查询优先）
	if changeCtx := buildChangeContext(ctx, query); changeCtx != "" {
		enrichedQuery += changeCtx
	}

	if hints := getSkillFocusCollector().Collect(query); len(hints) > 0 {
		enrichedQuery = enrichedQuery + "\n\n场景分析方向（基于 Skill 匹配）：\n" + skills.FormatFocusHints(hints)
	}

	rootTask := protocol.NewRootTask(sessionID, query, agentName)
	rootTask.Input = map[string]any{
		"raw_query":        query,
		"executable_query": enrichedQuery,
		"response_mode":    "ai_ops",
		"entrypoint":       "ai_ops",
		"engine":           agentName,
	}

	runInfo := &AIOpsRunInfo{
		TraceID: rootTask.TraceID,
		TaskID:  rootTask.TaskID,
		Engine:  agentName,
		Status:  "running",
	}

	go func() {
		bgCtx := context.Background()
		bgCtx = context.WithValue(bgCtx, consts.CtxKeySessionID, sessionID)
		bgCtx = context.WithValue(bgCtx, consts.CtxKeyRequestID, ctx.Value(consts.CtxKeyRequestID))
		bgCtx = WithAIOpsEngine(bgCtx, agentName)

		asyncTimeout := aiOpsAsyncTimeout(ctx)
		bgCtx, cancel := context.WithTimeout(bgCtx, asyncTimeout)
		defer cancel()

		g.Log().Infof(bgCtx, "[AIOps] async dispatch started, trace_id=%s, timeout=%s", rootTask.TraceID, asyncTimeout)
		result, dispatchErr := rt.Dispatch(bgCtx, rootTask)
		if dispatchErr != nil {
			g.Log().Warningf(bgCtx, "[AIOps] async dispatch failed, trace_id=%s: %v", rootTask.TraceID, dispatchErr)
		}
		if result != nil {
			memorySvc.PersistOutcome(bgCtx, sessionID, query, result.Summary)
		}
		g.Log().Infof(bgCtx, "[AIOps] async dispatch completed, trace_id=%s", rootTask.TraceID)
	}()

	return runInfo, nil
}

func GetAIOpsResult(ctx context.Context, traceID string) (*ExecutionResponse, error) {
	if strings.TrimSpace(traceID) == "" {
		return nil, nil
	}
	rt, err := getOrCreateAIOpsRuntime(ctx)
	if err != nil {
		return nil, fmt.Errorf("aiops runtime unavailable: %w", err)
	}
	result, err := rt.ResultByTraceID(ctx, traceID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	events, _ := rt.TraceEvents(ctx, traceID)
	detail := make([]string, 0, len(events))
	for _, ev := range events {
		if ev.Message != "" && includeTraceEvent(ev.Type) {
			detail = append(detail, ev.Message)
		}
	}
	resp := ExecutionResponseFromResult(result, detail, traceID)
	return &resp, nil
}

func includeTraceEvent(eventType string) bool {
	switch eventType {
	case "task_started", "task_info", "task_completed":
		return true
	default:
		return false
	}
}

func GetAIOpsTrace(_ context.Context, traceID string) ([]*protocol.TaskEvent, []string, error) {
	if traceID == "" {
		return nil, nil, fmt.Errorf("traceID is empty")
	}
	rt, err := getOrCreateAIOpsRuntime(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("aiops runtime unavailable: %w", err)
	}
	events, err := rt.TraceEvents(context.Background(), traceID)
	if err != nil {
		return nil, nil, err
	}
	if len(events) == 0 {
		return nil, nil, fmt.Errorf("trace %s not found", traceID)
	}
	return events, rt.DetailMessages(context.Background(), traceID), nil
}

// asyncTimeoutOverrideKey 用于允许调用方（如 ProactiveAnalyzer）覆盖
// RunAIOpsAsync 的 dispatch 超时；优先级高于配置项 aiops.async_timeout_ms。
type asyncTimeoutOverrideKey struct{}

// WithAsyncTimeoutOverride 在 ctx 中注入异步派发的超时覆盖值。
// 用于让上游配置（如 change_events.proactive.inspection_timeout_ms）
// 真正生效到 RunAIOpsAsync 内部派生的后台 goroutine。
func WithAsyncTimeoutOverride(ctx context.Context, timeout time.Duration) context.Context {
	if timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, asyncTimeoutOverrideKey{}, timeout)
}

func aiOpsAsyncTimeout(ctx context.Context) time.Duration {
	const defaultTimeout = 5 * time.Minute
	if ctx != nil {
		if override, ok := ctx.Value(asyncTimeoutOverrideKey{}).(time.Duration); ok && override > 0 {
			return override
		}
	}
	v, err := g.Cfg().Get(ctx, "aiops.async_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultTimeout
}

// aiOpsEnabled checks the primary AIOps switch.
// Reads aiops.enabled first; falls back to multi_agent.enabled / multi_agent.ai_ops_enabled
// for backward compatibility.
func aiOpsEnabled(ctx context.Context) bool {
	// Primary: aiops.enabled (new canonical key)
	if enabled, configured := multiAgentConfigBool(ctx, "aiops.enabled"); configured {
		return enabled
	}
	// Fallback: multi_agent.enabled (legacy)
	if enabled, configured := multiAgentConfigBool(ctx, "multi_agent.enabled"); configured {
		if !enabled {
			return false
		}
	}
	// Fallback: multi_agent.ai_ops_enabled (legacy)
	if enabled, configured := multiAgentConfigBool(ctx, "multi_agent.ai_ops_enabled"); configured {
		return enabled
	}
	return true
}

func aiOpsDisabledResponse() ExecutionResponse {
	return ExecutionResponse{
		Content:           "AIOps is disabled; use the Chat/RAG baseline route.",
		Detail:            []string{"aiops.enabled=false"},
		Status:            protocol.ResultStatusDegraded,
		DegradationReason: "aiops_disabled",
	}
}

func aiOpsEngineUnavailableResponse(agentName, reason string) ExecutionResponse {
	return ExecutionResponse{
		Content:           reason,
		Detail:            []string{reason},
		Engine:            agentName,
		Status:            protocol.ResultStatusDegraded,
		DegradationReason: reason,
	}
}

func aiOpsEngineUnavailableRunInfo(agentName, reason string) *AIOpsRunInfo {
	return &AIOpsRunInfo{
		Engine:            agentName,
		Status:            "degraded",
		Degraded:          true,
		DegradationReason: reason,
	}
}
