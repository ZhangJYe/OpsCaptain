package events

import (
	"regexp"
	"strings"
)

var defaultMetricPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(ms|秒|s|%)|P[0-9]+\s*[:：]\s*(\d+(?:\.\d+)?)`)

// ValidateOutputAgainstToolResults 检查输出中的关键指标是否在工具结果中有来源
func ValidateOutputAgainstToolResults(output string, toolResults []string) []string {
	return ValidateOutputWithConfig(nil, output, toolResults)
}

// ValidateOutputWithConfig 带配置的输出校验
func ValidateOutputWithConfig(hc *HallucinationConfig, output string, toolResults []string) []string {
	if len(toolResults) == 0 {
		return nil
	}

	pattern := defaultMetricPattern
	if hc != nil && hc.MetricPattern != nil {
		pattern = hc.MetricPattern
	}

	var warnings []string
	outputMetrics := extractMetricsWithPattern(output, pattern)

	combined := strings.Join(toolResults, " ")
	for _, m := range outputMetrics {
		if !strings.Contains(combined, m) {
			warnings = append(warnings, "指标 "+m+" 在工具结果中未找到来源")
		}
	}

	return warnings
}

func extractMetricsWithPattern(text string, pattern *regexp.Regexp) []string {
	matches := pattern.FindAllString(text, -1)
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

func extractMetrics(text string) []string {
	return extractMetricsWithPattern(text, defaultMetricPattern)
}
