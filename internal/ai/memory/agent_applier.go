package memory

import (
	"context"
	"strings"
)

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