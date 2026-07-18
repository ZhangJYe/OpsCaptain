package experts

import (
	"encoding/json"
	"regexp"
	"strings"
)

func parseToolOutput(output string) ToolOutput {
	var toolOutput ToolOutput
	if err := json.Unmarshal([]byte(output), &toolOutput); err != nil {
		return ToolOutput{
			Content: output,
		}
	}

	hasExplicitFields := false
	hasSuccess := false
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(output), &raw); err == nil {
		if _, ok := raw["success"]; ok {
			hasExplicitFields = true
			hasSuccess = true
		}
		if _, ok := raw["degraded"]; ok {
			hasExplicitFields = true
		}
		if _, ok := raw["isError"]; ok {
			hasExplicitFields = true
		}
	}

	toolOutput.HasExplicitFields = hasExplicitFields
	toolOutput.HasSuccess = hasSuccess

	if !hasExplicitFields {
		return ToolOutput{
			Content: output,
		}
	}

	return toolOutput
}

func isEmptyRetrievalOutput(output string) bool {
	s := strings.TrimSpace(output)
	if s == "" || s == "[]" || s == "null" {
		return true
	}

	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return len(arr) == 0
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return false
	}

	for _, key := range []string{"data", "content", "alerts", "documents", "results"} {
		value, ok := obj[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case []interface{}:
			if len(v) == 0 {
				return true
			}
		case string:
			if strings.TrimSpace(v) == "" || strings.TrimSpace(v) == "[]" {
				return true
			}
		}
	}

	return false
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret|authorization)\s*[:=]\s*["']?([^\s"']{8,})["']?`),
	regexp.MustCompile(`(?i)bearer\s+[^\s]+`),
	regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
	regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
}

func redactSecrets(s string) string {
	result := s
	for _, pattern := range secretPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}
