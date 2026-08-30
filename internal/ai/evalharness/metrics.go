package evalharness

import (
	"sort"
	"strings"
	"time"
)

var failurePhases = map[string]struct{}{
	"route": {}, "retrieve": {}, "plan": {}, "act": {}, "update": {}, "report": {}, "evidence": {},
}

func NormalizeFailurePhase(phase, reason string) string {
	phase = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(phase, "_failed")))
	if _, ok := failurePhases[phase]; ok {
		return phase
	}
	reason = strings.ToLower(reason)
	for _, candidate := range []string{"route", "retrieve", "plan", "act", "update", "evidence", "report"} {
		if strings.Contains(reason, candidate) {
			return candidate
		}
	}
	return "report"
}

func AggregateCommon(results []CaseResult) CommonMetrics {
	metrics := CommonMetrics{Cases: len(results)}
	var usage Usage
	latencies := make([]time.Duration, 0, len(results))
	traceComplete := 0
	evidencePresent := 0
	for _, result := range results {
		switch result.Status {
		case StatusSucceeded:
			metrics.Succeeded++
		case StatusDegraded:
			metrics.Degraded++
		case StatusSkipped:
			metrics.Skipped++
		case StatusBudgetExceeded:
			metrics.BudgetExceeded++
		default:
			metrics.Failed++
		}
		usage.LLMCalls += result.Usage.LLMCalls
		usage.ToolCalls += result.Usage.ToolCalls
		usage.RAGCalls += result.Usage.RAGCalls
		usage.Tokens += result.Usage.Tokens
		usage.Cost += result.Usage.Cost
		latencies = append(latencies, result.Latency)
		if result.TraceComplete {
			traceComplete++
		}
		if result.EvidenceCount > 0 {
			evidencePresent++
		}
	}
	if metrics.Cases > 0 {
		count := float64(metrics.Cases)
		metrics.SuccessRate = AvailableMetric(float64(metrics.Succeeded)/count, "ratio")
		metrics.FailureRate = AvailableMetric(float64(metrics.Failed+metrics.BudgetExceeded)/count, "ratio")
		metrics.DegradationRate = AvailableMetric(float64(metrics.Degraded)/count, "ratio")
		metrics.TraceCompleteness = AvailableMetric(float64(traceComplete)/count, "ratio")
		metrics.EvidenceCoverage = AvailableMetric(float64(evidencePresent)/count, "ratio")
		metrics.P95LatencyMS = AvailableMetric(float64(percentileDuration(latencies, 0.95).Milliseconds()), "ms")
	} else {
		metrics.SuccessRate = UnavailableMetric("ratio")
		metrics.FailureRate = UnavailableMetric("ratio")
		metrics.DegradationRate = UnavailableMetric("ratio")
		metrics.TraceCompleteness = UnavailableMetric("ratio")
		metrics.EvidenceCoverage = UnavailableMetric("ratio")
		metrics.P95LatencyMS = UnavailableMetric("ms")
	}
	metrics.LLMCalls = AvailableMetric(float64(usage.LLMCalls), "count")
	metrics.ToolCalls = AvailableMetric(float64(usage.ToolCalls), "count")
	metrics.RAGCalls = AvailableMetric(float64(usage.RAGCalls), "count")
	if usage.Tokens > 0 {
		metrics.Tokens = AvailableMetric(float64(usage.Tokens), "tokens")
	} else {
		metrics.Tokens = UnavailableMetric("tokens")
	}
	if usage.Cost > 0 {
		metrics.Cost = AvailableMetric(usage.Cost, "currency")
	} else {
		metrics.Cost = UnavailableMetric("currency")
	}
	return metrics
}

func percentileDuration(values []time.Duration, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]time.Duration(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	index := int(float64(len(copyValues)-1) * percentile)
	return copyValues[index]
}

func reportStatus(suites []SuiteReport, cross []GateResult) SuiteStatus {
	status := StatusSucceeded
	allSkipped := len(suites) > 0
	for _, suite := range suites {
		if suite.Status != StatusSkipped {
			allSkipped = false
		}
		switch suite.Status {
		case StatusFailed:
			return StatusFailed
		case StatusBudgetExceeded:
			status = StatusBudgetExceeded
		case StatusDegraded:
			if status == StatusSucceeded {
				status = StatusDegraded
			}
		}
		for _, gate := range suite.Gates {
			if gate.Severity == GateBlocking && !gate.Passed {
				return StatusFailed
			}
		}
	}
	if allSkipped {
		return StatusSkipped
	}
	for _, gate := range cross {
		if gate.Severity == GateBlocking && !gate.Passed {
			return StatusFailed
		}
	}
	return status
}
