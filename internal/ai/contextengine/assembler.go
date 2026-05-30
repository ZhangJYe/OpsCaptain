package contextengine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/memory"

	"github.com/cloudwego/eino/schema"
)

type Assembler struct {
	resolver     *PolicyResolver
	now          func() time.Time
	historyRec   *HistoryRecaller
	toolRec      *ToolRecaller
	toolReranker *ToolReranker
	intentRec    *IntentRecognizer
}

func NewAssembler() *Assembler {
	return &Assembler{
		resolver:   NewPolicyResolver(),
		now:        time.Now,
		historyRec: NewHistoryRecaller(),
		toolRec:    NewToolRecaller(),
	}
}

func (a *Assembler) WithToolReranker(r *ToolReranker) *Assembler {
	a.toolReranker = r
	return a
}

func (a *Assembler) WithIntentRecognizer(r *IntentRecognizer) *Assembler {
	a.intentRec = r
	return a
}

func (a *Assembler) Assemble(ctx context.Context, req ContextRequest, history []*schema.Message) (*ContextPackage, error) {
	start := a.now()
	profile := a.resolver.Resolve(ctx, req)

	trace := ContextAssemblyTrace{
		Profile: profile.Name,
		BudgetBefore: BudgetSnapshot{
			HistoryTokens:  profile.Budget.HistoryTokens,
			MemoryTokens:   profile.Budget.MemoryTokens,
			DocumentTokens: profile.Budget.DocumentTokens,
			ToolTokens:     profile.Budget.ToolTokens,
		},
	}

	if a.intentRec != nil {
		intentResult := a.intentRec.Recognize(ctx, req.Query)
		trace.Intent = &intentResult
		if !intentResult.Degraded && intentResult.Type != IntentUnknown {
			overridden := a.resolver.ResolveByProfile(ctx, req, ProfileForIntent(intentResult.Type))
			if overridden.Name != "" {
				profile = overridden
				trace.Profile = profile.Name
			}
		}
	}

	pkg := &ContextPackage{
		Request: req,
		Profile: profile,
		Query:   strings.TrimSpace(req.Query),
	}

	if profile.AllowHistory && len(history) > 0 {
		recallResult := a.historyRec.Recall(ctx, req.Query, history, profile.MaxHistoryMessages)
		trace.HistoryRecall = &recallResult

		selectedHistory, droppedHistory, usedHistory, historyNotes := selectHistory(recallResult.Messages, profile)
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
			Recall: &RecallStageMetrics{
				CacheHits: recallResult.CacheHits,
				Embedded:  recallResult.Embedded,
				LatencyMs: recallResult.LatencyMs,
				Degraded:  recallResult.Degraded,
			},
		})
	}

	if profile.AllowMemory && req.SessionID != "" {
		retrieveLimit := profile.MaxMemoryItems
		if retrieveLimit < 1 {
			retrieveLimit = 1
		}
		entries := memory.GetLongTermMemory().RetrieveScoped(ctx, req.Query, retrieveLimit*3, memory.MemoryRetrievePolicy{
			ReadOnly:  true,
			ScopeRefs: memoryScopeRefs(req),
		})
		entries = rankMemoryEntries(entries, profile, a.now())
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
		toolRecallResult := a.toolRec.Recall(ctx, req.Query, req.ToolItems, profile.MaxToolItems)
		trace.ToolRecall = &toolRecallResult

		items := toolRecallResult.Items
		if a.toolReranker != nil {
			rerankOutcome := a.toolReranker.Rerank(ctx, req.Query, items, profile.MaxToolItems)
			trace.ToolRerank = &rerankOutcome
			if !rerankOutcome.Degraded {
				items = rerankOutcome.Items
			}
		}

		selectedTools, droppedTools, usedTools, toolNotes := selectToolItems(items, profile)
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
			Recall: &RecallStageMetrics{
				LatencyMs: toolRecallResult.LatencyMs,
			},
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

func memoryScopeRefs(req ContextRequest) []memory.MemoryScopeRef {
	refs := []memory.MemoryScopeRef{
		{Scope: memory.MemoryScopeSession, ScopeID: req.SessionID},
		{Scope: memory.MemoryScopeGlobal, ScopeID: "global"},
	}
	if strings.TrimSpace(req.UserID) != "" {
		refs = append(refs, memory.MemoryScopeRef{Scope: memory.MemoryScopeUser, ScopeID: strings.TrimSpace(req.UserID)})
	}
	if strings.TrimSpace(req.ProjectID) != "" {
		refs = append(refs, memory.MemoryScopeRef{Scope: memory.MemoryScopeProject, ScopeID: strings.TrimSpace(req.ProjectID)})
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
		item.TokenEstimate = memory.EstimateTokens(item.Content)
		if idx >= profile.MaxToolItems {
			item.DroppedReason = "tool_window"
			dropped = append(dropped, item)
			continue
		}
		if item.TokenEstimate > remaining {
			trimmed := memory.TrimToTokenBudget(item.Content, remaining)
			if strings.TrimSpace(trimmed) == "" {
				item.DroppedReason = "tool_budget"
				dropped = append(dropped, item)
				continue
			}
			item.Content = trimmed
			item.TokenEstimate = memory.EstimateTokens(trimmed)
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
		tokens := memory.EstimateTokens(history[i].Content)
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
		summaryTokens := memory.EstimateTokens(history[0].Content)
		if summaryTokens <= remaining {
			selectedIdx[0] = true
			remaining -= summaryTokens
			used += summaryTokens
		}
		if len(history) > 1 && !selectedIdx[1] {
			replyTokens := memory.EstimateTokens(history[1].Content)
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

func selectMemories(entries []*memory.MemoryEntry, profile ContextProfile, now time.Time) ([]ContextItem, []ContextItem, int, []string) {
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

func newMemoryItem(entry *memory.MemoryEntry) ContextItem {
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
		TokenEstimate:  memory.EstimateTokens(entry.Content),
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
		scope = string(memory.MemoryScopeSession)
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

func memoryTrustLevel(entry *memory.MemoryEntry) string {
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
		TokenEstimate: memory.EstimateTokens(content),
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

func rankMemoryEntries(entries []*memory.MemoryEntry, profile ContextProfile, now time.Time) []*memory.MemoryEntry {
	if len(entries) <= 1 {
		return entries
	}
	type scored struct {
		entry *memory.MemoryEntry
		score float64
	}
	scoredEntries := make([]scored, len(entries))
	for i, entry := range entries {
		scoredEntries[i] = scored{entry: entry, score: memoryCompositeScore(entry, profile, now)}
	}
	sort.Slice(scoredEntries, func(i, j int) bool {
		return scoredEntries[i].score > scoredEntries[j].score
	})
	result := make([]*memory.MemoryEntry, len(entries))
	for i, s := range scoredEntries {
		result[i] = s.entry
	}
	return result
}

func memoryCompositeScore(entry *memory.MemoryEntry, profile ContextProfile, now time.Time) float64 {
	confidence := entry.Confidence

	freshness := 1.0
	hours := now.Sub(entry.LastUsed).Hours()
	freshness = 1.0 / (1.0 + hours/24.0)

	scopePriority := 0.0
	switch entry.Scope {
	case memory.MemoryScopeSession:
		scopePriority = 0.4
	case memory.MemoryScopeUser:
		scopePriority = 0.3
	case memory.MemoryScopeProject:
		scopePriority = 0.2
	case memory.MemoryScopeGlobal:
		scopePriority = 0.1
	}

	return confidence*0.5 + freshness*0.3 + scopePriority*0.2
}
