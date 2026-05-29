package service

import (
	"SuperBizAgent/internal/ai/skills/domains/knowledge"
	"SuperBizAgent/internal/ai/skills/domains/logs"
	"SuperBizAgent/internal/ai/skills/domains/metrics"
	"SuperBizAgent/internal/consts"
	"context"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/skills"

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

	skillFocusCollector = skills.NewFocusCollector(
		logs.SkillRegistry(),
		metrics.SkillRegistry(),
		knowledge.SkillRegistry(),
	)
	multiAgentConfigBool = func(ctx context.Context, key string) (bool, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || strings.TrimSpace(v.String()) == "" {
			return false, false
		}
		return v.Bool(), true
	}
)

type aiOpsMemory interface {
	ResolveSessionID(ctx context.Context) string
	BuildContextPlan(ctx context.Context, mode, sessionID, query string) (string, []protocol.MemoryRef, []string)
	PersistOutcome(ctx context.Context, sessionID, query, summary string)
}

type aiOpsIncidentContextKey struct{}

func WithAIOpsIncidentContext(ctx context.Context, contextText string) context.Context {
	contextText = strings.TrimSpace(contextText)
	if ctx == nil || contextText == "" {
		return ctx
	}
	return context.WithValue(ctx, aiOpsIncidentContextKey{}, contextText)
}

func aiOpsIncidentContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(aiOpsIncidentContextKey{}).(string)
	return strings.TrimSpace(value)
}

func RunAIOpsMultiAgent(ctx context.Context, query string) (ExecutionResponse, error) {
	agentName, agentAvailable, unavailableReason := resolveAIOpsAgentName(ctx)
	if !aiOpsMultiAgentEnabled(ctx) {
		response := multiAgentDisabledResponse()
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
	if incidentContext := aiOpsIncidentContext(ctx); incidentContext != "" {
		enrichedQuery = enrichedQuery + "\n\n当前事故排障会话：\n" + incidentContext
	}

	if hints := skillFocusCollector.Collect(query); len(hints) > 0 {
		enrichedQuery = enrichedQuery + "\n\n场景分析方向（基于 Skill 匹配）：\n" + skills.FormatFocusHints(hints)
		g.Log().Infof(ctx, "[AIOps] skill focus injected: %d hints", len(hints))
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

func ListApprovalRequests(ctx context.Context, status string) ([]ApprovalRequest, error) {
	return listApprovalRequests(ctx, parseApprovalStatus(status))
}

func RejectQueuedAIOpsRequest(ctx context.Context, requestID, reviewReason string) (*ApprovalRequest, error) {
	request, err := rejectApprovalRequest(ctx, requestID, reviewerIdentity(ctx), reviewReason)
	if err != nil {
		return nil, err
	}
	_ = RecordIncidentApprovalRejection(ctx, requestID, reviewReason)
	return request, nil
}

func ApproveQueuedAIOpsRequest(ctx context.Context, requestID string) (ExecutionResponse, error) {
	request, err := approveApprovalRequest(ctx, requestID, reviewerIdentity(ctx))
	if err != nil {
		return ExecutionResponse{}, err
	}

	runCtx := context.WithValue(ctx, consts.CtxKeyApprovalBypass, true)
	runCtx = context.WithValue(runCtx, consts.CtxKeyApprovalRequestID, requestID)
	if request.SessionID != "" {
		runCtx = context.WithValue(runCtx, consts.CtxKeySessionID, request.SessionID)
	}
	if request.UserID != "" {
		runCtx = context.WithValue(runCtx, consts.CtxKeyUserID, request.UserID)
	}
	runCtx = withIncidentApprovalRun(runCtx, requestID)

	response, err := RunAIOpsMultiAgent(runCtx, request.Query)
	if err != nil {
		return response, err
	}
	response.ApprovalRequestID = requestID
	response.ApprovalStatus = string(ApprovalStatusApproved)
	if response.TraceID != "" {
		if markErr := markApprovalRequestExecuted(ctx, requestID, response.TraceID); markErr == nil {
			response.ApprovalStatus = string(ApprovalStatusExecuted)
		}
	}
	_ = RecordIncidentApprovalExecution(runCtx, requestID, response)
	return response, nil
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
	if !aiOpsMultiAgentEnabled(ctx) {
		return &AIOpsRunInfo{
			Engine:            agentName,
			Status:            "degraded",
			Degraded:          true,
			DegradationReason: "multi_agent_disabled",
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

	if hints := skillFocusCollector.Collect(query); len(hints) > 0 {
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

		g.Log().Infof(bgCtx, "[AIOps] async dispatch started, trace_id=%s", rootTask.TraceID)
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

func reviewerIdentity(ctx context.Context) string {
	if userID, ok := ctx.Value(consts.CtxKeyUserID).(string); ok && userID != "" {
		return userID
	}
	return "system"
}

func parseApprovalStatus(status string) ApprovalStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(ApprovalStatusApproved):
		return ApprovalStatusApproved
	case string(ApprovalStatusRejected):
		return ApprovalStatusRejected
	case string(ApprovalStatusExecuted):
		return ApprovalStatusExecuted
	default:
		return ApprovalStatusPending
	}
}

func aiOpsMultiAgentEnabled(ctx context.Context) bool {
	if !multiAgentEnabled(ctx) {
		return false
	}
	enabled, configured := multiAgentConfigBool(ctx, "multi_agent.ai_ops_enabled")
	if configured {
		return enabled
	}
	return true
}

func multiAgentEnabled(ctx context.Context) bool {
	enabled, configured := multiAgentConfigBool(ctx, "multi_agent.enabled")
	if configured {
		return enabled
	}
	return true
}

func multiAgentDisabledResponse() ExecutionResponse {
	return ExecutionResponse{
		Content:           "Multi-agent runtime is disabled; use the Chat/RAG baseline route.",
		Detail:            []string{"multi_agent.enabled=false"},
		Status:            protocol.ResultStatusDegraded,
		DegradationReason: "multi_agent_disabled",
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
