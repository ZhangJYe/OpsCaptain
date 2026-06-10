package service

import (
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/skills/domains/knowledge"
	"SuperBizAgent/internal/ai/skills/domains/logs"
	"SuperBizAgent/internal/ai/skills/domains/metrics"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/safety"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
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

	// changeEventBusHolder 通过 atomic.Pointer 暴露 ChangeEventBus，
	// 由 main 在启动阶段注入，被 AIOps 请求路径并发读取。
	// 包级 var 跨 goroutine 共享必须有同步，避免 -race 触发。
	changeEventBusHolder atomic.Pointer[changeEventQuery]
)

// changeEventQuery is the subset of ChangeEventBus needed for context injection.
type changeEventQuery interface {
	Query(ctx context.Context, filter protocol.ChangeEventFilter) ([]*protocol.ChangeEvent, error)
	RecentByService(service string, since time.Time, limit int) []*protocol.ChangeEvent
	RecentAll(since time.Time, limit int) []*protocol.ChangeEvent
}

// SetChangeEventBus 注入变更事件总线（从 main 调用）。传 nil 表示禁用。
func SetChangeEventBus(bus changeEventQuery) {
	if bus == nil {
		changeEventBusHolder.Store(nil)
		return
	}
	changeEventBusHolder.Store(&bus)
}

// loadChangeEventBus 原子读取当前注入的总线，可能为 nil。
func loadChangeEventBus() changeEventQuery {
	ptr := changeEventBusHolder.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

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

// buildChangeContext 从查询中提取服务名，查询最近变更事件，构建上下文。
// 所有写入 LLM prompt 的外部字段（summary/before/after/diff/operator）都会
// 经过 safety.SanitizeForLLMContext 清洗，避免变更事件成为 prompt-injection 通道。
func buildChangeContext(ctx context.Context, query string) string {
	bus := loadChangeEventBus()
	if bus == nil {
		return ""
	}
	services := extractServiceNames(query)
	since := time.Now().Add(-2 * time.Hour)
	if len(services) == 0 {
		since = time.Now().Add(-30 * time.Minute)
	}
	filter := protocol.ChangeEventFilter{
		Services: services,
		Since:    &since,
		Limit:    5,
	}
	changes, err := bus.Query(ctx, filter)
	if err != nil {
		// 降级路径：合并所有服务的最近内存事件，不只取 services[0]。
		changes = recentEventsFallback(bus, services, since, 5)
	}
	if len(changes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n最近相关变更事件（结构化查询）：\n")
	for _, c := range changes {
		b.WriteString(fmt.Sprintf("- [%s] %s %s@%s: %s (风险:%s, 操作者:%s, 时间:%s)\n",
			safety.SanitizeForLLMContext(c.EventType),
			safety.SanitizeForLLMContext(c.Service),
			safety.SanitizeForLLMContext(c.Env),
			safety.SanitizeForLLMContext(c.Cluster),
			safety.SanitizeForLLMContext(c.Summary),
			safety.SanitizeForLLMContext(c.RiskLevel),
			safety.SanitizeForLLMContext(c.Operator),
			c.StartedAt.Format("15:04")))
		if c.Before != nil || c.After != nil {
			b.WriteString(fmt.Sprintf("  Before: %s → After: %s\n",
				safety.SanitizeForLLMContext(stringifyMap(c.Before)),
				safety.SanitizeForLLMContext(stringifyMap(c.After))))
		}
		if c.Diff != "" {
			b.WriteString(fmt.Sprintf("  Diff: %s\n", safety.SanitizeForLLMContext(c.Diff)))
		}
	}
	return b.String()
}

// recentEventsFallback 合并多个服务的最近事件并按时间倒序，
// 替代「只取 services[0]」导致的关联丢失。
func recentEventsFallback(bus changeEventQuery, services []string, since time.Time, limit int) []*protocol.ChangeEvent {
	if bus == nil {
		return nil
	}
	if len(services) == 0 {
		return bus.RecentAll(since, limit)
	}
	merged := make([]*protocol.ChangeEvent, 0, limit*len(services))
	seen := make(map[string]bool, limit*len(services))
	for _, svc := range services {
		for _, e := range bus.RecentByService(svc, since, limit) {
			if e == nil || seen[e.EventID] {
				continue
			}
			seen[e.EventID] = true
			merged = append(merged, e)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].StartedAt.After(merged[j].StartedAt)
	})
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

// stringifyMap 用稳定排序的 key 序列化 map，避免 fmt %v 的随机顺序破坏可重现性。
func stringifyMap(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString(": ")
		fmt.Fprintf(&b, "%v", m[k])
	}
	b.WriteByte('}')
	return b.String()
}

// extractServiceNames 从查询文本中提取可能的服务名。
// 使用简单的启发式：查找包含 "-" 的词（如 user-service, order-api）。
func extractServiceNames(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	var services []string
	seen := make(map[string]bool)
	for _, w := range words {
		// 清理标点
		w = strings.Trim(w, ",.;:!?。，；：！？")
		if len(w) > 3 && strings.Contains(w, "-") && !seen[w] {
			seen[w] = true
			services = append(services, w)
		}
	}
	return services
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
