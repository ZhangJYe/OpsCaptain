package contextengine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/memory"

	"github.com/cloudwego/eino/schema"
)

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
	if len(trace.CompressionReports) > 0 {
		totalBefore := 0
		totalAfter := 0
		for _, r := range trace.CompressionReports {
			totalBefore += r.TokensBefore
			totalAfter += r.TokensAfter
		}
		ratio := 1.0
		if totalBefore > 0 {
			ratio = float64(totalAfter) / float64(totalBefore)
		}
		details = append(details, fmt.Sprintf("compression items=%d before=%d after=%d ratio=%.2f",
			len(trace.CompressionReports), totalBefore, totalAfter, ratio))
	}
	return details
}
