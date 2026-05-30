package rag

import (
	"context"
	"testing"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type fakeQueryRetriever struct {
	queries      []string
	requestedTop []int
	docs         []*schema.Document
}

func (f *fakeQueryRetriever) Retrieve(ctx context.Context, query string, opts ...retrieverapi.Option) ([]*schema.Document, error) {
	f.queries = append(f.queries, query)
	options := retrieverapi.GetCommonOptions(&retrieverapi.Options{}, opts...)
	requestedTop := 0
	if options.TopK != nil {
		requestedTop = *options.TopK
	}
	f.requestedTop = append(f.requestedTop, requestedTop)
	return f.docs, nil
}

func TestQueryForEval_RetrieveOnlySkipsRewriteAndRerank(t *testing.T) {
	t.Parallel()

	retriever := &fakeQueryRetriever{docs: []*schema.Document{{ID: "doc-1", Content: "hello"}}}
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) { return retriever, nil },
		func(context.Context) string { return "test" },
		nil,
	)

	docs, trace, err := QueryForEval(
		context.Background(),
		pool,
		"cpu high",
		false, // wantRewrite
		false, // wantRerank
	)
	if err != nil {
		t.Fatalf("QueryForEval returned error: %v", err)
	}
	if trace.RewriteLatencyMs != 0 {
		t.Fatalf("expected rewrite to be skipped, got latency %dms", trace.RewriteLatencyMs)
	}
	if trace.RerankLatencyMs != 0 {
		t.Fatalf("expected rerank to be skipped, got latency %dms", trace.RerankLatencyMs)
	}
	if len(retriever.queries) != 1 || retriever.queries[0] != "cpu high" {
		t.Fatalf("expected retrieve query to use original query, got %#v", retriever.queries)
	}
	if trace.RewrittenQuery != "cpu high" {
		t.Fatalf("expected rewritten query to stay original, got %q", trace.RewrittenQuery)
	}
	if trace.RerankEnabled {
		t.Fatalf("expected rerank disabled trace")
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
}

func TestQuery_DefaultConfigDisablesRewriteAndRerank(t *testing.T) {
	ResetSharedBM25Index()
	defer ResetSharedBM25Index()

	idx := SharedBM25Index()
	idx.AddDocument("case-A", "checkoutservice rrt timeout", nil)
	idx.AddDocument("case-B", "frontend latency high", nil)
	idx.AddDocument("case-C", "emailservice smtp error", nil)

	retriever := &fakeQueryRetriever{docs: []*schema.Document{
		{ID: "d1", Content: "checkoutservice issue", MetaData: map[string]any{"case_id": "case-A"}},
		{ID: "d2", Content: "frontend issue", MetaData: map[string]any{"case_id": "case-B"}},
		{ID: "d3", Content: "email issue", MetaData: map[string]any{"case_id": "case-C"}},
	}}
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) { return retriever, nil },
		func(context.Context) string { return "test-default-query" },
		nil,
	)

	docs, trace, err := Query(context.Background(), pool, "checkoutservice timeout")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if trace.Mode != "hybrid" {
		t.Fatalf("expected mode hybrid, got %s", trace.Mode)
	}
	if trace.RerankEnabled {
		t.Fatal("expected RerankEnabled=false with default config")
	}
	if trace.RewriteLatencyMs != 0 {
		t.Fatalf("expected rewrite skipped, got latency %dms", trace.RewriteLatencyMs)
	}
	if trace.RerankLatencyMs != 0 {
		t.Fatalf("expected rerank skipped, got latency %dms", trace.RerankLatencyMs)
	}
	if trace.RewrittenQuery != "checkoutservice timeout" {
		t.Fatalf("expected rewritten query unchanged, got %q", trace.RewrittenQuery)
	}
	if len(docs) == 0 {
		t.Fatal("expected at least one result")
	}
	if trace.ResultCount != len(docs) {
		t.Fatalf("ResultCount %d != len(docs) %d", trace.ResultCount, len(docs))
	}
}

func TestRefineRetrievedDocs_PrefersMetadataAndLexicalOverlap(t *testing.T) {
	t.Parallel()

	docs := []*schema.Document{
		{
			ID:      "generic",
			Content: "payment latency spike with sparse details",
			MetaData: map[string]any{
				"service":        "paymentservice",
				"instance_type":  "service",
				"metric_names":   []any{"rrt"},
				"trace_services": []any{"paymentservice"},
			},
		},
		{
			ID:      "match",
			Content: "checkoutservice rrt timeout spike with paymentservice downstream failures",
			MetaData: map[string]any{
				"service":          "checkoutservice",
				"instance_type":    "service",
				"source":           "checkoutservice",
				"destination":      "paymentservice",
				"service_tokens":   []any{"checkoutservice", "paymentservice"},
				"metric_names":     []any{"rrt", "timeout"},
				"trace_operations": []any{"charge"},
			},
		},
	}

	ranked := refineRetrievedDocs("checkoutservice rrt timeout to paymentservice", docs)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(ranked))
	}
	if ranked[0].ID != "match" {
		t.Fatalf("expected metadata/lexical match to rank first, got %s", ranked[0].ID)
	}
}
