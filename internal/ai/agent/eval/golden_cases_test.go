package eval

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type fixtureRunner struct {
	byQuery map[string]*RunResult
}

func (r fixtureRunner) Run(_ context.Context, query string) (*RunResult, error) {
	result, ok := r.byQuery[query]
	if !ok {
		return nil, fmt.Errorf("missing fixture for query %q", query)
	}
	return result, nil
}

func TestDiagGoldenCases(t *testing.T) {
	cases, err := LoadDiagCasesJSONL("testdata/diag_golden.jsonl")
	if err != nil {
		t.Fatalf("load golden cases: %v", err)
	}
	if len(cases) != 20 {
		t.Fatalf("expected 20 golden cases, got %d", len(cases))
	}

	runner := fixtureRunner{byQuery: map[string]*RunResult{}}
	for _, tc := range cases {
		runner.byQuery[tc.Query] = &RunResult{
			Summary: buildFixtureSummary(tc),
			Intent:  tc.ExpectedIntent,
			Domains: append([]string(nil), tc.ExpectedDomains...),
			Status:  "succeeded",
		}
	}

	summary, results, err := RunGoldenCases(context.Background(), cases, runner)
	if err != nil {
		t.Fatalf("run golden cases: %v", err)
	}
	if summary.Failed != 0 || summary.Passed != len(cases) {
		t.Fatalf("expected all cases to pass, summary=%#v results=%#v", summary, results)
	}
}

func TestRunGoldenCasesReportsFailures(t *testing.T) {
	cases := []DiagCase{
		{
			ID:              "bad-001",
			Query:           "paymentservice CPU 使用率 95%",
			ExpectedIntent:  "alert_analysis",
			ExpectedDomains: []string{"metrics", "logs"},
			MustMention:     []string{"CPU"},
			MustNotMention:  []string{"天气"},
			ExpectedAction:  "检查 resource limits",
		},
	}
	runner := fixtureRunner{byQuery: map[string]*RunResult{
		cases[0].Query: {
			Summary: "天气不错",
			Intent:  "kb_qa",
			Domains: []string{"knowledge"},
		},
	}}

	summary, results, err := RunGoldenCases(context.Background(), cases, runner)
	if err != nil {
		t.Fatalf("run golden cases: %v", err)
	}
	if summary.Failed != 1 || results[0].Passed {
		t.Fatalf("expected failure, summary=%#v result=%#v", summary, results[0])
	}
	joined := strings.Join(results[0].Failures, "\n")
	for _, want := range []string{"intent expected", "domains expected", "summary must mention", "summary must not mention", "expected action"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected failure containing %q, got %s", want, joined)
		}
	}
}

func buildFixtureSummary(tc DiagCase) string {
	parts := make([]string, 0, len(tc.MustMention)+2)
	parts = append(parts, "诊断结果")
	parts = append(parts, tc.MustMention...)
	if tc.ExpectedAction != "" {
		parts = append(parts, tc.ExpectedAction)
	}
	return strings.Join(parts, "；")
}
