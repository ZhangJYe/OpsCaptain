package safety

import (
	"context"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

type FilteredOutput struct {
	Content  string
	Redacted bool
	Reasons  []string
}

type outputPattern struct {
	name        string
	replacement string
	regex       *regexp.Regexp
}

var outputPatterns = []outputPattern{
	{
		name:        "system_prompt_block",
		replacement: "[REDACTED_SYSTEM_PROMPT]",
		regex:       regexp.MustCompile(`(?is)<<\s*sys\s*>>.*?<</\s*sys\s*>>`),
	},
	{
		name:        "system_prompt_line",
		replacement: "[REDACTED_SYSTEM_PROMPT]",
		regex:       regexp.MustCompile(`(?im)^\s*system\s*:\s*.*$`),
	},
	{
		name:        "inst_block",
		replacement: "[REDACTED_SYSTEM_PROMPT]",
		regex:       regexp.MustCompile(`(?is)\[inst\].*?\[/inst\]`),
	},
	{
		name:        "api_key",
		replacement: "[REDACTED_API_KEY]",
		regex:       regexp.MustCompile(`(?i)(sk-[a-z0-9]{16,}|api[_-]?key\s*[:=]\s*[a-z0-9._-]{10,}|bearer\s+[a-z0-9._-]{16,})`),
	},
	{
		name:        "internal_ip",
		replacement: "[REDACTED_INTERNAL_IP]",
		regex:       regexp.MustCompile(`\b(?:10\.\d{1,3}\.\d{1,3}\.\d{1,3}|127\.\d{1,3}\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[0-1])\.\d{1,3}\.\d{1,3})\b`),
	},
}

func FilterOutput(ctx context.Context, content string) FilteredOutput {
	if !outputFilterEnabled(ctx) || strings.TrimSpace(content) == "" {
		return FilteredOutput{Content: content}
	}

	filtered := content
	reasons := make([]string, 0, 2)
	for _, pattern := range outputPatterns {
		if !pattern.regex.MatchString(filtered) {
			continue
		}
		filtered = pattern.regex.ReplaceAllString(filtered, pattern.replacement)
		reasons = append(reasons, pattern.name)
	}
	return FilteredOutput{
		Content:  filtered,
		Redacted: len(reasons) > 0,
		Reasons:  reasons,
	}
}

func FilterDetails(ctx context.Context, details []string) []string {
	if len(details) == 0 {
		return nil
	}
	out := make([]string, 0, len(details))
	for _, detail := range details {
		out = append(out, FilterOutput(ctx, detail).Content)
	}
	return out
}

func outputFilterEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "safety.output_filter.enabled")
	if err != nil || v.String() == "" {
		return true
	}
	return v.Bool()
}
