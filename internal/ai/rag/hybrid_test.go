package rag

import (
	"context"
	"testing"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

func TestRRFFusion_MergesAndRanks(t *testing.T) {
	t.Parallel()

	denseDocs := []*schema.Document{
		{ID: "dense-1", Content: "a", MetaData: map[string]any{"case_id": "case-001"}},
		{ID: "dense-2", Content: "b", MetaData: map[string]any{"case_id": "case-002"}},
		{ID: "dense-3", Content: "c", MetaData: map[string]any{"case_id": "case-003"}},
	}

	lexHits := []BM25Hit{
		{DocID: "case-002", Score: 5.0},
		{DocID: "case-004", Score: 3.0},
		{DocID: "case-001", Score: 1.0},
	}

	fused := rrfFusion(denseDocs, lexHits, 60)

	if len(fused) != 4 {
		t.Fatalf("expected 4 fused docs (3 dense + 1 lex-only), got %d", len(fused))
	}

	firstID := docFusionKey(fused[0].doc)
	if firstID != "case-001" && firstID != "case-002" {
		t.Fatalf("expected case-001 or case-002 to rank high (both channels), got %s", firstID)
	}

	var bothCount int
	for _, f := range fused {
		if f.denseRank > 0 && f.lexRank > 0 {
			bothCount++
		}
	}
	if bothCount != 2 {
		t.Fatalf("expected 2 docs from both channels, got %d", bothCount)
	}
}

func TestRRFFusion_EmptyInputs(t *testing.T) {
	t.Parallel()

	fused := rrfFusion(nil, nil, 60)
	if len(fused) != 0 {
		t.Fatalf("expected empty fusion, got %d", len(fused))
	}

	fused = rrfFusion([]*schema.Document{{ID: "a", MetaData: map[string]any{"case_id": "x"}}}, nil, 60)
	if len(fused) != 1 {
		t.Fatalf("expected 1 from dense-only, got %d", len(fused))
	}
}

func TestBuildBM25IndexFromDocs(t *testing.T) {
	t.Parallel()

	docs := []*schema.Document{
		{
			ID:      "doc-1",
			Content: "checkoutservice rrt timeout spike",
			MetaData: map[string]any{
				"case_id":      "case-001",
				"service":      "checkoutservice",
				"metric_names": []any{"rrt", "timeout"},
			},
		},
		{
			ID:      "doc-2",
			Content: "frontend latency high",
			MetaData: map[string]any{
				"case_id": "case-002",
				"service": "frontend",
			},
		},
	}

	idx := BuildBM25IndexFromDocs(docs)
	if idx.Size() != 2 {
		t.Fatalf("expected 2 docs indexed, got %d", idx.Size())
	}

	hits := idx.Search("checkoutservice rrt", 5)
	if len(hits) == 0 {
		t.Fatal("expected at least 1 hit")
	}
	if hits[0].DocID != "case-001" {
		t.Fatalf("expected case-001 to rank first, got %s", hits[0].DocID)
	}
}

func TestHybridRetrieve_Integration(t *testing.T) {
	t.Parallel()

	denseResults := []*schema.Document{
		{ID: "d1", Content: "semantically similar but no exact match", MetaData: map[string]any{"case_id": "case-010"}},
		{ID: "d2", Content: "checkoutservice payment timeout", MetaData: map[string]any{"case_id": "case-020", "service": "checkoutservice"}},
	}

	fr := &fakeHybridRetriever{docs: denseResults}
	pool := NewRetrieverPool(
		func(ctx context.Context) (retrieverapi.Retriever, error) { return fr, nil },
		func(ctx context.Context) string { return "test-hybrid" },
		nil,
	)

	lexIdx := NewBM25Index()
	lexIdx.AddDocument("case-020", "checkoutservice rrt timeout with paymentservice", map[string]string{
		"service":     "checkoutservice",
		"destination": "paymentservice",
	})
	lexIdx.AddDocument("case-030", "checkoutservice memory leak detected", map[string]string{
		"service": "checkoutservice",
	})
	lexIdx.AddDocument("case-040", "emailservice smtp connection error", map[string]string{
		"service": "emailservice",
	})

	cfg := HybridConfig{
		DenseTopK:            10,
		LexicalTopK:          10,
		FusionK:              60,
		FinalTopK:            5,
		MetadataBoostEnabled: true,
	}

	docs, trace, err := HybridRetrieve(context.Background(), pool, lexIdx, "checkoutservice rrt timeout paymentservice", cfg)
	if err != nil {
		t.Fatalf("HybridRetrieve returned error: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected at least one result")
	}
	if trace.DenseCount != 2 {
		t.Fatalf("expected dense count 2, got %d", trace.DenseCount)
	}
	if trace.LexicalCount == 0 {
		t.Fatal("expected non-zero lexical count")
	}
	if trace.FusedCount == 0 {
		t.Fatal("expected non-zero fused count")
	}

	firstID := docFusionKey(docs[0])
	if firstID != "case-020" {
		t.Fatalf("expected case-020 (both channels + metadata match) to rank first, got %s", firstID)
	}
}

func TestParseQueryMode_Hybrid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want QueryMode
	}{
		{raw: "hybrid", want: QueryModeHybrid},
		{raw: "hybrid_retrieval", want: QueryModeHybrid},
		{raw: "hybrid-retrieval", want: QueryModeHybrid},
	}
	for _, tt := range tests {
		got, err := ParseQueryMode(tt.raw)
		if err != nil {
			t.Fatalf("ParseQueryMode(%q) returned error: %v", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("ParseQueryMode(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestQueryWithMode_HybridWithPopulatedSharedBM25(t *testing.T) {
	ResetSharedBM25Index()
	defer ResetSharedBM25Index()

	idx := SharedBM25Index()
	idx.AddDocument("case-A", "checkoutservice rrt timeout paymentservice downstream", map[string]string{
		"service":     "checkoutservice",
		"destination": "paymentservice",
	})
	idx.AddDocument("case-B", "emailservice smtp connection refused", map[string]string{
		"service": "emailservice",
	})

	denseResults := []*schema.Document{
		{ID: "d1", Content: "generic anomaly", MetaData: map[string]any{"case_id": "case-C"}},
		{ID: "d2", Content: "checkoutservice issue", MetaData: map[string]any{"case_id": "case-A", "service": "checkoutservice"}},
	}
	fr := &fakeHybridRetriever{docs: denseResults}
	pool := NewRetrieverPool(
		func(ctx context.Context) (retrieverapi.Retriever, error) { return fr, nil },
		func(ctx context.Context) string { return "test-hybrid-shared" },
		nil,
	)

	origSharedPool := sharedPool
	sharedPool = pool
	defer func() { sharedPool = origSharedPool }()

	docs, trace, err := QueryWithMode(context.Background(), pool, "checkoutservice rrt timeout", QueryModeHybrid)
	if err != nil {
		t.Fatalf("QueryWithMode hybrid returned error: %v", err)
	}
	if trace.Mode != string(QueryModeHybrid) {
		t.Fatalf("expected mode hybrid, got %s", trace.Mode)
	}
	if trace.Hybrid == nil {
		t.Fatal("expected hybrid trace to be non-nil")
	}
	if trace.Hybrid.LexicalCount == 0 {
		t.Fatal("expected lexical hits from shared BM25 index, got 0")
	}
	if trace.Hybrid.BothHits == 0 && trace.Hybrid.LexicalOnlyHits == 0 {
		t.Fatal("expected at least one hit from lexical channel")
	}
	if len(docs) == 0 {
		t.Fatal("expected at least one result")
	}
}

func TestQueryWithMode_HybridWithEmptySharedBM25FallsBackToDenseOnly(t *testing.T) {
	ResetSharedBM25Index()
	defer ResetSharedBM25Index()

	denseResults := []*schema.Document{
		{ID: "d1", Content: "some result", MetaData: map[string]any{"case_id": "case-X"}},
	}
	fr := &fakeHybridRetriever{docs: denseResults}
	pool := NewRetrieverPool(
		func(ctx context.Context) (retrieverapi.Retriever, error) { return fr, nil },
		func(ctx context.Context) string { return "test-hybrid-empty" },
		nil,
	)

	docs, trace, err := QueryWithMode(context.Background(), pool, "cpu high", QueryModeHybrid)
	if err != nil {
		t.Fatalf("QueryWithMode hybrid returned error: %v", err)
	}
	if trace.Hybrid == nil {
		t.Fatal("expected hybrid trace")
	}
	if trace.Hybrid.LexicalCount != 0 {
		t.Fatalf("expected 0 lexical hits from empty BM25, got %d", trace.Hybrid.LexicalCount)
	}
	if trace.Hybrid.DenseCount != 1 {
		t.Fatalf("expected 1 dense hit, got %d", trace.Hybrid.DenseCount)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc (dense-only fallback), got %d", len(docs))
	}
}

type fakeHybridRetriever struct {
	docs []*schema.Document
}

func (f *fakeHybridRetriever) Retrieve(ctx context.Context, query string, opts ...retrieverapi.Option) ([]*schema.Document, error) {
	return f.docs, nil
}
