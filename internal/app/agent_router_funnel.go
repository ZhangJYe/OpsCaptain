package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type AgentRouteIntent string

const (
	AgentRouteIntentKnowledgeQA       AgentRouteIntent = "knowledge_qa"
	AgentRouteIntentIncidentDiagnosis AgentRouteIntent = "incident_diagnosis"
	AgentRouteIntentIncidentFollowup  AgentRouteIntent = "incident_followup"
	AgentRouteIntentResourceQuery     AgentRouteIntent = "resource_query"
	AgentRouteIntentActionRequest     AgentRouteIntent = "action_request"
	AgentRouteIntentOutOfScope        AgentRouteIntent = "out_of_scope"
)

type AgentRouteRisk string

const (
	AgentRouteRiskLow    AgentRouteRisk = "low"
	AgentRouteRiskMedium AgentRouteRisk = "medium"
	AgentRouteRiskHigh   AgentRouteRisk = "high"
)

type AgentRouteCandidate struct {
	Intent      AgentRouteIntent `json:"intent"`
	Confidence  float64          `json:"confidence"`
	ReasonCodes []string         `json:"reason_codes,omitempty"`
}

// RoutingContextSnapshot contains only confirmed routing state. It must not
// contain long-term memory, free-text summaries, tool output, or chat history.
type RoutingContextSnapshot struct {
	ActiveRoute         AgentRouteDecision `json:"active_route,omitempty"`
	ActiveIncidentID    string             `json:"active_incident_id,omitempty"`
	LastConfirmedIntent AgentRouteIntent   `json:"last_confirmed_intent,omitempty"`
	ConfirmedEntities   map[string]string  `json:"confirmed_entities,omitempty"`
	PendingSlots        []string           `json:"pending_slots,omitempty"`
	StateVersion        string             `json:"state_version,omitempty"`
	UpdatedAt           time.Time          `json:"updated_at,omitempty"`
}

type AgentRouteClarification struct {
	ID           string             `json:"id"`
	Question     string             `json:"question"`
	MissingSlots []string           `json:"missing_slots,omitempty"`
	Candidates   []AgentRouteIntent `json:"candidates,omitempty"`
	StateVersion string             `json:"state_version,omitempty"`
	Round        int                `json:"round"`
	ExpiresAt    time.Time          `json:"expires_at"`
}

type AgentRouteClarificationAnswer struct {
	ID           string `json:"id"`
	Slot         string `json:"slot,omitempty"`
	Value        string `json:"value"`
	StateVersion string `json:"state_version,omitempty"`
	Round        int    `json:"round"`
}

type AgentRouteLayerTrace struct {
	Layer       string   `json:"layer"`
	Outcome     string   `json:"outcome"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
	LatencyMS   int64    `json:"latency_ms"`
}

type AgentRouteTrace struct {
	PolicyVersion      string                 `json:"policy_version"`
	QueryHash          string                 `json:"query_hash"`
	ContextFingerprint string                 `json:"context_fingerprint,omitempty"`
	ContextUsed        bool                   `json:"context_used"`
	ContextReason      string                 `json:"context_reason,omitempty"`
	DependencyStatus   string                 `json:"dependency_status"`
	Layers             []AgentRouteLayerTrace `json:"layers"`
	TotalLatencyMS     int64                  `json:"total_latency_ms"`
}

func (a *AgentRouterApp) decideIntentFunnel(ctx context.Context, input *AgentRouteInput, mode AgentRouteMode, strategy AgentDiagnosisStrategy, cfg AgentRouterConfig) *AgentRouteResult {
	startedAt := a.now()
	trace := &AgentRouteTrace{
		PolicyVersion:      cfg.IntentFunnelPolicyVersion,
		QueryHash:          routeQueryHash(input.Query),
		ContextFingerprint: routingContextFingerprint(input.RoutingContext),
		DependencyStatus:   "not_called",
	}

	layerStartedAt := a.now()
	if result := a.applyDeterministicRouteGuard(input, strategy, cfg); result != nil {
		trace.Layers = append(trace.Layers, routeLayerTrace("layer1", "terminal", []string{result.Reason}, a.now().Sub(layerStartedAt)))
		return finishFunnelResult(result, trace, startedAt, a.now())
	}
	trace.Layers = append(trace.Layers, routeLayerTrace("layer1", "continue", nil, a.now().Sub(layerStartedAt)))

	layerStartedAt = a.now()
	output, cacheHit := a.loadCandidateCache(cfg, input.Query)
	if !cacheHit {
		routeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		raw, err := a.generate(routeCtx, strings.TrimSpace(input.Query))
		if err != nil {
			trace.DependencyStatus = "degraded"
			trace.Layers = append(trace.Layers, routeLayerTrace("layer2", "degraded", []string{"classifier_unavailable"}, a.now().Sub(layerStartedAt)))
			return finishFunnelResult(confirmRoute("智能路由暂不可用，请手动选择执行方式"), trace, startedAt, a.now())
		}
		output, err = parseFunnelRouteOutput(raw, cfg.IntentFunnelTopK)
		if err != nil {
			trace.DependencyStatus = "invalid_schema"
			trace.Layers = append(trace.Layers, routeLayerTrace("layer2", "degraded", []string{"invalid_classifier_schema"}, a.now().Sub(layerStartedAt)))
			return finishFunnelResult(confirmRoute("路由结果无法确认，请手动选择执行方式"), trace, startedAt, a.now())
		}
		a.cacheCandidates(cfg, input.Query, output)
	}
	if input.Clarification != nil && input.RoutingContext != nil && input.Clarification.StateVersion != "" && input.Clarification.StateVersion != input.RoutingContext.StateVersion {
		trace.ContextReason = "clarification_state_changed"
		trace.Layers = append(trace.Layers, routeLayerTrace("layer3", "clarify", []string{"clarification_state_changed"}, 0))
		return finishFunnelResult(newClarificationResult(input, output, cfg, "clarification_state_changed", a.now()), trace, startedAt, a.now())
	}
	applyClarificationAnswer(output, input.Clarification)
	if cacheHit {
		trace.DependencyStatus = "candidate_cache"
	} else {
		trace.DependencyStatus = "ok"
	}
	trace.Layers = append(trace.Layers, routeLayerTrace("layer2", "candidates", candidateReasonCodes(output.Candidates), a.now().Sub(layerStartedAt)))

	layerStartedAt = a.now()
	selected, contextUsed, contextReason := selectCandidateWithContext(input.Query, output.Candidates, input.RoutingContext, cfg.IntentFunnelContextTTL, a.now())
	trace.ContextUsed = contextUsed
	trace.ContextReason = contextReason
	if contextReason != "" && contextReason != "topic_switch" {
		trace.Layers = append(trace.Layers, routeLayerTrace("layer3", "clarify", []string{contextReason}, a.now().Sub(layerStartedAt)))
		return finishFunnelResult(newClarificationResult(input, output, cfg, contextReason, a.now()), trace, startedAt, a.now())
	}
	if contextUsed && selected.Confidence < cfg.IntentFunnelAcceptThreshold {
		selected.Confidence = cfg.IntentFunnelAcceptThreshold
	}

	result := routeCandidateResult(selected, output, mode, strategy, cfg)
	if result.Decision == AgentRouteDecisionConfirm && result.Clarification == nil && result.RiskHint != AgentRouteRiskHigh && selected.Intent != AgentRouteIntentOutOfScope {
		result = newClarificationResult(input, output, cfg, result.Reason, a.now())
	}
	trace.Layers = append(trace.Layers, routeLayerTrace("layer3", string(result.Decision), candidateReasonCodes([]AgentRouteCandidate{selected}), a.now().Sub(layerStartedAt)))
	return finishFunnelResult(result, trace, startedAt, a.now())
}

func (a *AgentRouterApp) applyDeterministicRouteGuard(input *AgentRouteInput, strategy AgentDiagnosisStrategy, cfg AgentRouterConfig) *AgentRouteResult {
	query := strings.ToLower(strings.TrimSpace(input.Query))
	if containsAny(query, cfg.InjectionKeywords) {
		result := confirmRoute("请求包含不安全的指令覆盖内容，无法自动路由")
		result.RiskHint = AgentRouteRiskHigh
		result.Source = "funnel.layer1.safety"
		return result
	}
	if containsAny(query, cfg.HighRiskActionKeywords) {
		return &AgentRouteResult{
			Decision:   AgentRouteDecisionConfirm,
			Confidence: 1,
			Reason:     "检测到可能改变系统状态的操作，需要确认和审批",
			Source:     "funnel.layer1.risk",
			RiskHint:   AgentRouteRiskHigh,
			Candidates: []AgentRouteCandidate{{Intent: AgentRouteIntentActionRequest, Confidence: 1, ReasonCodes: []string{"high_risk_action"}}},
		}
	}
	if cfg.matchesHighConfidenceKeyword(query) {
		selectedStrategy := strategy
		if selectedStrategy == AgentDiagnosisStrategyAuto {
			selectedStrategy = cfg.DefaultDiagnosisStrategy
		}
		if !cfg.isAllowed(selectedStrategy) {
			return confirmRoute("默认诊断策略当前不可用，请选择 Plan 或 GoS")
		}
		return &AgentRouteResult{
			Decision:   AgentRouteDecisionIncident,
			Strategy:   selectedStrategy,
			Confidence: 1,
			Reason:     "命中高精度故障规则，进入诊断路径",
			Source:     "funnel.layer1.rule",
			RiskHint:   AgentRouteRiskLow,
			Candidates: []AgentRouteCandidate{{Intent: AgentRouteIntentIncidentDiagnosis, Confidence: 1, ReasonCodes: []string{"strong_incident_rule"}}},
		}
	}
	return nil
}

func parseFunnelRouteOutput(raw string, topK int) (*flashRouteOutput, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	var output flashRouteOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(trimmed)), &output); err != nil {
		return nil, err
	}
	if len(output.Candidates) == 0 {
		legacyIntent, ok := legacyIntentToInternal(strings.TrimSpace(output.Intent))
		if !ok {
			return nil, fmt.Errorf("candidates are required")
		}
		output.Candidates = []AgentRouteCandidate{{Intent: legacyIntent, Confidence: output.Confidence, ReasonCodes: []string{"legacy_classifier_output"}}}
	}
	for index := range output.Candidates {
		candidate := &output.Candidates[index]
		if !validAgentRouteIntent(candidate.Intent) || candidate.Confidence < 0 || candidate.Confidence > 1 {
			return nil, fmt.Errorf("invalid candidate at index %d", index)
		}
		candidate.ReasonCodes = normalizedReasonCodes(candidate.ReasonCodes)
	}
	sort.SliceStable(output.Candidates, func(i, j int) bool {
		return output.Candidates[i].Confidence > output.Candidates[j].Confidence
	})
	if topK <= 0 {
		topK = 2
	}
	if len(output.Candidates) > topK {
		output.Candidates = output.Candidates[:topK]
	}
	if len(output.Candidates) == 0 {
		return nil, fmt.Errorf("no valid candidates")
	}
	if output.RiskHint == "" {
		output.RiskHint = AgentRouteRiskLow
	}
	if output.RiskHint != AgentRouteRiskLow && output.RiskHint != AgentRouteRiskMedium && output.RiskHint != AgentRouteRiskHigh {
		return nil, fmt.Errorf("invalid risk_hint %q", output.RiskHint)
	}
	output.Entities = normalizedEntities(output.Entities)
	output.RequiredSlots = normalizedReasonCodes(output.RequiredSlots)
	return &output, nil
}

func routeCandidateResult(candidate AgentRouteCandidate, output *flashRouteOutput, mode AgentRouteMode, strategy AgentDiagnosisStrategy, cfg AgentRouterConfig) *AgentRouteResult {
	result := &AgentRouteResult{
		Confidence:   candidate.Confidence,
		Reason:       "意图候选通过置信度与上下文校验",
		Source:       "funnel.layer3",
		Candidates:   append([]AgentRouteCandidate(nil), output.Candidates...),
		Entities:     output.Entities,
		MissingSlots: append([]string(nil), output.RequiredSlots...),
		RiskHint:     output.RiskHint,
	}
	if output.RiskHint == AgentRouteRiskHigh || candidate.Intent == AgentRouteIntentActionRequest {
		result.Decision = AgentRouteDecisionConfirm
		result.Reason = "高风险操作只允许进入确认和审批路径"
		result.RiskHint = AgentRouteRiskHigh
		return result
	}
	if candidate.Intent == AgentRouteIntentOutOfScope {
		result.Decision = AgentRouteDecisionConfirm
		result.Reason = "请求超出当前路由能力范围"
		return result
	}
	runnerUp := 0.0
	for _, other := range output.Candidates {
		if other.Intent != candidate.Intent && other.Confidence > runnerUp {
			runnerUp = other.Confidence
		}
	}
	margin := candidate.Confidence - runnerUp
	if candidate.Confidence < cfg.IntentFunnelAcceptThreshold {
		result.Decision = AgentRouteDecisionConfirm
		result.Reason = "最高候选置信度不足"
		return result
	}
	if len(output.Candidates) > 1 && margin < cfg.IntentFunnelMarginThreshold {
		result.Decision = AgentRouteDecisionConfirm
		result.Reason = "前两个意图候选过于接近"
		return result
	}
	if len(output.RequiredSlots) > 0 {
		result.Decision = AgentRouteDecisionConfirm
		result.Reason = "缺少继续路由所需的信息"
		return result
	}
	if mode == AgentRouteModeDiagnosis {
		result.Decision = AgentRouteDecisionIncident
		result.Strategy = strategy
		if result.Strategy == AgentDiagnosisStrategyAuto {
			result.Strategy = AgentDiagnosisStrategy(output.RecommendedStrategy)
		}
		if !cfg.isAllowed(result.Strategy) {
			result.Decision, result.Strategy, result.Reason = AgentRouteDecisionConfirm, "", "推荐诊断策略当前不可用"
		}
		return result
	}
	switch candidate.Intent {
	case AgentRouteIntentKnowledgeQA, AgentRouteIntentResourceQuery:
		result.Decision = AgentRouteDecisionChat
	case AgentRouteIntentIncidentDiagnosis, AgentRouteIntentIncidentFollowup:
		result.Decision = AgentRouteDecisionIncident
		result.Strategy = strategy
		if result.Strategy == AgentDiagnosisStrategyAuto {
			result.Strategy = AgentDiagnosisStrategy(output.RecommendedStrategy)
		}
		if !cfg.isAllowed(result.Strategy) {
			result.Decision = AgentRouteDecisionConfirm
			result.Strategy = ""
			result.Reason = "推荐诊断策略当前不可用"
		}
	default:
		result.Decision = AgentRouteDecisionConfirm
		result.Reason = "意图无法映射到公开路由"
	}
	return result
}

func (a *AgentRouterApp) loadCandidateCache(cfg AgentRouterConfig, query string) (*flashRouteOutput, bool) {
	if cfg.RouteCacheTTL <= 0 || cfg.RouteCacheMaxEntries <= 0 {
		return nil, false
	}
	key := candidateCacheKey(query, cfg.IntentFunnelPolicyVersion)
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	entry, ok := a.candidates[key]
	if !ok || !entry.expiresAt.After(a.now()) {
		delete(a.candidates, key)
		return nil, false
	}
	output := cloneFlashRouteOutput(entry.output)
	return &output, true
}

func (a *AgentRouterApp) cacheCandidates(cfg AgentRouterConfig, query string, output *flashRouteOutput) {
	if output == nil || cfg.RouteCacheTTL <= 0 || cfg.RouteCacheMaxEntries <= 0 || output.RiskHint == AgentRouteRiskHigh {
		return
	}
	key := candidateCacheKey(query, cfg.IntentFunnelPolicyVersion)
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if _, exists := a.candidates[key]; !exists && len(a.candidates) >= cfg.RouteCacheMaxEntries {
		for cachedKey := range a.candidates {
			delete(a.candidates, cachedKey)
			break
		}
	}
	a.candidates[key] = agentRouteCandidateCacheEntry{output: cloneFlashRouteOutput(*output), expiresAt: a.now().Add(cfg.RouteCacheTTL)}
}

func candidateCacheKey(query, version string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(strings.Join(strings.Fields(query), " "))))
	return version + ":" + hex.EncodeToString(digest[:])
}

func cloneFlashRouteOutput(input flashRouteOutput) flashRouteOutput {
	input.Candidates = append([]AgentRouteCandidate(nil), input.Candidates...)
	input.Entities = normalizedEntities(input.Entities)
	input.RequiredSlots = append([]string(nil), input.RequiredSlots...)
	return input
}

func selectCandidateWithContext(query string, candidates []AgentRouteCandidate, snapshot *RoutingContextSnapshot, ttl time.Duration, now time.Time) (AgentRouteCandidate, bool, string) {
	selected := candidates[0]
	if snapshot == nil {
		return selected, false, ""
	}
	if hasExplicitTopicSwitch(query) {
		return selected, false, "topic_switch"
	}
	if snapshot.StateVersion == "" || snapshot.UpdatedAt.IsZero() {
		return selected, false, "context_version_missing"
	}
	if ttl > 0 && now.Sub(snapshot.UpdatedAt) > ttl {
		return selected, false, "context_expired"
	}
	if len(snapshot.PendingSlots) > 0 && isEllipticalQuery(query) {
		return selected, true, "pending_slot_answer"
	}
	if conflictsWithConfirmedEntities(query, snapshot.ConfirmedEntities) {
		return selected, false, "context_entity_conflict"
	}
	if selected.Intent == AgentRouteIntentIncidentFollowup && snapshot.ActiveIncidentID == "" {
		return selected, false, "active_incident_missing"
	}
	if snapshot.ActiveIncidentID != "" && isEllipticalQuery(query) {
		for _, candidate := range candidates {
			if candidate.Intent == AgentRouteIntentIncidentFollowup {
				return candidate, true, ""
			}
		}
		return selected, false, "context_candidate_missing"
	}
	return selected, false, ""
}

func newClarificationResult(input *AgentRouteInput, output *flashRouteOutput, cfg AgentRouterConfig, reason string, now time.Time) *AgentRouteResult {
	round := 1
	if input.Clarification != nil {
		round = input.Clarification.Round + 1
		if input.Clarification.StateVersion != "" && input.RoutingContext != nil && input.Clarification.StateVersion != input.RoutingContext.StateVersion {
			reason = "clarification_state_changed"
		}
	}
	result := &AgentRouteResult{
		Decision:     AgentRouteDecisionConfirm,
		Confidence:   output.Candidates[0].Confidence,
		Reason:       reason,
		Source:       "funnel.layer3.clarify",
		Candidates:   append([]AgentRouteCandidate(nil), output.Candidates...),
		Entities:     output.Entities,
		MissingSlots: append([]string(nil), output.RequiredSlots...),
		RiskHint:     output.RiskHint,
	}
	if cfg.IntentFunnelMaxClarifications == 0 || round > cfg.IntentFunnelMaxClarifications {
		result.Reason = "自动澄清轮次已用尽，请手动选择执行方式"
		return result
	}
	intents := make([]AgentRouteIntent, 0, len(output.Candidates))
	for _, candidate := range output.Candidates {
		intents = append(intents, candidate.Intent)
	}
	question := "你希望继续知识问答，还是启动故障诊断？"
	if len(output.RequiredSlots) > 0 {
		question = clarificationQuestion(output.RequiredSlots[0])
	}
	stateVersion := ""
	if input.RoutingContext != nil {
		stateVersion = input.RoutingContext.StateVersion
	}
	result.Clarification = &AgentRouteClarification{
		ID:           clarificationID(input.Query, stateVersion, round),
		Question:     question,
		MissingSlots: append([]string(nil), output.RequiredSlots...),
		Candidates:   intents,
		StateVersion: stateVersion,
		Round:        round,
		ExpiresAt:    now.Add(cfg.IntentFunnelContextTTL),
	}
	return result
}

func applyClarificationAnswer(output *flashRouteOutput, answer *AgentRouteClarificationAnswer) {
	if output == nil || answer == nil || strings.TrimSpace(answer.Value) == "" {
		return
	}
	slot := strings.TrimSpace(strings.ToLower(answer.Slot))
	if slot == "" && len(output.RequiredSlots) == 1 {
		slot = output.RequiredSlots[0]
	}
	if slot == "" {
		return
	}
	if normalized := normalizedEntities(map[string]string{slot: answer.Value}); normalized != nil {
		if output.Entities == nil {
			output.Entities = make(map[string]string)
		}
		output.Entities[slot] = normalized[slot]
	}
	remaining := output.RequiredSlots[:0]
	for _, required := range output.RequiredSlots {
		if required != slot {
			remaining = append(remaining, required)
		}
	}
	output.RequiredSlots = remaining
}

func finishFunnelResult(result *AgentRouteResult, trace *AgentRouteTrace, startedAt, finishedAt time.Time) *AgentRouteResult {
	trace.TotalLatencyMS = finishedAt.Sub(startedAt).Milliseconds()
	result.Trace = trace
	return result
}

func routeLayerTrace(layer, outcome string, reasonCodes []string, latency time.Duration) AgentRouteLayerTrace {
	return AgentRouteLayerTrace{Layer: layer, Outcome: outcome, ReasonCodes: normalizedReasonCodes(reasonCodes), LatencyMS: latency.Milliseconds()}
}

func routingContextFingerprint(snapshot *RoutingContextSnapshot) string {
	if snapshot == nil {
		return "none"
	}
	encoded, _ := json.Marshal(struct {
		ActiveRoute         AgentRouteDecision
		ActiveIncidentID    string
		LastConfirmedIntent AgentRouteIntent
		ConfirmedEntities   map[string]string
		PendingSlots        []string
		StateVersion        string
		UpdatedAt           int64
	}{snapshot.ActiveRoute, snapshot.ActiveIncidentID, snapshot.LastConfirmedIntent, snapshot.ConfirmedEntities, snapshot.PendingSlots, snapshot.StateVersion, snapshot.UpdatedAt.Unix()})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:8])
}

func routeQueryHash(query string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(query)))
	return hex.EncodeToString(digest[:])
}

func conflictsWithConfirmedEntities(query string, entities map[string]string) bool {
	service := strings.ToLower(strings.TrimSpace(entities["service"]))
	if service == "" {
		return false
	}
	normalized := strings.ToLower(query)
	if strings.Contains(normalized, service) {
		return false
	}
	return containsAny(normalized, []string{"另一个服务", "其他服务", "换成", "instead"})
}

func clarificationID(query, stateVersion string, round int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", strings.TrimSpace(query), stateVersion, round)))
	return hex.EncodeToString(digest[:8])
}

func clarificationQuestion(slot string) string {
	switch slot {
	case "service":
		return "需要排查哪个服务？"
	case "time_range":
		return "需要查看哪个时间范围？"
	case "task_id":
		return "请提供需要查询的任务 ID。"
	default:
		return "请补充继续处理所需的关键信息。"
	}
}

func legacyIntentToInternal(intent string) (AgentRouteIntent, bool) {
	switch intent {
	case string(AgentRouteDecisionChat):
		return AgentRouteIntentKnowledgeQA, true
	case string(AgentRouteDecisionIncident):
		return AgentRouteIntentIncidentDiagnosis, true
	default:
		return "", false
	}
}

func validAgentRouteIntent(intent AgentRouteIntent) bool {
	switch intent {
	case AgentRouteIntentKnowledgeQA, AgentRouteIntentIncidentDiagnosis, AgentRouteIntentIncidentFollowup,
		AgentRouteIntentResourceQuery, AgentRouteIntentActionRequest, AgentRouteIntentOutOfScope:
		return true
	default:
		return false
	}
}

func normalizedEntities(entities map[string]string) map[string]string {
	if len(entities) == 0 {
		return nil
	}
	allowed := map[string]struct{}{"service": {}, "task_id": {}, "time_range": {}}
	result := make(map[string]string, len(entities))
	for key, value := range entities {
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.TrimSpace(value)
		if _, ok := allowed[key]; ok && value != "" {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizedReasonCodes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func candidateReasonCodes(candidates []AgentRouteCandidate) []string {
	var result []string
	for _, candidate := range candidates {
		result = append(result, candidate.ReasonCodes...)
	}
	return normalizedReasonCodes(result)
}

func containsAny(query string, keywords []string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(query, keyword) {
			return true
		}
	}
	return false
}

func hasExplicitTopicSwitch(query string) bool {
	normalized := strings.ToLower(strings.TrimSpace(query))
	return containsAny(normalized, []string{"换个问题", "另一个问题", "另外问", "new topic", "unrelated"})
}

func isEllipticalQuery(query string) bool {
	normalized := strings.TrimSpace(query)
	if utf8.RuneCountInString(normalized) <= 12 {
		return true
	}
	return containsAny(strings.ToLower(normalized), []string{"那它", "那这个", "继续看", "然后呢", "what about"})
}
