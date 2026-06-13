package contextengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/contextcompression"
	"SuperBizAgent/internal/ai/memory"

	"github.com/cloudwego/eino/schema"
)

func selectToolItems(items []ContextItem, profile ContextProfile) ([]ContextItem, []ContextItem, int, []string) {
	selected, dropped, used, notes, _ := selectToolItemsWithCompression(context.Background(), items, profile, "", nil)
	return selected, dropped, used, notes
}

func selectToolItemsWithCompression(ctx context.Context, items []ContextItem, profile ContextProfile, query string, compCfg *contextcompression.CompressionConfig) ([]ContextItem, []ContextItem, int, []string, []contextcompression.Report) {
	if len(items) == 0 || profile.MaxToolItems == 0 || profile.Budget.ToolTokens == 0 {
		return nil, nil, 0, []string{"tool results empty or disabled"}, nil
	}

	remaining := profile.Budget.ToolTokens
	selected := make([]ContextItem, 0, min(len(items), profile.MaxToolItems))
	dropped := make([]ContextItem, 0)
	used := 0
	compressionReports := make([]contextcompression.Report, 0)
	for idx, item := range items {
		item.TokenEstimate = memory.EstimateTokens(item.Content)
		if idx >= profile.MaxToolItems {
			item.DroppedReason = "tool_window"
			dropped = append(dropped, item)
			continue
		}

		if compCfg != nil && compCfg.Enabled && compCfg.Mode != contextcompression.ModeOff {
			compResult := contextcompression.Compress(ctx, contextcompression.Request{
				SourceType: contextcompression.SourceTool,
				SourceID:   item.SourceID,
				Query:      query,
				Content:    item.Content,
			}, compCfg)
			if shouldRecordCompressionReport(compResult.Report) {
				compressionReports = append(compressionReports, compResult.Report)
			}
			if compCfg.Mode == contextcompression.ModeOptimize &&
				!compResult.Report.Degraded &&
				compResult.Report.CompressionRatio < 1.0 {
				item.Content = compResult.Content
				item.TokenEstimate = memory.EstimateTokens(item.Content)
				item.CompressionLevel = "compressed"
			} else if compCfg.Mode == contextcompression.ModeAudit && shouldRecordCompressionReport(compResult.Report) {
				item.CompressionLevel = "audit"
			}
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
			if item.CompressionLevel == "" || item.CompressionLevel == "audit" {
				item.CompressionLevel = "trimmed"
			}
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
	return selected, dropped, used, []string{fmt.Sprintf("tokens=%d/%d", used, profile.Budget.ToolTokens)}, compressionReports
}

func shouldRecordCompressionReport(report contextcompression.Report) bool {
	return report.Strategy != "" &&
		report.Strategy != "disabled" &&
		report.Strategy != "source_type_excluded" &&
		report.Strategy != "below_min_tokens"
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
