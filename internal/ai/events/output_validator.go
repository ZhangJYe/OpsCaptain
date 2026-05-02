package events

import (
	"regexp"
	"strings"
)

var metricPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(ms|秒|s|%)|P[0-9]+[[:space:]]*[:：][[:space:]]*(\d+(?:\.\d+)?)`)

// ValidateOutputAgainstToolResults 检查输出中的关键指标是否在工具结果中有来源
// 返回警告列表，空列表表示没有发现问题
func ValidateOutputAgainstToolResults(output string, toolResults []string) []string {
	if len(toolResults) == 0 {
		return nil
	}

	var warnings []string
	outputMetrics := extractMetrics(output)

	combined := strings.Join(toolResults, " ")
	for _, m := range outputMetrics {
		if !strings.Contains(combined, m) {
			warnings = append(warnings, "指标 "+m+" 在工具结果中未找到来源")
		}
	}

	return warnings
}

func extractMetrics(text string) []string {
	matches := metricPattern.FindAllString(text, -1)
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}
