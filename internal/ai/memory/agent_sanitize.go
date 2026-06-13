package memory

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

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