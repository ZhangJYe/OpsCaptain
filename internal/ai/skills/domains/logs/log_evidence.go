package logs

import (
	"encoding/json"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

func buildLogEvidence(sourceName, output string, limit int) []protocol.EvidenceItem {
	snippets := collectLogSnippets(strings.TrimSpace(output), limit)
	if len(snippets) == 0 {
		return nil
	}
	evidence := make([]protocol.EvidenceItem, 0, len(snippets))
	for idx, snippet := range snippets {
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "log",
			SourceID:   fmt.Sprintf("%s-%d", sourceName, idx+1),
			Title:      fmt.Sprintf("%s log evidence %d", sourceName, idx+1),
			Snippet:    snippet,
			Score:      0.72 - float64(idx)*0.06,
		})
	}
	return evidence
}

func collectLogSnippets(output string, limit int) []string {
	if output == "" || limit <= 0 {
		return nil
	}

	var payload any
	if err := json.Unmarshal([]byte(output), &payload); err == nil {
		snippets := collectFromValue(payload, limit)
		return dedupAndLimit(snippets, limit)
	}

	return dedupAndLimit(splitLogLines(output), limit)
}

func collectFromValue(value any, limit int) []string {
	if limit <= 0 || value == nil {
		return nil
	}

	switch typed := value.(type) {
	case map[string]any:
		var snippets []string
		for _, key := range []string{"logs", "items", "results", "data", "entries", "records"} {
			if nested, ok := typed[key]; ok {
				snippets = append(snippets, collectFromValue(nested, limit-len(snippets))...)
				if len(snippets) >= limit {
					return snippets[:limit]
				}
			}
		}
		if snippet := snippetFromMap(typed); snippet != "" {
			snippets = append(snippets, snippet)
		}
		if len(snippets) == 0 {
			if raw, err := json.Marshal(typed); err == nil {
				snippets = append(snippets, shorten(string(raw), 200))
			}
		}
		return snippets
	case []any:
		var snippets []string
		for _, item := range typed {
			snippets = append(snippets, collectFromValue(item, limit-len(snippets))...)
			if len(snippets) >= limit {
				return snippets[:limit]
			}
		}
		return snippets
	case string:
		return splitLogLines(typed)
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text == "" || text == "<nil>" {
			return nil
		}
		return []string{shorten(text, 200)}
	}
}

func snippetFromMap(item map[string]any) string {
	message := firstString(item, "message", "msg", "content", "text", "log", "line", "raw", "description")
	if message == "" {
		return ""
	}
	timestamp := firstString(item, "timestamp", "time", "ts")
	level := firstString(item, "level", "severity")
	source := firstString(item, "service", "app", "source", "host")

	parts := make([]string, 0, 4)
	if timestamp != "" {
		parts = append(parts, timestamp)
	}
	if level != "" {
		parts = append(parts, "["+level+"]")
	}
	if source != "" {
		parts = append(parts, "("+source+")")
	}
	parts = append(parts, message)
	return shorten(strings.Join(parts, " "), 200)
}

func splitLogLines(output string) []string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, shorten(line, 200))
	}
	return out
}

func dedupAndLimit(items []string, limit int) []string {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, min(limit, len(items)))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func firstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fallbackSnippet(output, desc string) string {
	if snippets := collectLogSnippets(output, 1); len(snippets) > 0 {
		return snippets[0]
	}
	return shorten(desc, 160)
}

func fallback(value, alt string) string {
	if strings.TrimSpace(value) == "" {
		return alt
	}
	return value
}

func shorten(input string, max int) string {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
	if len(input) <= max {
		return input
	}
	return input[:max] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}