package contextengine

import (
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/utility/mem"
)

func ToolItemsFromResults(results []*protocol.TaskResult) []ContextItem {
	if len(results) == 0 {
		return nil
	}
	items := make([]ContextItem, 0)
	for _, result := range results {
		if result == nil {
			continue
		}
		if len(result.Evidence) > 0 {
			for _, evidence := range result.Evidence {
				content := strings.TrimSpace(evidence.Snippet)
				if content == "" {
					continue
				}
				items = append(items, ContextItem{
					ID:            itemID(result.Agent, evidence.SourceID, evidence.Title),
					SourceType:    normalizeSourceType(evidence.SourceType),
					SourceID:      fallbackString(evidence.SourceID, evidence.Title),
					Title:         fallbackString(evidence.Title, result.Agent+" evidence"),
					Content:       content,
					Score:         evidence.Score,
					TrustLevel:    "tool_evidence",
					TokenEstimate: mem.EstimateTokens(content),
					OriginAgent:   result.Agent,
					SafetyLabel:   "tool_output",
					UpdatePolicy:  "ephemeral",
					ConflictGroup: result.Agent,
				})
			}
			continue
		}

		summary := strings.TrimSpace(result.Summary)
		if summary == "" {
			continue
		}
		items = append(items, ContextItem{
			ID:            itemID(result.Agent, result.TaskID, "summary"),
			SourceType:    "tool_result",
			SourceID:      result.TaskID,
			Title:         result.Agent + " summary",
			Content:       summary,
			Score:         result.Confidence,
			TrustLevel:    "tool_result",
			TokenEstimate: mem.EstimateTokens(summary),
			OriginAgent:   result.Agent,
			SafetyLabel:   "tool_output",
			UpdatePolicy:  "ephemeral",
			ConflictGroup: result.Agent,
		})
	}
	return items
}

func ToolItemSnippets(items []ContextItem, limit int) []string {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	out := make([]string, 0, min(limit, len(items)))
	for idx, item := range items {
		if idx >= limit {
			break
		}
		label := fallbackString(item.Title, item.SourceID)
		snippet := fallbackString(strings.TrimSpace(item.Content), "无摘要")
		out = append(out, fmt.Sprintf("%s：%s", label, snippet))
	}
	return out
}

func fallbackString(value, alt string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return alt
}

func itemID(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return "context-item"
	}
	return strings.Join(filtered, ":")
}

func normalizeSourceType(value string) string {
	if strings.TrimSpace(value) == "" {
		return "tool_result"
	}
	return value
}
