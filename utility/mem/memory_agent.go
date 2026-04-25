package mem

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type MemoryEvent struct {
	SessionID        string
	UserID           string
	ProjectID        string
	Query            string
	Answer           string
	TraceID          string
	ExistingMemories []*MemoryEntry
}

type MemoryActionOp string

const (
	MemoryActionSkip      MemoryActionOp = "skip"
	MemoryActionUpsert    MemoryActionOp = "upsert"
	MemoryActionSupersede MemoryActionOp = "supersede"
	MemoryActionPromote   MemoryActionOp = "promote"
)

type MemoryDecision struct {
	Actions []MemoryAction `json:"actions"`
}

type MemoryAction struct {
	Op            MemoryActionOp `json:"op"`
	TargetID      string         `json:"target_id,omitempty"`
	Type          MemoryType     `json:"type,omitempty"`
	Content       string         `json:"content,omitempty"`
	Scope         MemoryScope    `json:"scope,omitempty"`
	ScopeID       string         `json:"scope_id,omitempty"`
	Confidence    float64        `json:"confidence,omitempty"`
	ConflictGroup string         `json:"conflict_group,omitempty"`
	ExpiresAt     int64          `json:"expires_at,omitempty"`
	Reason        string         `json:"reason,omitempty"`
}

type MemoryAuditRecord struct {
	ID        string         `json:"id"`
	MemoryID  string         `json:"memory_id,omitempty"`
	Op        MemoryActionOp `json:"op"`
	Reason    string         `json:"reason,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	Actor     string         `json:"actor"`
	Before    *MemoryEntry   `json:"before,omitempty"`
	After     *MemoryEntry   `json:"after,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

type MemoryAgent interface {
	Decide(ctx context.Context, event MemoryEvent) (*MemoryDecision, error)
}

type MemoryChatModel interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

type RuleMemoryAgent struct{}

func NewRuleMemoryAgent() *RuleMemoryAgent {
	return &RuleMemoryAgent{}
}

func (a *RuleMemoryAgent) Decide(ctx context.Context, event MemoryEvent) (*MemoryDecision, error) {
	if ctx != nil && ctx.Err() != nil {
		return &MemoryDecision{}, ctx.Err()
	}
	actions := make([]MemoryAction, 0)
	for _, candidate := range ExtractMemoryCandidates(event.Query, event.Answer) {
		action := MemoryAction{
			Op:            MemoryActionUpsert,
			Type:          candidate.Type,
			Content:       candidate.Content,
			Scope:         candidate.Scope,
			ScopeID:       candidate.ScopeID,
			Confidence:    candidate.Confidence,
			ConflictGroup: inferMemoryConflictGroup(candidate),
			Reason:        "rule extractor produced a durable memory candidate",
		}
		if candidate.Type == MemoryTypePreference && strings.TrimSpace(event.UserID) != "" {
			action.Scope = MemoryScopeUser
			action.ScopeID = strings.TrimSpace(event.UserID)
		}
		if strings.TrimSpace(action.ScopeID) == "" {
			action.ScopeID = defaultActionScopeID(event, action.Scope)
		}
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		actions = append(actions, MemoryAction{
			Op:     MemoryActionSkip,
			Reason: "no durable memory candidate found",
		})
	}
	return &MemoryDecision{Actions: actions}, nil
}

type LLMMemoryAgent struct {
	chat     MemoryChatModel
	fallback MemoryAgent
}

func NewLLMMemoryAgent(chat MemoryChatModel, fallback MemoryAgent) *LLMMemoryAgent {
	if fallback == nil {
		fallback = NewRuleMemoryAgent()
	}
	return &LLMMemoryAgent{
		chat:     chat,
		fallback: fallback,
	}
}

func (a *LLMMemoryAgent) Decide(ctx context.Context, event MemoryEvent) (*MemoryDecision, error) {
	if a == nil || a.chat == nil {
		return a.fallbackDecision(ctx, event)
	}
	if ctx != nil && ctx.Err() != nil {
		return &MemoryDecision{}, ctx.Err()
	}
	event = withExistingMemories(normalizeMemoryEvent(event))
	resp, err := a.chat.Generate(ctx, []*schema.Message{
		schema.SystemMessage(memoryAgentSystemPrompt()),
		schema.UserMessage(memoryAgentUserPrompt(event)),
	})
	if err != nil {
		return a.fallbackDecision(memoryFallbackContext(ctx), event)
	}
	if resp == nil {
		return a.fallbackDecision(memoryFallbackContext(ctx), event)
	}
	decision, err := parseMemoryDecisionJSON(resp.Content)
	if err != nil {
		return a.fallbackDecision(memoryFallbackContext(ctx), event)
	}
	return sanitizeMemoryDecision(decision, event), nil
}

func (a *LLMMemoryAgent) fallbackDecision(ctx context.Context, event MemoryEvent) (*MemoryDecision, error) {
	if a != nil && a.fallback != nil {
		return a.fallback.Decide(ctx, event)
	}
	return NewRuleMemoryAgent().Decide(ctx, event)
}

func memoryFallbackContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if ctx.Err() != nil {
		return context.WithoutCancel(ctx)
	}
	return ctx
}

type MemoryApplier struct {
	ltm *LongTermMemory
}

func NewMemoryApplier(ltm *LongTermMemory) *MemoryApplier {
	if ltm == nil {
		ltm = GetLongTermMemory()
	}
	return &MemoryApplier{ltm: ltm}
}

func ProcessMemoryEventWithReport(ctx context.Context, event MemoryEvent) *MemoryExtractionReport {
	return ProcessMemoryEventWithAgent(ctx, event, NewRuleMemoryAgent())
}

func ProcessMemoryEventWithAgent(ctx context.Context, event MemoryEvent, agent MemoryAgent) *MemoryExtractionReport {
	if ctx != nil && ctx.Err() != nil {
		return &MemoryExtractionReport{}
	}
	event = normalizeMemoryEvent(event)
	if agent == nil {
		agent = NewRuleMemoryAgent()
	}
	event = withExistingMemories(event)
	decision, err := agent.Decide(ctx, event)
	if err != nil {
		return &MemoryExtractionReport{
			Dropped: []DroppedMemoryCandidate{{Reason: err.Error()}},
		}
	}
	applyCtx := ctx
	if applyCtx != nil && applyCtx.Err() != nil {
		applyCtx = context.WithoutCancel(applyCtx)
	}
	return NewMemoryApplier(GetLongTermMemory()).Apply(applyCtx, event, decision)
}

func (a *MemoryApplier) Apply(ctx context.Context, event MemoryEvent, decision *MemoryDecision) *MemoryExtractionReport {
	report := &MemoryExtractionReport{}
	if decision == nil {
		return report
	}
	event = normalizeMemoryEvent(event)
	for _, action := range decision.Actions {
		if ctx != nil && ctx.Err() != nil {
			return report
		}
		report.Actions = append(report.Actions, action)
		switch action.Op {
		case MemoryActionSkip:
			report.AuditRecords = append(report.AuditRecords, newMemoryAuditRecord("", action, event, nil, nil))
		case MemoryActionUpsert:
			a.applyUpsert(ctx, event, action, report)
		case MemoryActionSupersede:
			a.applySupersede(ctx, event, action, report)
		case MemoryActionPromote:
			a.applyPromote(ctx, event, action, report)
		default:
			report.Dropped = append(report.Dropped, DroppedMemoryCandidate{Reason: "unsupported_action"})
		}
	}
	return report
}

func (a *MemoryApplier) applyUpsert(ctx context.Context, event MemoryEvent, action MemoryAction, report *MemoryExtractionReport) {
	candidate := candidateFromAction(action)
	report.Candidates = append(report.Candidates, candidate)
	if ok, reason := ValidateMemoryCandidate(candidate); !ok {
		report.Dropped = append(report.Dropped, DroppedMemoryCandidate{Candidate: candidate, Reason: reason})
		return
	}
	scope := action.Scope
	if scope == "" {
		scope = candidate.Scope
	}
	scopeID := action.ScopeID
	if strings.TrimSpace(scopeID) == "" {
		scopeID = defaultActionScopeID(event, scope)
	}
	id := a.ltm.StoreWithOptions(ctx, event.SessionID, candidate.Type, candidate.Content, candidate.Source, MemoryStoreOptions{
		Scope:         scope,
		ScopeID:       scopeID,
		Confidence:    action.Confidence,
		SafetyLabel:   candidate.SafetyLabel,
		Provenance:    candidate.Provenance,
		ConflictGroup: strings.TrimSpace(action.ConflictGroup),
		ExpiresAt:     action.ExpiresAt,
	})
	report.StoredIDs = append(report.StoredIDs, id)
	after := a.ltm.Get(id)
	report.AuditRecords = append(report.AuditRecords, newMemoryAuditRecord(id, action, event, nil, after))
}

func (a *MemoryApplier) applySupersede(ctx context.Context, event MemoryEvent, action MemoryAction, report *MemoryExtractionReport) {
	candidate := candidateFromAction(action)
	if ok, reason := ValidateMemoryCandidate(candidate); !ok {
		report.Candidates = append(report.Candidates, candidate)
		report.Dropped = append(report.Dropped, DroppedMemoryCandidate{Candidate: candidate, Reason: reason})
		return
	}
	var before *MemoryEntry
	if strings.TrimSpace(action.TargetID) != "" {
		before = a.ltm.Get(action.TargetID)
		_ = a.ltm.Disable(ctx, action.TargetID)
	}
	a.applyUpsert(ctx, event, action, report)
	if strings.TrimSpace(action.TargetID) != "" {
		report.AuditRecords = append(report.AuditRecords, newMemoryAuditRecord(action.TargetID, action, event, before, a.ltm.Get(action.TargetID)))
	}
}

func (a *MemoryApplier) applyPromote(ctx context.Context, event MemoryEvent, action MemoryAction, report *MemoryExtractionReport) {
	if strings.TrimSpace(action.TargetID) == "" {
		report.Dropped = append(report.Dropped, DroppedMemoryCandidate{Reason: "missing_target"})
		return
	}
	before := a.ltm.Get(action.TargetID)
	scope := action.Scope
	if scope == "" {
		scope = MemoryScopeProject
	}
	scopeID := action.ScopeID
	if strings.TrimSpace(scopeID) == "" {
		scopeID = defaultActionScopeID(event, scope)
	}
	if !a.ltm.Promote(ctx, action.TargetID, scope, scopeID, action.Confidence) {
		report.Dropped = append(report.Dropped, DroppedMemoryCandidate{Reason: "target_not_found"})
		return
	}
	report.AuditRecords = append(report.AuditRecords, newMemoryAuditRecord(action.TargetID, action, event, before, a.ltm.Get(action.TargetID)))
}

func candidateFromAction(action MemoryAction) MemoryCandidate {
	return MemoryCandidate{
		Type:        action.Type,
		Content:     action.Content,
		Source:      "memory_agent",
		Scope:       action.Scope,
		ScopeID:     action.ScopeID,
		Confidence:  action.Confidence,
		SafetyLabel: "internal",
		Provenance:  "memory_agent",
	}
}

func normalizeMemoryEvent(event MemoryEvent) MemoryEvent {
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.UserID = strings.TrimSpace(event.UserID)
	event.ProjectID = strings.TrimSpace(event.ProjectID)
	event.Query = strings.TrimSpace(event.Query)
	event.Answer = strings.TrimSpace(event.Answer)
	event.TraceID = strings.TrimSpace(event.TraceID)
	return event
}

func withExistingMemories(event MemoryEvent) MemoryEvent {
	if len(event.ExistingMemories) > 0 {
		return event
	}
	refs := make([]MemoryScopeRef, 0, 4)
	if event.SessionID != "" {
		refs = append(refs, MemoryScopeRef{Scope: MemoryScopeSession, ScopeID: event.SessionID})
	}
	if event.UserID != "" {
		refs = append(refs, MemoryScopeRef{Scope: MemoryScopeUser, ScopeID: event.UserID})
	}
	if event.ProjectID != "" {
		refs = append(refs, MemoryScopeRef{Scope: MemoryScopeProject, ScopeID: event.ProjectID})
	}
	refs = append(refs, MemoryScopeRef{Scope: MemoryScopeGlobal, ScopeID: "global"})
	items := GetLongTermMemory().List(refs, false)
	if len(items) > 20 {
		items = items[:20]
	}
	event.ExistingMemories = items
	return event
}

func defaultActionScopeID(event MemoryEvent, scope MemoryScope) string {
	switch scope {
	case MemoryScopeUser:
		if event.UserID != "" {
			return event.UserID
		}
	case MemoryScopeProject:
		if event.ProjectID != "" {
			return event.ProjectID
		}
	case MemoryScopeGlobal:
		return "global"
	}
	return event.SessionID
}

func inferMemoryConflictGroup(candidate MemoryCandidate) string {
	content := strings.ToLower(candidate.Content)
	switch candidate.Type {
	case MemoryTypePreference:
		return "preference:" + firstMatchedMarker(content, []string{"中文", "简洁", "详细", "先给结论", "不要用"})
	case MemoryTypeFact:
		return "fact:" + firstMatchedMarker(content, []string{"服务名", "应用名", "ip地址", "端口", "数据库名", "集群名", "版本号", "负责人", "告警规则", "阈值", "sla", "域名"})
	default:
		return ""
	}
}

func firstMatchedMarker(content string, markers []string) string {
	for _, marker := range markers {
		if strings.Contains(content, strings.ToLower(marker)) {
			return marker
		}
	}
	return "general"
}

func memoryAgentSystemPrompt() string {
	return strings.Join([]string{
		"你是 OpsCaption 的 Memory Agent，只负责决定哪些对话内容应该进入长期记忆。",
		"你必须只输出 JSON，不要输出 Markdown、解释或多余文本。",
		"输出格式：{\"actions\":[{\"op\":\"skip|upsert|supersede|promote\",\"target_id\":\"\",\"type\":\"fact|preference|procedure|episode\",\"content\":\"\",\"scope\":\"session|user|project|global\",\"scope_id\":\"\",\"confidence\":0.0,\"conflict_group\":\"\",\"expires_at\":0,\"reason\":\"\"}]}",
		"只保存长期稳定、有复用价值的信息：用户偏好、项目约定、服务事实、排障流程、被明确纠正的新事实。",
		"不要保存临时闲聊、模型套话、代码块、密钥、token、password、authorization、一次性中间推理。",
		"用户个人偏好用 user scope；当前会话事实用 session scope；项目约定和排障流程用 project scope；global scope 只用于明确跨项目通用的稳定规则。",
		"如果新事实纠正了已有记忆，用 supersede 并填写 target_id；如果已有记忆应提升范围，用 promote；没有可保存内容就只返回 skip。",
		"content 必须简短、可直接复用，confidence 必须在 0 到 1 之间。",
	}, "\n")
}

func memoryAgentUserPrompt(event MemoryEvent) string {
	payload := struct {
		SessionID string               `json:"session_id"`
		UserID    string               `json:"user_id,omitempty"`
		ProjectID string               `json:"project_id,omitempty"`
		Query     string               `json:"query"`
		Answer    string               `json:"answer"`
		Existing  []memoryPromptMemory `json:"existing_memories,omitempty"`
	}{
		SessionID: event.SessionID,
		UserID:    event.UserID,
		ProjectID: event.ProjectID,
		Query:     event.Query,
		Answer:    event.Answer,
		Existing:  memoryPromptMemories(event.ExistingMemories),
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf("session_id=%s\nquery=%s\nanswer=%s", event.SessionID, event.Query, event.Answer)
	}
	return string(body)
}

type memoryPromptMemory struct {
	ID            string      `json:"id"`
	Type          MemoryType  `json:"type"`
	Content       string      `json:"content"`
	Scope         MemoryScope `json:"scope"`
	ScopeID       string      `json:"scope_id,omitempty"`
	Confidence    float64     `json:"confidence"`
	ConflictGroup string      `json:"conflict_group,omitempty"`
}

func memoryPromptMemories(entries []*MemoryEntry) []memoryPromptMemory {
	if len(entries) == 0 {
		return nil
	}
	result := make([]memoryPromptMemory, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		result = append(result, memoryPromptMemory{
			ID:            entry.ID,
			Type:          entry.Type,
			Content:       entry.Content,
			Scope:         entry.Scope,
			ScopeID:       entry.ScopeID,
			Confidence:    entry.Confidence,
			ConflictGroup: entry.ConflictGroup,
		})
	}
	return result
}

func parseMemoryDecisionJSON(content string) (*MemoryDecision, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("empty memory agent response")
	}
	trimmed = stripJSONFence(trimmed)
	if start, end := strings.Index(trimmed, "{"), strings.LastIndex(trimmed, "}"); start >= 0 && end > start {
		body := trimmed[start : end+1]
		var decision MemoryDecision
		if err := json.Unmarshal([]byte(body), &decision); err == nil {
			if len(decision.Actions) > 0 {
				return &decision, nil
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(body), &raw); err == nil {
				if _, ok := raw["actions"]; ok {
					return &decision, nil
				}
			}
			var action MemoryAction
			if err := json.Unmarshal([]byte(body), &action); err == nil && action.Op != "" {
				return &MemoryDecision{Actions: []MemoryAction{action}}, nil
			}
		}
	}
	if start, end := strings.Index(trimmed, "["), strings.LastIndex(trimmed, "]"); start >= 0 && end > start {
		body := trimmed[start : end+1]
		var actions []MemoryAction
		if err := json.Unmarshal([]byte(body), &actions); err == nil {
			return &MemoryDecision{Actions: actions}, nil
		}
	}
	return nil, fmt.Errorf("invalid memory agent json")
}

func stripJSONFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= 2 {
		return trimmed
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

func sanitizeMemoryDecision(decision *MemoryDecision, event MemoryEvent) *MemoryDecision {
	if decision == nil {
		return &MemoryDecision{Actions: []MemoryAction{{Op: MemoryActionSkip, Reason: "empty llm memory decision"}}}
	}
	actions := make([]MemoryAction, 0, len(decision.Actions))
	for _, action := range decision.Actions {
		action = sanitizeMemoryAction(action, event)
		if action.Op == "" {
			continue
		}
		if action.Op == MemoryActionSkip && len(decision.Actions) > 1 {
			continue
		}
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		actions = append(actions, MemoryAction{Op: MemoryActionSkip, Reason: "no valid llm memory action"})
	}
	return &MemoryDecision{Actions: actions}
}

func sanitizeMemoryAction(action MemoryAction, event MemoryEvent) MemoryAction {
	action.Op = normalizeMemoryActionOp(action.Op)
	action.TargetID = strings.TrimSpace(action.TargetID)
	action.Content = strings.TrimSpace(action.Content)
	action.ScopeID = strings.TrimSpace(action.ScopeID)
	action.ConflictGroup = strings.TrimSpace(action.ConflictGroup)
	action.Reason = strings.TrimSpace(action.Reason)
	if action.Reason == "" {
		action.Reason = "llm memory decision"
	}
	if action.Op == MemoryActionSkip {
		return MemoryAction{Op: MemoryActionSkip, Reason: action.Reason}
	}
	action.Type = normalizeMemoryType(action.Type)
	action.Scope = normalizeMemoryScope(action.Scope, action.Op)
	if action.ScopeID == "" {
		action.ScopeID = defaultActionScopeID(event, action.Scope)
	}
	if action.Confidence <= 0 {
		action.Confidence = defaultMemoryConfidence(action.Type)
	}
	if action.Confidence > 1 {
		action.Confidence = 1
	}
	if action.Op == MemoryActionSupersede && action.TargetID == "" {
		action.Op = MemoryActionUpsert
	}
	if action.ConflictGroup == "" && (action.Op == MemoryActionUpsert || action.Op == MemoryActionSupersede) {
		action.ConflictGroup = inferMemoryConflictGroup(MemoryCandidate{
			Type:    action.Type,
			Content: action.Content,
		})
	}
	return action
}

func normalizeMemoryActionOp(op MemoryActionOp) MemoryActionOp {
	switch op {
	case MemoryActionSkip, MemoryActionUpsert, MemoryActionSupersede, MemoryActionPromote:
		return op
	default:
		return ""
	}
}

func normalizeMemoryType(memType MemoryType) MemoryType {
	switch memType {
	case MemoryTypeFact, MemoryTypePreference, MemoryTypeProcedure, MemoryTypeEpisode:
		return memType
	default:
		return MemoryTypeFact
	}
}

func normalizeMemoryScope(scope MemoryScope, op MemoryActionOp) MemoryScope {
	switch scope {
	case MemoryScopeSession, MemoryScopeUser, MemoryScopeProject, MemoryScopeGlobal:
		return scope
	default:
		if op == MemoryActionPromote {
			return MemoryScopeProject
		}
		return MemoryScopeSession
	}
}

func newMemoryAuditRecord(memoryID string, action MemoryAction, event MemoryEvent, before, after *MemoryEntry) MemoryAuditRecord {
	now := time.Now().UnixMilli()
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%d", memoryID, action.Op, action.Reason, now)))
	return MemoryAuditRecord{
		ID:        fmt.Sprintf("%x", sum[:8]),
		MemoryID:  memoryID,
		Op:        action.Op,
		Reason:    action.Reason,
		TraceID:   event.TraceID,
		Actor:     "memory_agent",
		Before:    before,
		After:     after,
		CreatedAt: now,
	}
}
