package evalharness

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	secretPattern    = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9._-]+|(?:ghp|github_pat|sk)-[a-z0-9_-]{8,}|(?:api[_-]?key|token|password|secret)\s*[:=]\s*[^\s,;]+)`)
	privateIPPattern = regexp.MustCompile(`\b(?:10(?:\.\d{1,3}){3}|192\.168(?:\.\d{1,3}){2}|172\.(?:1[6-9]|2\d|3[01])(?:\.\d{1,3}){2})\b`)
)

func RedactReport(report *Report, cfg RedactionConfig) {
	if report == nil {
		return
	}
	for suiteIndex := range report.Suites {
		for caseIndex := range report.Suites[suiteIndex].Cases {
			result := &report.Suites[suiteIndex].Cases[caseIndex]
			result.Reason = redactText(result.Reason, cfg.MaxTextChars)
			result.Domain = redactJSON(result.Domain, cfg)
		}
		for failureIndex := range report.Suites[suiteIndex].Failures {
			failure := &report.Suites[suiteIndex].Failures[failureIndex]
			failure.Reason = redactText(failure.Reason, cfg.MaxTextChars)
		}
	}
	for failureIndex := range report.Failures {
		report.Failures[failureIndex].Reason = redactText(report.Failures[failureIndex].Reason, cfg.MaxTextChars)
	}
	for comparisonIndex := range report.PlanGoSComparison {
		for suite, domain := range report.PlanGoSComparison[comparisonIndex].Domain {
			report.PlanGoSComparison[comparisonIndex].Domain[suite] = redactJSON(domain, cfg)
		}
	}
}

func redactJSON(raw json.RawMessage, cfg RedactionConfig) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return json.RawMessage(`{"redacted":"invalid domain payload"}`)
	}
	value = redactValue(value, cfg)
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"redacted":"domain payload unavailable"}`)
	}
	return encoded
}

func redactValue(value any, cfg RedactionConfig) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if sensitiveKey(key, cfg.SensitiveKeys) {
				typed[key] = "[REDACTED]"
				continue
			}
			typed[key] = redactValue(item, cfg)
		}
		return typed
	case []any:
		for index := range typed {
			typed[index] = redactValue(typed[index], cfg)
		}
		return typed
	case string:
		return redactText(typed, cfg.MaxTextChars)
	default:
		return value
	}
}

func sensitiveKey(key string, sensitive []string) bool {
	key = strings.ToLower(key)
	for _, candidate := range sensitive {
		if strings.Contains(key, strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func redactText(value string, maxChars int) string {
	value = secretPattern.ReplaceAllString(value, "[REDACTED]")
	value = privateIPPattern.ReplaceAllString(value, "[PRIVATE_IP]")
	if maxChars <= 0 || utf8.RuneCountInString(value) <= maxChars {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxChars]) + "..."
}
