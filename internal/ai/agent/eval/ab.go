package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func RunAB(ctx context.Context, cases []DiagCase, baseline, candidate Runner, judge Judge) (*ABReport, error) {
	if baseline == nil {
		return nil, fmt.Errorf("baseline runner is nil")
	}
	if candidate == nil {
		return nil, fmt.Errorf("candidate runner is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	report := &ABReport{
		Cases:   len(cases),
		Results: make([]ABCaseResult, 0, len(cases)),
	}

	for _, tc := range cases {
		item := ABCaseResult{
			CaseID: tc.ID,
			Query:  tc.Query,
		}

		baseResult, baseErr := baseline.Run(ctx, tc.Query)
		candResult, candErr := candidate.Run(ctx, tc.Query)
		item.Baseline = normalizeRunResult(baseResult)
		item.Candidate = normalizeRunResult(candResult)
		if baseErr != nil {
			item.Failures = append(item.Failures, fmt.Sprintf("baseline error: %v", baseErr))
		}
		if candErr != nil {
			item.Failures = append(item.Failures, fmt.Sprintf("candidate error: %v", candErr))
		}

		if judge != nil && baseErr == nil && candErr == nil && item.Baseline != nil && item.Candidate != nil {
			baseScores, err := judge.Score(ctx, tc.Query, item.Baseline.Summary)
			if err != nil {
				item.Failures = append(item.Failures, fmt.Sprintf("baseline judge error: %v", err))
			} else {
				item.BaselineScores = baseScores
			}
			candScores, err := judge.Score(ctx, tc.Query, item.Candidate.Summary)
			if err != nil {
				item.Failures = append(item.Failures, fmt.Sprintf("candidate judge error: %v", err))
			} else {
				item.CandidateScores = candScores
			}
			if len(item.Failures) == 0 {
				item.DeltaScores = scoreDelta(item.CandidateScores, item.BaselineScores)
			}
		}

		report.Results = append(report.Results, item)
	}

	summarizeABReport(report)
	return report, nil
}

func (r *ABReport) Markdown() string {
	if r == nil {
		return ""
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = "Multi-Agent Evaluation"
	}
	var b strings.Builder
	b.WriteString("# A/B 对比报告\n\n")
	b.WriteString(fmt.Sprintf("- 优化项: %s\n", name))
	b.WriteString(fmt.Sprintf("- Case 数: %d\n\n", r.Cases))
	b.WriteString("| 指标 | 基线 | 候选 | 变化 |\n")
	b.WriteString("|---|---:|---:|---:|\n")
	writeScoreRow(&b, "正确性", r.BaselineAverageScores.Correctness, r.CandidateAverageScores.Correctness, r.DeltaAverageScores.Correctness)
	writeScoreRow(&b, "完整性", r.BaselineAverageScores.Completeness, r.CandidateAverageScores.Completeness, r.DeltaAverageScores.Completeness)
	writeScoreRow(&b, "逻辑性", r.BaselineAverageScores.Coherence, r.CandidateAverageScores.Coherence, r.DeltaAverageScores.Coherence)
	writeScoreRow(&b, "可操作性", r.BaselineAverageScores.Actionability, r.CandidateAverageScores.Actionability, r.DeltaAverageScores.Actionability)
	writeScoreRow(&b, "综合分", r.BaselineAverageScores.Overall, r.CandidateAverageScores.Overall, r.DeltaAverageScores.Overall)
	b.WriteString(fmt.Sprintf("| 中位延迟(ms) | %d | %d | %+d |\n", r.BaselineMedianLatencyMs, r.CandidateMedianLatencyMs, r.CandidateMedianLatencyMs-r.BaselineMedianLatencyMs))
	b.WriteString(fmt.Sprintf("| 平均 Token | %.1f | %.1f | %+.1f |\n", r.BaselineAverageTokens, r.CandidateAverageTokens, r.CandidateAverageTokens-r.BaselineAverageTokens))
	b.WriteString(fmt.Sprintf("| 平均 LLM 调用 | %.1f | %.1f | %+.1f |\n", r.BaselineAverageLLMCalls, r.CandidateAverageLLMCalls, r.CandidateAverageLLMCalls-r.BaselineAverageLLMCalls))
	return b.String()
}

func summarizeABReport(report *ABReport) {
	if report == nil || len(report.Results) == 0 {
		return
	}
	var (
		baseScores       []DiagScores
		candScores       []DiagScores
		baseLatencies    []int64
		candLatencies    []int64
		baseTokens       int64
		candTokens       int64
		baseLLMCalls     int
		candLLMCalls     int
		baseTokenCount   int
		candTokenCount   int
		baseLLMCallCount int
		candLLMCallCount int
	)
	for _, item := range report.Results {
		if hasScores(item.BaselineScores) {
			baseScores = append(baseScores, item.BaselineScores)
		}
		if hasScores(item.CandidateScores) {
			candScores = append(candScores, item.CandidateScores)
		}
		if item.Baseline != nil {
			if item.Baseline.LatencyMillis > 0 {
				baseLatencies = append(baseLatencies, item.Baseline.LatencyMillis)
			}
			if item.Baseline.TokensUsed > 0 {
				baseTokens += item.Baseline.TokensUsed
				baseTokenCount++
			}
			if item.Baseline.LLMCalls > 0 {
				baseLLMCalls += item.Baseline.LLMCalls
				baseLLMCallCount++
			}
		}
		if item.Candidate != nil {
			if item.Candidate.LatencyMillis > 0 {
				candLatencies = append(candLatencies, item.Candidate.LatencyMillis)
			}
			if item.Candidate.TokensUsed > 0 {
				candTokens += item.Candidate.TokensUsed
				candTokenCount++
			}
			if item.Candidate.LLMCalls > 0 {
				candLLMCalls += item.Candidate.LLMCalls
				candLLMCallCount++
			}
		}
	}
	report.BaselineAverageScores = averageScores(baseScores)
	report.CandidateAverageScores = averageScores(candScores)
	report.DeltaAverageScores = averageScoreDelta(report.CandidateAverageScores, report.BaselineAverageScores)
	report.BaselineMedianLatencyMs = medianInt64(baseLatencies)
	report.CandidateMedianLatencyMs = medianInt64(candLatencies)
	report.BaselineAverageTokens = averageInt64(baseTokens, baseTokenCount)
	report.CandidateAverageTokens = averageInt64(candTokens, candTokenCount)
	report.BaselineAverageLLMCalls = averageInt(baseLLMCalls, baseLLMCallCount)
	report.CandidateAverageLLMCalls = averageInt(candLLMCalls, candLLMCallCount)
}

func normalizeRunResult(result *RunResult) *RunResult {
	if result == nil {
		return nil
	}
	if result.LatencyMillis == 0 && result.Latency > 0 {
		result.LatencyMillis = result.Latency.Milliseconds()
	}
	return result
}

func averageScores(scores []DiagScores) DiagScoreAverages {
	if len(scores) == 0 {
		return DiagScoreAverages{}
	}
	var out DiagScoreAverages
	for _, item := range scores {
		out.Correctness += float64(item.Correctness)
		out.Completeness += float64(item.Completeness)
		out.Coherence += float64(item.Coherence)
		out.Actionability += float64(item.Actionability)
		out.Overall += float64(item.Overall)
	}
	count := float64(len(scores))
	out.Correctness /= count
	out.Completeness /= count
	out.Coherence /= count
	out.Actionability /= count
	out.Overall /= count
	return out
}

func medianInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func averageInt64(total int64, count int) float64 {
	if count <= 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func averageInt(total int, count int) float64 {
	if count <= 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func hasScores(scores DiagScores) bool {
	return scores.Correctness != 0 || scores.Completeness != 0 || scores.Coherence != 0 || scores.Actionability != 0 || scores.Overall != 0
}

func averageScoreDelta(candidate, baseline DiagScoreAverages) DiagScoreAverages {
	return DiagScoreAverages{
		Correctness:   candidate.Correctness - baseline.Correctness,
		Completeness:  candidate.Completeness - baseline.Completeness,
		Coherence:     candidate.Coherence - baseline.Coherence,
		Actionability: candidate.Actionability - baseline.Actionability,
		Overall:       candidate.Overall - baseline.Overall,
	}
}

func writeScoreRow(b *strings.Builder, name string, baseline, candidate, delta float64) {
	b.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %+.2f |\n", name, baseline, candidate, delta))
}
