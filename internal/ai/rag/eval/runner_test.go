package eval

import (
	"context"
	"fmt"
	"testing"
)

type stubSearcher struct {
	results map[string][]RetrievedDoc
	err     error
}

func (s stubSearcher) Search(_ context.Context, query string, topK int) ([]RetrievedDoc, error) {
	if s.err != nil {
		return nil, s.err
	}
	docs := append([]RetrievedDoc(nil), s.results[query]...)
	if topK > 0 && topK < len(docs) {
		docs = docs[:topK]
	}
	return docs, nil
}

func TestRunComputesRecallAtK(t *testing.T) {
	searcher := stubSearcher{
		results: map[string][]RetrievedDoc{
			"case-1": {
				{ID: "doc-1"},
				{ID: "doc-2"},
			},
			"case-2": {
				{ID: "doc-3"},
				{ID: "doc-4"},
				{ID: "doc-5"},
			},
		},
	}
	cases := []EvalCase{
		{ID: "case-1", Query: "case-1", RelevantIDs: []string{"doc-1"}},
		{ID: "case-2", Query: "case-2", RelevantIDs: []string{"doc-4", "doc-5"}},
	}

	summary, results, err := Run(context.Background(), searcher, cases, []int{1, 3})
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if got := results[1].RecallAtK[1]; got != 0 {
		t.Fatalf("expected second case recall@1 = 0, got %v", got)
	}
	if got := results[1].RecallAtK[3]; got != 1 {
		t.Fatalf("expected second case recall@3 = 1, got %v", got)
	}
	if got := summary.AvgRecallAtK[1]; got != 0.5 {
		t.Fatalf("expected avg recall@1 = 0.5, got %v", got)
	}
	if got := summary.AvgRecallAtK[3]; got != 1 {
		t.Fatalf("expected avg recall@3 = 1, got %v", got)
	}
	if summary.Succeeded != 2 {
		t.Fatalf("expected succeeded = 2, got %d", summary.Succeeded)
	}
	if summary.Failed != 0 {
		t.Fatalf("expected failed = 0, got %d", summary.Failed)
	}
}

func TestRunEmptyCases(t *testing.T) {
	searcher := stubSearcher{results: map[string][]RetrievedDoc{}}
	summary, results, err := Run(context.Background(), searcher, nil, []int{1, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
	if summary.Cases != 0 {
		t.Fatalf("expected cases = 0, got %d", summary.Cases)
	}
	if summary.Succeeded != 0 {
		t.Fatalf("expected succeeded = 0, got %d", summary.Succeeded)
	}
}

func TestRunNilSearcher(t *testing.T) {
	_, _, err := Run(context.Background(), nil, nil, []int{1})
	if err == nil {
		t.Fatal("expected error for nil searcher")
	}
}

func TestRunEmptyKs(t *testing.T) {
	searcher := stubSearcher{results: map[string][]RetrievedDoc{}}
	_, _, err := Run(context.Background(), searcher, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty ks")
	}
}

func TestRunDuplicateAndNegativeKs(t *testing.T) {
	searcher := stubSearcher{
		results: map[string][]RetrievedDoc{
			"c1": {{ID: "d1"}, {ID: "d2"}},
		},
	}
	cases := []EvalCase{
		{ID: "c1", Query: "c1", RelevantIDs: []string{"d1"}},
	}
	summary, results, err := Run(context.Background(), searcher, cases, []int{3, 1, -1, 0, 1, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if _, ok := summary.AvgRecallAtK[1]; !ok {
		t.Fatal("expected @1 in summary")
	}
	if _, ok := summary.AvgRecallAtK[3]; !ok {
		t.Fatal("expected @3 in summary")
	}
	if _, ok := summary.AvgRecallAtK[-1]; ok {
		t.Fatal("should not have @-1 in summary")
	}
}

func TestRunEmptyRelevantIDs(t *testing.T) {
	searcher := stubSearcher{
		results: map[string][]RetrievedDoc{
			"c1": {{ID: "d1"}, {ID: "d2"}},
		},
	}
	cases := []EvalCase{
		{ID: "c1", Query: "c1", RelevantIDs: nil},
	}
	summary, results, err := Run(context.Background(), searcher, cases, []int{1, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].RecallAtK[1] != 0 {
		t.Fatalf("expected recall@1 = 0 for empty relevant, got %v", results[0].RecallAtK[1])
	}
	if summary.HitRateAtK[1] != 0 {
		t.Fatalf("expected hit rate = 0, got %v", summary.HitRateAtK[1])
	}
}

func TestRunDuplicateRankedIDs(t *testing.T) {
	searcher := stubSearcher{
		results: map[string][]RetrievedDoc{
			"c1": {{ID: "d1"}, {ID: "d1"}, {ID: "d2"}},
		},
	}
	cases := []EvalCase{
		{ID: "c1", Query: "c1", RelevantIDs: []string{"d1", "d2"}},
	}
	summary, results, err := Run(context.Background(), searcher, cases, []int{2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hits := results[0].HitIDsByK[2]
	if len(hits) != 1 {
		t.Fatalf("expected 1 unique hit in top-2 (d1 deduped), got %d: %v", len(hits), hits)
	}
	if summary.FullRecallAtK[2] != 0 {
		t.Fatalf("should not be full recall with deduped hits")
	}
}

func TestRunSearcherErrorFailFast(t *testing.T) {
	searcher := stubSearcher{err: fmt.Errorf("connection refused")}
	cases := []EvalCase{
		{ID: "c1", Query: "q1", RelevantIDs: []string{"d1"}},
	}
	_, _, err := Run(context.Background(), searcher, cases, []int{1})
	if err == nil {
		t.Fatal("expected error from searcher")
	}
}

func TestRunSearcherErrorContinueOnError(t *testing.T) {
	searcher := stubSearcher{
		err: fmt.Errorf("timeout"),
	}
	cases := []EvalCase{
		{ID: "c1", Query: "q1", RelevantIDs: []string{"d1"}},
	}
	summary, results, err := RunWithOpts(context.Background(), searcher, cases, []int{1}, RunOptions{ContinueOnError: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
	if summary.Failed != 1 {
		t.Fatalf("expected failed = 1, got %d", summary.Failed)
	}
	if summary.Succeeded != 0 {
		t.Fatalf("expected succeeded = 0, got %d", summary.Succeeded)
	}
	if len(summary.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(summary.Failures))
	}
	if summary.Failures[0].CaseID != "c1" {
		t.Fatalf("expected failure case_id = c1, got %s", summary.Failures[0].CaseID)
	}
}

func TestRunContinueOnErrorPartialSuccess(t *testing.T) {
	failSearcher := stubSearcher{err: fmt.Errorf("boom")}
	passSearcher := stubSearcher{
		results: map[string][]RetrievedDoc{
			"q2": {{ID: "d2"}},
		},
	}

	type multiSearcher struct {
		pass stubSearcher
		fail stubSearcher
	}
	ms := struct {
		pass stubSearcher
		fail stubSearcher
	}{pass: passSearcher, fail: failSearcher}

	searcher := &conditionalSearcher{
		failQueries: map[string]bool{"q1": true},
		passResults: passSearcher.results,
	}

	cases := []EvalCase{
		{ID: "c1", Query: "q1", RelevantIDs: []string{"d1"}},
		{ID: "c2", Query: "q2", RelevantIDs: []string{"d2"}},
	}
	summary, results, err := RunWithOpts(context.Background(), searcher, cases, []int{1}, RunOptions{ContinueOnError: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if summary.Failed != 1 {
		t.Fatalf("expected failed = 1, got %d", summary.Failed)
	}
	if summary.Succeeded != 1 {
		t.Fatalf("expected succeeded = 1, got %d", summary.Succeeded)
	}
	if summary.AvgRecallAtK[1] != 1.0 {
		t.Fatalf("expected avg recall@1 = 1.0 (only succeeded case), got %v", summary.AvgRecallAtK[1])
	}

	_ = ms
}

func TestRunHitRateAndFullRecall(t *testing.T) {
	searcher := stubSearcher{
		results: map[string][]RetrievedDoc{
			"q1": {{ID: "d1"}, {ID: "d2"}, {ID: "d3"}},
			"q2": {{ID: "d4"}, {ID: "x1"}},
			"q3": {{ID: "x2"}, {ID: "x3"}},
		},
	}
	cases := []EvalCase{
		{ID: "c1", Query: "q1", RelevantIDs: []string{"d1", "d2", "d3"}},
		{ID: "c2", Query: "q2", RelevantIDs: []string{"d4"}},
		{ID: "c3", Query: "q3", RelevantIDs: []string{"d5"}},
	}
	summary, _, err := Run(context.Background(), searcher, cases, []int{1, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.HitRateAtK[1] != 2.0/3.0 {
		t.Fatalf("expected hit rate @1 = 0.667, got %v", summary.HitRateAtK[1])
	}
	if summary.FullRecallAtK[1] != 1 {
		t.Fatalf("expected full recall @1 = 1, got %d", summary.FullRecallAtK[1])
	}
	if summary.FullRecallAtK[3] != 2 {
		t.Fatalf("expected full recall @3 = 2, got %d", summary.FullRecallAtK[3])
	}
}

func TestNormalizeKs(t *testing.T) {
	got := normalizeKs([]int{5, 1, 3, 1, -1, 0, 3})
	want := []int{1, 3, 5}
	if len(got) != len(want) {
		t.Fatalf("expected len %d, got %d", len(want), len(got))
	}
	for i, v := range got {
		if v != want[i] {
			t.Fatalf("expected [%d] = %d, got %d", i, want[i], v)
		}
	}
}

func TestComputeRecall(t *testing.T) {
	if r := computeRecall([]string{"a", "b"}, []string{"a"}); r != 0.5 {
		t.Fatalf("expected 0.5, got %v", r)
	}
	if r := computeRecall(nil, nil); r != 0 {
		t.Fatalf("expected 0 for nil relevant, got %v", r)
	}
	if r := computeRecall([]string{}, nil); r != 0 {
		t.Fatalf("expected 0 for empty relevant, got %v", r)
	}
	if r := computeRecall([]string{"a"}, []string{"a"}); r != 1.0 {
		t.Fatalf("expected 1.0, got %v", r)
	}
}

type conditionalSearcher struct {
	failQueries map[string]bool
	passResults map[string][]RetrievedDoc
}

func (s *conditionalSearcher) Search(_ context.Context, query string, topK int) ([]RetrievedDoc, error) {
	if s.failQueries[query] {
		return nil, fmt.Errorf("simulated failure for %s", query)
	}
	docs := append([]RetrievedDoc(nil), s.passResults[query]...)
	if topK > 0 && topK < len(docs) {
		docs = docs[:topK]
	}
	return docs, nil
}

func TestRunComputesMRR(t *testing.T) {
	searcher := stubSearcher{
		results: map[string][]RetrievedDoc{
			"q1": {{ID: "d1"}, {ID: "d2"}},
			"q2": {{ID: "x1"}, {ID: "d3"}},
			"q3": {{ID: "x2"}, {ID: "x3"}, {ID: "d4"}},
		},
	}
	cases := []EvalCase{
		{ID: "c1", Query: "q1", RelevantIDs: []string{"d1"}},
		{ID: "c2", Query: "q2", RelevantIDs: []string{"d3"}},
		{ID: "c3", Query: "q3", RelevantIDs: []string{"d4"}},
	}

	summary, _, err := Run(context.Background(), searcher, cases, []int{3})
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	// c1: rank 1 -> RR=1.0, c2: rank 2 -> RR=0.5, c3: rank 3 -> RR=0.333
	expectedMRR := (1.0 + 0.5 + 1.0/3.0) / 3.0
	if diff := summary.MRR - expectedMRR; diff > 0.001 || diff < -0.001 {
		t.Fatalf("expected MRR ~%.3f, got %.3f", expectedMRR, summary.MRR)
	}
}

func TestReciprocalRank(t *testing.T) {
	if rr := reciprocalRank([]string{"a"}, []string{"b", "a"}); rr != 0.5 {
		t.Fatalf("expected 0.5, got %v", rr)
	}
	if rr := reciprocalRank([]string{"a"}, []string{"a", "b"}); rr != 1.0 {
		t.Fatalf("expected 1.0, got %v", rr)
	}
	if rr := reciprocalRank([]string{"a"}, []string{"b", "c"}); rr != 0 {
		t.Fatalf("expected 0, got %v", rr)
	}
	if rr := reciprocalRank(nil, []string{"a"}); rr != 0 {
		t.Fatalf("expected 0 for nil relevant, got %v", rr)
	}
}
