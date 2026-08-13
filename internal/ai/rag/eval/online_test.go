package eval

import (
	"context"
	"fmt"
	"testing"
)

func TestRunQueryEvalReportsFailureRateAndLatencyPercentiles(t *testing.T) {
	latencies := map[string]int64{"q1": 10, "q2": 20, "q3": 30, "q4": 100}
	exec := func(_ context.Context, query string) ([]RetrievedDoc, QueryMetrics, error) {
		if query == "fail" {
			return nil, QueryMetrics{TotalLatencyMs: 5}, fmt.Errorf("timeout")
		}
		latency := latencies[query]
		return []RetrievedDoc{{ID: "doc"}}, QueryMetrics{
			RetrieveLatencyMs: latency - 2,
			DenseLatencyMs:    latency - 3,
			LexicalLatencyMs:  1,
			FusionLatencyMs:   1,
			RewriteAttempted:  true,
			RewriteApplied:    query != "q2",
			RewriteDegraded:   query == "q2",
			RerankAttempted:   true,
			RerankApplied:     query != "q3",
			RerankDegraded:    query == "q3",
			TotalLatencyMs:    latency,
		}, nil
	}
	cases := []EvalCase{
		{ID: "1", Query: "q1", RelevantIDs: []string{"doc"}},
		{ID: "2", Query: "q2", RelevantIDs: []string{"doc"}},
		{ID: "3", Query: "q3", RelevantIDs: []string{"doc"}},
		{ID: "4", Query: "q4", RelevantIDs: []string{"doc"}},
		{ID: "5", Query: "fail", RelevantIDs: []string{"doc"}},
	}

	summary, results, err := RunQueryEvalWithOpts(context.Background(), exec, cases, []int{1}, RunOptions{ContinueOnError: true})
	if err != nil {
		t.Fatalf("RunQueryEvalWithOpts returned error: %v", err)
	}
	if len(results) != 4 || summary.Succeeded != 4 || summary.Failed != 1 {
		t.Fatalf("unexpected result counts: results=%d succeeded=%d failed=%d", len(results), summary.Succeeded, summary.Failed)
	}
	if summary.FailureRate != 0.2 {
		t.Fatalf("failure rate = %v, want 0.2", summary.FailureRate)
	}
	if summary.Latency.Total.AvgMs != 40 || summary.Latency.Total.P50Ms != 20 || summary.Latency.Total.P95Ms != 100 {
		t.Fatalf("unexpected total latency stats: %+v", summary.Latency.Total)
	}
	if summary.AvgRecallAtK[1] != 1 {
		t.Fatalf("quality metrics must use successful cases, got recall@1=%v", summary.AvgRecallAtK[1])
	}
	if summary.RewriteAttempted != 4 || summary.RewriteApplied != 3 || summary.RewriteDegraded != 1 || summary.RewriteDegradedRate != 0.25 {
		t.Fatalf("unexpected rewrite outcome summary: %+v", summary)
	}
	if summary.RerankAttempted != 4 || summary.RerankApplied != 3 || summary.RerankDegraded != 1 || summary.RerankDegradedRate != 0.25 {
		t.Fatalf("unexpected rerank outcome summary: %+v", summary)
	}
}

func TestRunQueryEvalFailureRateWhenAllQueriesFail(t *testing.T) {
	exec := func(context.Context, string) ([]RetrievedDoc, QueryMetrics, error) {
		return nil, QueryMetrics{}, fmt.Errorf("unavailable")
	}
	summary, _, err := RunQueryEvalWithOpts(
		context.Background(),
		exec,
		[]EvalCase{{ID: "1", Query: "q", RelevantIDs: []string{"doc"}}},
		[]int{1},
		RunOptions{ContinueOnError: true},
	)
	if err != nil {
		t.Fatalf("RunQueryEvalWithOpts returned error: %v", err)
	}
	if summary.FailureRate != 1 || summary.Succeeded != 0 {
		t.Fatalf("unexpected all-failed summary: %+v", summary)
	}
}
