package eval

import (
	"context"
	"testing"
)

func TestEvaluateRouting(t *testing.T) {
	cases := []DiagCase{
		{
			ID:              "route-001",
			Query:           "请查询知识库 SOP",
			ExpectedIntent:  "kb_qa",
			ExpectedDomains: []string{"knowledge"},
		},
		{
			ID:              "route-002",
			Query:           "pod 一直重启",
			ExpectedIntent:  "incident_analysis",
			ExpectedDomains: []string{"logs", "knowledge"},
		},
	}
	report, err := EvaluateRouting(context.Background(), cases, NewTriageRunner())
	if err != nil {
		t.Fatalf("evaluate routing: %v", err)
	}
	if report.Summary.Cases != 2 {
		t.Fatalf("expected 2 cases, got %#v", report.Summary)
	}
	if report.Summary.IntentCorrect != 1 {
		t.Fatalf("expected one intent hit, got %#v", report.Summary)
	}
	if report.Summary.FallbackCount != 1 {
		t.Fatalf("expected one fallback, got %#v", report.Summary)
	}
	if report.Results[0].DomainRecall != 1 {
		t.Fatalf("expected first case full domain recall, got %#v", report.Results[0])
	}
	if !report.Results[1].Fallback {
		t.Fatalf("expected second case fallback, got %#v", report.Results[1])
	}
}

func TestDomainScore(t *testing.T) {
	precision, recall, f1 := domainScore([]string{"logs", "knowledge"}, []string{"metrics", "logs", "knowledge"})
	if precision != 2.0/3.0 || recall != 1 || f1 <= 0 {
		t.Fatalf("unexpected domain score precision=%.2f recall=%.2f f1=%.2f", precision, recall, f1)
	}
	precision, recall, f1 = domainScore(nil, nil)
	if precision != 1 || recall != 1 || f1 != 1 {
		t.Fatalf("expected empty-empty score to be perfect")
	}
}
