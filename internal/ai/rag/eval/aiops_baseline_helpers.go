package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func observationKeywordSet(observations []AIOPSKeyObservation) map[string]struct{} {
	out := make(map[string]struct{})
	for _, obs := range observations {
		for _, kw := range obs.Keyword {
			k := strings.ToLower(strings.TrimSpace(kw))
			if k != "" {
				out[k] = struct{}{}
			}
		}
	}
	return out
}

func keywordSetOverlap(a, b map[string]struct{}) int {
	count := 0
	for k := range a {
		if _, ok := b[k]; ok {
			count++
		}
	}
	return count
}

func unionIDs(groups ...[]string) []string {
	merged := make([]string, 0)
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return uniqueNonEmpty(merged)
}

func appendEvalNotes(base string, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	switch {
	case base == "":
		return extra
	case extra == "":
		return base
	default:
		return base + "; " + extra
	}
}

func joinTopKeywords(keywords []string, limit int) string {
	items := uniqueNonEmpty(keywords)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return strings.Join(items, " ")
}

func observationKeywords(observations []AIOPSKeyObservation) []string {
	var out []string
	for _, obs := range observations {
		out = append(out, strings.TrimSpace(obs.Type))
		out = append(out, obs.Keyword...)
	}
	return uniqueNonEmpty(out)
}

func joinKeywords(values []string) string {
	return strings.Join(uniqueNonEmpty(values), ", ")
}

func uniqueNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func fallback(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func nonEmptyOr(value string, fallbackValue string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallbackValue
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir json dir: %w", err)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json %s: %w", path, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write json %s: %w", path, err)
	}
	return nil
}
