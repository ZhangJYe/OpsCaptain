package eval

import (
	"context"
	"testing"
)

type stubSearcher struct {
	results map[string][]RetrievedDoc
}

func (s stubSearcher) Search(_ context.Context, query string, topK int) ([]RetrievedDoc, error) {
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
}
