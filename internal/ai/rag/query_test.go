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

func TestParseQueryMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want QueryMode
	}{
		{raw: "", want: QueryModeRewriteRetrieveRerank},
		{raw: "retrieve", want: QueryModeRetrieveOnly},
		{raw: "retrieve_only", want: QueryModeRetrieveOnly},
		{raw: "rewrite", want: QueryModeRewriteRetrieve},
		{raw: "full", want: QueryModeRewriteRetrieveRerank},
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

	if _, err := ParseQueryMode("unknown"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestDefaultQueryModeDefaultsToRetrieve(t *testing.T) {
	t.Parallel()

	if got := DefaultQueryMode(context.Background()); got != QueryModeRetrieveOnly {
		t.Fatalf("expected default query mode retrieve, got %q", got)
	}
}

func TestQueryWithMode_RetrieveOnlySkipsRewriteAndRerank(t *testing.T) {
	t.Parallel()

	retriever := &fakeQueryRetriever{docs: []*schema.Document{{ID: "doc-1", Content: "hello"}}}
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) { return retriever, nil },
		func(context.Context) string { return "test" },
		nil,
	)

	rewriteCalls := 0
	rerankCalls := 0
	docs, trace, err := queryWithMode(
		context.Background(),
		pool,
		"cpu high",
		QueryModeRetrieveOnly,
		func(ctx context.Context, query string) string {
			rewriteCalls++
			return "rewritten"
		},
		func(ctx context.Context, query string, docs []*schema.Document, topK int) RerankResult {
			rerankCalls++
			return RerankResult{Docs: docs, Enabled: true}
		},
	)
	if err != nil {
		t.Fatalf("queryWithMode returned error: %v", err)
	}
	if rewriteCalls != 0 {
		t.Fatalf("expected rewrite to be skipped, got %d calls", rewriteCalls)
	}
	if rerankCalls != 0 {
		t.Fatalf("expected rerank to be skipped, got %d calls", rerankCalls)
	}
	if len(retriever.queries) != 1 || retriever.queries[0] != "cpu high" {
		t.Fatalf("expected retrieve query to use original query, got %#v", retriever.queries)
	}
	if len(retriever.requestedTop) != 1 || retriever.requestedTop[0] != RetrieverCandidateTopK(context.Background()) {
		t.Fatalf("expected expanded retrieve topK, got %#v", retriever.requestedTop)
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

func TestQueryWithMode_EmptyModeUsesDefaultRetrieve(t *testing.T) {
	t.Parallel()

	retriever := &fakeQueryRetriever{docs: []*schema.Document{{ID: "doc-1", Content: "hello"}}}
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) { return retriever, nil },
		func(context.Context) string { return "test" },
		nil,
	)

	rewriteCalls := 0
	rerankCalls := 0
	_, trace, err := queryWithMode(
		context.Background(),
		pool,
		"cpu high",
		"",
		func(ctx context.Context, query string) string {
			rewriteCalls++
			return "rewritten"
		},
		func(ctx context.Context, query string, docs []*schema.Document, topK int) RerankResult {
			rerankCalls++
			return RerankResult{Docs: docs, Enabled: true}
		},
	)
	if err != nil {
		t.Fatalf("queryWithMode returned error: %v", err)
	}
	if rewriteCalls != 0 || rerankCalls != 0 {
		t.Fatalf("expected default empty mode to skip rewrite/rerank, got rewrite=%d rerank=%d", rewriteCalls, rerankCalls)
	}
	if trace.Mode != string(QueryModeRetrieveOnly) {
		t.Fatalf("expected default trace mode retrieve, got %q", trace.Mode)
	}
}

func TestQueryWithMode_RewriteModeUsesRewrittenQueryWithoutRerank(t *testing.T) {
	t.Parallel()

	retriever := &fakeQueryRetriever{docs: []*schema.Document{{ID: "doc-1", Content: "hello"}}}
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) { return retriever, nil },
		func(context.Context) string { return "test" },
		nil,
	)

	rewriteCalls := 0
	rerankCalls := 0
	_, trace, err := queryWithMode(
		context.Background(),
		pool,
		"cpu high",
		QueryModeRewriteRetrieve,
		func(ctx context.Context, query string) string {
			rewriteCalls++
			return "cpu usage saturation"
		},
		func(ctx context.Context, query string, docs []*schema.Document, topK int) RerankResult {
			rerankCalls++
			return RerankResult{Docs: docs, Enabled: true}
		},
	)
	if err != nil {
		t.Fatalf("queryWithMode returned error: %v", err)
	}
	if rewriteCalls != 1 {
		t.Fatalf("expected rewrite once, got %d", rewriteCalls)
	}
	if rerankCalls != 0 {
		t.Fatalf("expected rerank to be skipped, got %d calls", rerankCalls)
	}
	if len(retriever.queries) != 1 || retriever.queries[0] != "cpu usage saturation" {
		t.Fatalf("expected retrieve query to use rewritten query, got %#v", retriever.queries)
	}
	if len(retriever.requestedTop) != 1 || retriever.requestedTop[0] != RetrieverCandidateTopK(context.Background()) {
		t.Fatalf("expected expanded retrieve topK, got %#v", retriever.requestedTop)
	}
	if trace.RewrittenQuery != "cpu usage saturation" {
		t.Fatalf("expected rewritten query trace, got %q", trace.RewrittenQuery)
	}
	if trace.RerankEnabled {
		t.Fatalf("expected rerank disabled trace")
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
