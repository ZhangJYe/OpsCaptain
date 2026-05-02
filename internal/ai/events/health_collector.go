package events

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// ToolHealthReport 工具健康度报告
type ToolHealthReport struct {
	ToolName      string    `json:"tool_name"`
	TotalCalls    int       `json:"total_calls"`
	SuccessCount  int       `json:"success_count"`
	FailCount     int       `json:"fail_count"`
	SuccessRate   float64   `json:"success_rate"`
	P50DurationMs int64     `json:"p50_duration_ms"`
	P95DurationMs int64     `json:"p95_duration_ms"`
	P99DurationMs int64     `json:"p99_duration_ms"`
	AvgDurationMs int64     `json:"avg_duration_ms"`
	CommonErrors  []string  `json:"common_errors,omitempty"`
	LastFailure   time.Time `json:"last_failure,omitempty"`
	LastCall      time.Time `json:"last_call"`
}

// toolCallRecord 单次工具调用记录
type toolCallRecord struct {
	toolName  string
	duration  time.Duration
	success   bool
	errMsg    string
	timestamp time.Time
}

// HealthCollector 工具健康度收集器
type HealthCollector struct {
	mu      sync.RWMutex
	records map[string][]toolCallRecord // toolName -> records
	maxPer  int                         // 每个工具最多保留的记录数
}

// NewHealthCollector 创建健康度收集器
// maxPer: 每个工具最多保留的记录数（超出后丢弃最早的）
func NewHealthCollector(maxPer int) *HealthCollector {
	if maxPer <= 0 {
		maxPer = 1000
	}
	return &HealthCollector{
		records: make(map[string][]toolCallRecord),
		maxPer:  maxPer,
	}
}

// Emitter 实现 Emitter 接口，收集 tool_call_end 事件
func (h *HealthCollector) Emit(ctx context.Context, event AgentEvent) {
	if event.Type != EventToolCallEnd {
		return
	}

	toolName, _ := event.Payload["tool_name"].(string)
	if toolName == "" {
		return
	}

	durationMs, _ := event.Payload["duration_ms"].(int64)
	success, _ := event.Payload["success"].(bool)
	errMsg, _ := event.Payload["error"].(string)

	record := toolCallRecord{
		toolName:  toolName,
		duration:  time.Duration(durationMs) * time.Millisecond,
		success:   success,
		errMsg:    errMsg,
		timestamp: event.Timestamp,
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	records := h.records[toolName]
	records = append(records, record)
	if len(records) > h.maxPer {
		records = records[len(records)-h.maxPer:]
	}
	h.records[toolName] = records
}

// Report 生成单个工具的健康度报告
func (h *HealthCollector) Report(toolName string) *ToolHealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()

	records, ok := h.records[toolName]
	if !ok || len(records) == 0 {
		return nil
	}

	return buildReport(toolName, records)
}

// Reports 生成所有工具的健康度报告
func (h *HealthCollector) Reports() []ToolHealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()

	reports := make([]ToolHealthReport, 0, len(h.records))
	for toolName, records := range h.records {
		if len(records) > 0 {
			reports = append(reports, *buildReport(toolName, records))
		}
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].ToolName < reports[j].ToolName
	})

	return reports
}

// globalHealthCollector 全局健康度收集器（单例）
var (
	globalHC     *HealthCollector
	globalHCOnce sync.Once
)

// GlobalHealthCollector 获取全局健康度收集器
func GlobalHealthCollector() *HealthCollector {
	globalHCOnce.Do(func() {
		globalHC = NewHealthCollector(1000)
	})
	return globalHC
}

// StartPeriodicReport 启动定期日志聚合
// interval: 聚合间隔；ctx 取消时停止
func (h *HealthCollector) StartPeriodicReport(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.logReport(ctx)
			}
		}
	}()
}

func (h *HealthCollector) logReport(ctx context.Context) {
	reports := h.Reports()
	if len(reports) == 0 {
		return
	}
	for _, r := range reports {
		g.Log().Infof(ctx,
			"[health] tool=%s calls=%d success_rate=%.1f%% p50=%dms p95=%dms p99=%dms errors=%v",
			r.ToolName, r.TotalCalls, r.SuccessRate*100,
			r.P50DurationMs, r.P95DurationMs, r.P99DurationMs,
			r.CommonErrors,
		)
	}
}

// Reset 清空所有记录
func (h *HealthCollector) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = make(map[string][]toolCallRecord)
}

func buildReport(toolName string, records []toolCallRecord) *ToolHealthReport {
	report := &ToolHealthReport{
		ToolName:   toolName,
		TotalCalls: len(records),
		LastCall:   records[len(records)-1].timestamp,
	}

	durations := make([]time.Duration, 0, len(records))
	errorCounts := make(map[string]int)

	for _, r := range records {
		if r.success {
			report.SuccessCount++
		} else {
			report.FailCount++
			report.LastFailure = r.timestamp
			if r.errMsg != "" {
				errorCounts[r.errMsg]++
			}
		}
		durations = append(durations, r.duration)
	}

	if report.TotalCalls > 0 {
		report.SuccessRate = float64(report.SuccessCount) / float64(report.TotalCalls)
	}

	// 计算分位数
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	report.P50DurationMs = percentile(durations, 0.5).Milliseconds()
	report.P95DurationMs = percentile(durations, 0.95).Milliseconds()
	report.P99DurationMs = percentile(durations, 0.99).Milliseconds()

	// 平均耗时
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	report.AvgDurationMs = (total / time.Duration(len(durations))).Milliseconds()

	// Top 3 常见错误
	report.CommonErrors = topErrors(errorCounts, 3)

	return report
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func topErrors(counts map[string]int, limit int) []string {
	type errCount struct {
		msg   string
		count int
	}
	errs := make([]errCount, 0, len(counts))
	for msg, count := range counts {
		errs = append(errs, errCount{msg, count})
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].count > errs[j].count })

	result := make([]string, 0, limit)
	for i, e := range errs {
		if i >= limit {
			break
		}
		result = append(result, e.msg)
	}
	return result
}
