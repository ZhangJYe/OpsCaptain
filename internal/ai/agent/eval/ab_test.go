package eval

import (
	"context"
	"strings"
	"testing"
	"time"
)

type staticRunner struct {
	label       string
	latency     time.Duration
	tokens      int64
	llmCalls    int
	intent      string
	domains     []string
	summaryText string
}

func (r staticRunner) Run(_ context.Context, query string) (*RunResult, error) {
	return &RunResult{
		Summary:    r.label + " " + query + " " + r.summaryText,
		Intent:     r.intent,
		Domains:    append([]string(nil), r.domains...),
		Status:     "succeeded",
		Latency:    r.latency,
		TokensUsed: r.tokens,
		LLMCalls:   r.llmCalls,
	}, nil
}

func TestRunABSummarizesScoresAndCosts(t *testing.T) {
	cases := []DiagCase{
		{ID: "ab-001", Query: "paymentservice CPU 高"},
		{ID: "ab-002", Query: "checkout 504"},
	}
	baseline := staticRunner{
		label:       "baseline",
		latency:     100 * time.Millisecond,
		tokens:      1000,
		llmCalls:    1,
		summaryText: "建议较少",
	}
	candidate := staticRunner{
		label:       "candidate",
		latency:     200 * time.Millisecond,
		tokens:      1600,
		llmCalls:    2,
		summaryText: "包含明确步骤",
	}
	judge := JudgeFunc(func(_ context.Context, _ string, report string) (DiagScores, error) {
		if strings.Contains(report, "candidate") {
			return DiagScores{Correctness: 5, Completeness: 4, Coherence: 4, Actionability: 5, Overall: 5}, nil
		}
		return DiagScores{Correctness: 3, Completeness: 3, Coherence: 3, Actionability: 2, Overall: 3}, nil
	})

	report, err := RunAB(context.Background(), cases, baseline, candidate, judge)
	if err != nil {
		t.Fatalf("run ab: %v", err)
	}
	if report.Cases != 2 || len(report.Results) != 2 {
		t.Fatalf("unexpected report shape: %#v", report)
	}
	if report.DeltaAverageScores.Overall != 2 || report.DeltaAverageScores.Actionability != 3 {
		t.Fatalf("unexpected score delta: %#v", report.DeltaAverageScores)
	}
	if report.BaselineMedianLatencyMs != 100 || report.CandidateMedianLatencyMs != 200 {
		t.Fatalf("unexpected latency summary: %#v", report)
	}
	if report.BaselineAverageTokens != 1000 || report.CandidateAverageLLMCalls != 2 {
		t.Fatalf("unexpected cost summary: %#v", report)
	}
	if markdown := report.Markdown(); !strings.Contains(markdown, "A/B 对比报告") || !strings.Contains(markdown, "综合分") {
		t.Fatalf("unexpected markdown report: %s", markdown)
	}
}

func TestCompareRunsReturnsJudgeResults(t *testing.T) {
	cases := []DiagCase{{ID: "cmp-001", Query: "paymentservice CPU 高"}}
	judge := JudgeFunc(func(_ context.Context, _ string, report string) (DiagScores, error) {
		if strings.Contains(report, "candidate") {
			return DiagScores{Correctness: 4, Completeness: 4, Coherence: 4, Actionability: 4, Overall: 4}, nil
		}
		return DiagScores{Correctness: 3, Completeness: 3, Coherence: 3, Actionability: 3, Overall: 3}, nil
	})

	results, err := CompareRuns(
		context.Background(),
		cases,
		staticRunner{label: "baseline"},
		staticRunner{label: "candidate"},
		judge,
	)
	if err != nil {
		t.Fatalf("compare runs: %v", err)
	}
	if len(results) != 1 || results[0].Delta.Overall != 1 {
		t.Fatalf("unexpected compare results: %#v", results)
	}
}
