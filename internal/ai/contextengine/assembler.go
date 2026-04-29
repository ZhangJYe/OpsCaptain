package contextengine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"SuperBizAgent/utility/mem"

	"github.com/cloudwego/eino/schema"
)

type Assembler struct {
	resolver *PolicyResolver
	now      func() time.Time
}

func NewAssembler() *Assembler {
	return &Assembler{
		resolver: NewPolicyResolver(),
		now:      time.Now,
	}
}

func (a *Assembler) Assemble(ctx context.Context, req ContextRequest, history []*schema.Message) (*ContextPackage, error) {
	start := a.now()
	profile := a.resolver.Resolve(ctx, req)
	pkg := &ContextPackage{
		Request: req,
		Profile: profile,
		Query:   strings.TrimSpace(req.Query),
	}
	trace := ContextAssemblyTrace{
		Profile: profile.Name,
		BudgetBefore: BudgetSnapshot{
			HistoryTokens:  profile.Budget.HistoryTokens,
			MemoryTokens:   profile.Budget.MemoryTokens,
			DocumentTokens: profile.Budget.DocumentTokens,
			ToolTokens:     profile.Budget.ToolTokens,
		},
	}

	if profile.AllowHistory && len(history) > 0 {
		selectedHistory, droppedHistory, usedHistory, historyNotes := selectHistory(history, profile)
		pkg.HistoryMessages = selectedHistory
		trace.SourcesConsidered += len(history)
		trace.SourcesSelected += len(selectedHistory)
		trace.DroppedItems = append(trace.DroppedItems, droppedHistory...)
		trace.BudgetAfter.HistoryTokens = usedHistory
		trace.Stages = append(trace.Stages, StageTrace{
			Name:          "history",
			SelectedCount: len(selectedHistory),
			DroppedCount:  len(droppedHistory),
			Notes:         historyNotes,
		})
	}

	if profile.AllowMemory && req.SessionID != "" {
		retrieveLimit := profile.MaxMemoryItems
		if retrieveLimit < 1 {
			retrieveLimit = 1
		}
		entries := mem.GetLongTermMemory().RetrieveScoped(ctx, req.Query, retrieveLimit*3, mem.MemoryRetrievePolicy{
			ReadOnly:  true,
			ScopeRefs: memoryScopeRefs(req),
		})
		selectedMemory, droppedMemory, usedMemory, memoryNotes := selectMemories(entries, profile, a.now())
		pkg.MemoryItems = selectedMemory
		trace.SourcesConsidered += len(entries)
		trace.SourcesSelected += len(selectedMemory)
		trace.DroppedItems = append(trace.DroppedItems, droppedMemory...)
		trace.BudgetAfter.MemoryTokens = usedMemory
		trace.Stages = append(trace.Stages, StageTrace{
			Name:          "memory",
			SelectedCount: len(selectedMemory),
			DroppedCount:  len(droppedMemory),
			Notes:         memoryNotes,
		})
	}

	if profile.AllowDocs {
		docResult := selectDocuments(ctx, req.Query, profile)
		pkg.DocumentItems = docResult.selected
		trace.SourcesConsidered += len(docResult.selected) + len(docResult.dropped)
		trace.SourcesSelected += len(docResult.selected)
		trace.DroppedItems = append(trace.DroppedItems, docResult.dropped...)
		trace.BudgetAfter.DocumentTokens = docResult.used
		trace.Stages = append(trace.Stages, StageTrace{
			Name:          "documents",
			SelectedCount: len(docResult.selected),
			DroppedCount:  len(docResult.dropped),
			Notes:         docResult.notes,
			Retrieval:     docResult.metrics,
		})
	}

	if profile.AllowToolResults && len(req.ToolItems) > 0 {
		selectedTools, droppedTools, usedTools, toolNotes := selectToolItems(req.ToolItems, profile)
		pkg.ToolItems = selectedTools
		trace.SourcesConsidered += len(selectedTools) + len(droppedTools)
		trace.SourcesSelected += len(selectedTools)
		trace.DroppedItems = append(trace.DroppedItems, droppedTools...)
		trace.BudgetAfter.ToolTokens = usedTools
		trace.Stages = append(trace.Stages, StageTrace{
			Name:          "tool_results",
			SelectedCount: len(selectedTools),
			DroppedCount:  len(droppedTools),
			Notes:         toolNotes,
		})
	}

	if profile.Staged && len(pkg.MemoryItems) > 0 {
		pkg.HistoryMessages = append(memoryItemsAsMessages(pkg.MemoryItems), pkg.HistoryMessages...)
	}

	trace.LatencyMs = a.now().Sub(start).Milliseconds()
	pkg.Trace = trace
	return pkg, nil
}

func MemoryContext(pkg *ContextPackage) string {
	if pkg == nil || len(pkg.MemoryItems) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pkg.MemoryItems))
	for _, item := range pkg.MemoryItems {
		parts = append(parts, fmt.Sprintf("- [%s] %s", item.Title, item.Content))
	}
	return strings.Join(parts, "\n")
}

func memoryScopeRefs(req ContextRequest) []mem.MemoryScopeRef {
	refs := []mem.MemoryScopeRef{
		{Scope: mem.MemoryScopeSession, ScopeID: req.SessionID},
		{Scope: mem.MemoryScopeGlobal, ScopeID: "global"},
	}
	if strings.TrimSpace(req.UserID) != "" {
		refs = append(refs, mem.MemoryScopeRef{Scope: mem.MemoryScopeUser, ScopeID: strings.TrimSpace(req.UserID)})
	}
	if strings.TrimSpace(req.ProjectID) != "" {
		refs = append(refs, mem.MemoryScopeRef{Scope: mem.MemoryScopeProject, ScopeID: strings.TrimSpace(req.ProjectID)})
	}
	return refs
}

func selectToolItems(items []ContextItem, profile ContextProfile) ([]ContextItem, []ContextItem, int, []string) {
	if len(items) == 0 || profile.MaxToolItems == 0 || profile.Budget.ToolTokens == 0 {
		return nil, nil, 0, []string{"tool results empty or disabled"}
	}

	remaining := profile.Budget.ToolTokens
	selected := make([]ContextItem, 0, min(len(items), profile.MaxToolItems))
	dropped := make([]ContextItem, 0)
	used := 0
	for idx, item := range items {
		item.TokenEstimate = mem.EstimateTokens(item.Content)
		if idx >= profile.MaxToolItems {
			item.DroppedReason = "tool_window"
			dropped = append(dropped, item)
			continue
		}
		if item.TokenEstimate > remaining {
			trimmed := mem.TrimToTokenBudget(item.Content, remaining)
			if strings.TrimSpace(trimmed) == "" {
				item.DroppedReason = "tool_budget"
				dropped = append(dropped, item)
				continue
			}
			item.Content = trimmed
			item.TokenEstimate = mem.EstimateTokens(trimmed)
			item.CompressionLevel = "trimmed"
		}
		item.Selected = true
		selected = append(selected, item)
		remaining -= item.TokenEstimate
		used += item.TokenEstimate
		if remaining <= 0 {
			for j := idx + 1; j < len(items); j++ {
				items[j].DroppedReason = "tool_budget"
				dropped = append(dropped, items[j])
			}
			break
		}
	}
	return selected, dropped, used, []string{fmt.Sprintf("tokens=%d/%d", used, profile.Budget.ToolTokens)}
}

func TraceDetails(trace ContextAssemblyTrace) []string {
	details := []string{
		fmt.Sprintf("context profile=%s", trace.Profile),
		fmt.Sprintf("context sources selected=%d/%d", trace.SourcesSelected, trace.SourcesConsidered),
	}
	for _, stage := range trace.Stages {
		if stage.Name == "" {
			continue
		}
		line := fmt.Sprintf("%s selected=%d dropped=%d", stage.Name, stage.SelectedCount, stage.DroppedCount)
		if len(stage.Notes) > 0 {
			line += " (" + strings.Join(stage.Notes, "; ") + ")"
		}
		if stage.Retrieval != nil {
			line += fmt.Sprintf(
				" [cache_hit=%t init_cached_error=%t init_ms=%d retrieve_ms=%d hits=%d]",
				stage.Retrieval.CacheHit,
				stage.Retrieval.InitFailureCached,
				stage.Retrieval.InitLatencyMs,
				stage.Retrieval.RetrieveLatencyMs,
				stage.Retrieval.ResultCount,
			)
		}
		details = append(details, line)
	}
	if len(trace.DroppedItems) > 0 {
		reasonCounts := make(map[string]int)
		for _, item := range trace.DroppedItems {
			reason := item.DroppedReason
			if reason == "" {
				reason = "unspecified"
			}
			reasonCounts[reason]++
		}
		reasons := make([]string, 0, len(reasonCounts))
		for reason, count := range reasonCounts {
			reasons = append(reasons, fmt.Sprintf("%s=%d", reason, count))
		}
		sort.Strings(reasons)
		details = append(details, "context dropped "+strings.Join(reasons, ", "))
	}
	return details
}

func selectHistory(history []*schema.Message, profile ContextProfile) ([]*schema.Message, []ContextItem, int, []string) {
	if len(history) == 0 || profile.MaxHistoryMessages == 0 || profile.Budget.HistoryTokens == 0 {
		return nil, nil, 0, []string{"history disabled"}
	}

	maxMessages := profile.MaxHistoryMessages
	remaining := profile.Budget.HistoryTokens
	selectedIdx := make(map[int]bool)
	selected := make([]*schema.Message, 0, min(len(history), maxMessages))
	dropped := make([]ContextItem, 0)
	used := 0
	selectedCount := 0

	for i := len(history) - 1; i >= 0; i-- {
		if selectedCount >= maxMessages {
			dropped = append(dropped, newDroppedHistoryItem(i, history[i], "history_window"))
			continue
		}
		tokens := mem.EstimateTokens(history[i].Content)
		if tokens > remaining {
			dropped = append(dropped, newDroppedHistoryItem(i, history[i], "history_budget"))
			continue
		}
		selectedIdx[i] = true
		remaining -= tokens
		used += tokens
		selectedCount++
	}

	if len(history) > 0 && hasSummaryPrefix(history[0]) && !selectedIdx[0] {
		summaryTokens := mem.EstimateTokens(history[0].Content)
		if summaryTokens <= remaining {
			selectedIdx[0] = true
			remaining -= summaryTokens
			used += summaryTokens
		}
		if len(history) > 1 && !selectedIdx[1] {
			replyTokens := mem.EstimateTokens(history[1].Content)
			if replyTokens <= remaining {
				selectedIdx[1] = true
				remaining -= replyTokens
				used += replyTokens
			}
		}
	}

	for idx, msg := range history {
		if selectedIdx[idx] {
			selected = append(selected, msg)
		} else if !containsDroppedHistory(dropped, idx) {
			dropped = append(dropped, newDroppedHistoryItem(idx, msg, "history_window"))
		}
	}

	notes := []string{fmt.Sprintf("tokens=%d/%d", used, profile.Budget.HistoryTokens)}
	return selected, dropped, used, notes
}

func selectMemories(entries []*mem.MemoryEntry, profile ContextProfile, now time.Time) ([]ContextItem, []ContextItem, int, []string) {
	if len(entries) == 0 || profile.MaxMemoryItems == 0 || profile.Budget.MemoryTokens == 0 {
		return nil, nil, 0, []string{"memory disabled or empty"}
	}

	remaining := profile.Budget.MemoryTokens
	selected := make([]ContextItem, 0, min(len(entries), profile.MaxMemoryItems))
	dropped := make([]ContextItem, 0)
	used := 0
	selectedCount := 0
	for _, entry := range entries {
		item := newMemoryItem(entry)
		if memoryItemExpired(item, now) {
			item.DroppedReason = "memory_expired"
			dropped = append(dropped, item)
			continue
		}
		if !memoryScopeAllowed(item.Scope, profile.AllowedMemoryScopes) {
			item.DroppedReason = "memory_scope"
			dropped = append(dropped, item)
			continue
		}
		if item.Confidence < profile.MinMemoryConfidence {
			item.DroppedReason = "memory_confidence"
			dropped = append(dropped, item)
			continue
		}
		if !memorySafetyAllowed(item.SafetyLabel) {
			item.DroppedReason = "memory_safety"
			dropped = append(dropped, item)
			continue
		}
		if selectedCount >= profile.MaxMemoryItems {
			item.DroppedReason = "memory_window"
			dropped = append(dropped, item)
			continue
		}
		if item.TokenEstimate > remaining {
			item.DroppedReason = "memory_budget"
			dropped = append(dropped, item)
			continue
		}
		item.Selected = true
		selected = append(selected, item)
		selectedCount++
		remaining -= item.TokenEstimate
		used += item.TokenEstimate
	}

	notes := []string{
		fmt.Sprintf("tokens=%d/%d", used, profile.Budget.MemoryTokens),
		fmt.Sprintf("min_confidence=%.2f", profile.MinMemoryConfidence),
	}
	return selected, dropped, used, notes
}

func newMemoryItem(entry *mem.MemoryEntry) ContextItem {
	freshness := 1.0
	hours := time.Since(entry.LastUsed).Hours()
	freshness = 1.0 / (1.0 + hours/24.0)
	return ContextItem{
		ID:             entry.ID,
		SourceType:     "memory",
		SourceID:       entry.SessionID,
		Title:          string(entry.Type),
		Content:        entry.Content,
		Score:          entry.Relevance,
		TrustLevel:     memoryTrustLevel(entry),
		TokenEstimate:  mem.EstimateTokens(entry.Content),
		Timestamp:      entry.UpdatedAt.UnixMilli(),
		FreshnessScore: freshness,
		SafetyLabel:    entry.SafetyLabel,
		UpdatePolicy:   entry.UpdatePolicy,
		Scope:          string(entry.Scope),
		Confidence:     entry.Confidence,
		Provenance:     entry.Provenance,
		ExpiresAt:      entry.ExpiresAt,
	}
}

func memoryItemExpired(item ContextItem, now time.Time) bool {
	return item.ExpiresAt > 0 && item.ExpiresAt <= now.UnixMilli()
}

func memoryScopeAllowed(scope string, allowed []string) bool {
	if scope == "" {
		scope = string(mem.MemoryScopeSession)
	}
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if candidate == scope {
			return true
		}
	}
	return false
}

func memorySafetyAllowed(label string) bool {
	switch strings.TrimSpace(strings.ToLower(label)) {
	case "", "internal", "trusted_internal", "safe":
		return true
	default:
		return false
	}
}

func memoryTrustLevel(entry *mem.MemoryEntry) string {
	if entry.SafetyLabel != "" {
		return entry.SafetyLabel
	}
	return "internal"
}

func memoryItemsAsMessages(items []ContextItem) []*schema.Message {
	if len(items) == 0 {
		return nil
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("- [%s] %s", item.Title, item.Content))
	}
	return []*schema.Message{
		{
			Role:    schema.User,
			Content: "[关键记忆]\n" + strings.Join(parts, "\n"),
		},
		schema.AssistantMessage("好的，我已了解这些背景信息。", nil),
	}
}

func newDroppedHistoryItem(idx int, msg *schema.Message, reason string) ContextItem {
	content := ""
	if msg != nil {
		content = msg.Content
	}
	return ContextItem{
		ID:            fmt.Sprintf("history-%d", idx),
		SourceType:    "history",
		SourceID:      fmt.Sprintf("%d", idx),
		Content:       content,
		TokenEstimate: mem.EstimateTokens(content),
		DroppedReason: reason,
	}
}

func containsDroppedHistory(items []ContextItem, idx int) bool {
	target := fmt.Sprintf("history-%d", idx)
	for _, item := range items {
		if item.ID == target {
			return true
		}
	}
	return false
}

func hasSummaryPrefix(msg *schema.Message) bool {
	if msg == nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(msg.Content), "[对话历史摘要]")
}
