package contextcompression

import (
	"strings"
)

// logErrorPatterns 日志中的错误行模式
var logErrorPatterns = []string{
	"ERROR", "FATAL", "PANIC", "OOM", "KILLED",
	"Exception", "Traceback", "StackOverflow",
	"error:", "fatal:", "panic:",
	"5xx", "500", "502", "503", "504",
	"crashloopbackoff", "imagepullbackoff", "errimagepull", "back-off",
	"oomkilled", "failedscheduling", "failedmount", "unhealthy",
	"failedcreatepodsandbox", "manifest unknown", "imagepullsecrets",
	"readiness probe failed", "liveness probe failed",
	// 中文运维词
	"告警", "超时", "异常", "故障", "错误", "失败",
	"溢出", "耗尽", "宕机", "崩溃", "中断",
}

// logWarnPatterns 日志中的告警行模式
var logWarnPatterns = []string{
	"WARN", "WARNING", "ALERT",
	"warn:", "warning:",
	"告警", "注意", "警告",
}

// compressLog 压缩日志/文本内容
// 策略:
// 1. 按行分割
// 2. 保留错误行 + 上下文窗口
// 3. 保留告警行
// 4. 保留命中 query 的行
// 5. 去重连续重复行
func compressLog(content, query string, contextLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 3 {
		return content
	}

	queryLower := strings.ToLower(query)
	queryTerms := extractQueryTerms(queryLower)

	total := len(lines)

	// 标记需要保留的行
	keep := make([]bool, total)

	// 第一遍: 标记错误行、告警行、query 命中行
	for i, line := range lines {
		lineTrimmed := strings.TrimSpace(line)
		if lineTrimmed == "" {
			continue
		}
		lineLower := strings.ToLower(line)

		// 错误行
		if containsAny(lineLower, logErrorPatterns) || containsAny(lineTrimmed, logErrorPatterns) {
			keep[i] = true
			// 保留上下文窗口
			for j := max(0, i-contextLines); j <= min(total-1, i+contextLines); j++ {
				keep[j] = true
			}
			continue
		}

		// 告警行
		if containsAny(lineLower, logWarnPatterns) || containsAny(lineTrimmed, logWarnPatterns) {
			keep[i] = true
			continue
		}

		// query 命中
		for _, term := range queryTerms {
			if len(term) >= 2 && strings.Contains(lineLower, term) {
				keep[i] = true
				// 保留上下文窗口
				for j := max(0, i-contextLines); j <= min(total-1, i+contextLines); j++ {
					keep[j] = true
				}
				break
			}
		}
	}

	// 始终保留首尾各 1 行
	if total > 0 {
		keep[0] = true
	}
	if total > 1 {
		keep[total-1] = true
	}

	// 构建结果，同时去重连续重复行
	var result []string
	lastLine := ""
	for i, line := range lines {
		if !keep[i] {
			continue
		}
		trimmed := strings.TrimSpace(line)
		// 去重连续重复行
		if trimmed == lastLine {
			continue
		}
		result = append(result, line)
		lastLine = trimmed
	}

	// 如果没有保留任何行（不太可能），返回原内容
	if len(result) == 0 {
		return content
	}

	return strings.Join(result, "\n")
}
