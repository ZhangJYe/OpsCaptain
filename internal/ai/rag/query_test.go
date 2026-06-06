package rag

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/common"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/document"
	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
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

func TestQueryForEval_FiltersOtherUserScopedSources(t *testing.T) {
	t.Parallel()

	alicePrefix := common.KnowledgeSourcePrefixForUser("alice@example.com")
	bobPrefix := common.KnowledgeSourcePrefixForUser("bob@example.com")
	retriever := &fakeQueryRetriever{docs: []*schema.Document{
		{ID: "alice-doc", Content: "redis failover", MetaData: map[string]any{"_source": alicePrefix + "runbook.md"}},
		{ID: "bob-doc", Content: "redis failover", MetaData: map[string]any{"_source": bobPrefix + "runbook.md"}},
		{ID: "shared-doc", Content: "redis failover", MetaData: map[string]any{"_source": "upload://shared.md"}},
	}}
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) { return retriever, nil },
		func(context.Context) string { return "test-eval-scope" },
		nil,
	)
	ctx := context.WithValue(context.Background(), consts.CtxKeyUserID, "alice@example.com")

	docs, trace, err := QueryForEval(ctx, pool, "redis", false, false)
	if err != nil {
		t.Fatalf("QueryForEval returned error: %v", err)
	}
	if trace.RawResultCount != 2 {
		t.Fatalf("expected raw result count after scope filter to be 2, got %d", trace.RawResultCount)
	}
	for _, doc := range docs {
		source := metadataString(doc.MetaData, "_source")
		if strings.HasPrefix(source, bobPrefix) {
			t.Fatalf("unexpected other user source returned: %s", source)
		}
	}
}

func TestIndexSourceDoesNotDeleteExistingWhenIndexGraphFails(t *testing.T) {
	oldFileDir := common.FileDir
	common.FileDir = t.TempDir()
	defer func() { common.FileDir = oldFileDir }()

	store := &fakeIndexVectorStore{}
	graph := &fakeIndexGraph{err: errors.New("index failed")}
	indexing := NewIndexingServiceWithStore(store)
	indexing.buildPipeline = func(context.Context) (compose.Runnable[document.Source, []string], error) {
		return graph, nil
	}
	indexing.newLoader = func(context.Context) (document.Loader, error) {
		return &fakeIndexLoader{docs: []*schema.Document{{ID: "doc", Content: "hello", MetaData: map[string]any{"_source": "upload://runbook.md"}}}}, nil
	}

	_, err := indexing.IndexSource(context.Background(), "/tmp/runbook.md")
	if err == nil {
		t.Fatal("expected index graph error")
	}
	if !graph.invoked {
		t.Fatal("expected index graph to be invoked")
	}
	if store.deleteCalls != 0 {
		t.Fatalf("delete should not be called when indexing fails, got %d", store.deleteCalls)
	}
}

func TestIndexSourceDeletesOldSourceExceptNewChunkIDs(t *testing.T) {
	oldFileDir := common.FileDir
	common.FileDir = t.TempDir()
	defer func() { common.FileDir = oldFileDir }()

	store := &fakeIndexVectorStore{deleted: 2}
	graph := &fakeIndexGraph{ids: []string{"new-1", "new-2"}}
	indexing := NewIndexingServiceWithStore(store)
	indexing.buildPipeline = func(context.Context) (compose.Runnable[document.Source, []string], error) {
		return graph, nil
	}
	indexing.newLoader = func(context.Context) (document.Loader, error) {
		return &fakeIndexLoader{docs: []*schema.Document{{ID: "doc", Content: "hello", MetaData: map[string]any{"_source": "upload://runbook.md"}}}}, nil
	}

	summary, err := indexing.IndexSource(context.Background(), "/tmp/runbook.md")
	if err != nil {
		t.Fatalf("IndexSource returned error: %v", err)
	}
	if store.deleteCalls != 1 {
		t.Fatalf("expected delete once, got %d", store.deleteCalls)
	}
	if store.sourceValue != "upload://runbook.md" {
		t.Fatalf("unexpected source value: %s", store.sourceValue)
	}
	if strings.Join(store.keepIDs, ",") != "new-1,new-2" {
		t.Fatalf("unexpected keep IDs: %#v", store.keepIDs)
	}
	if summary.DeletedExisting != 2 {
		t.Fatalf("expected deleted count 2, got %d", summary.DeletedExisting)
	}
	if strings.Join(summary.ChunkIDs, ",") != "new-1,new-2" {
		t.Fatalf("unexpected chunk IDs: %#v", summary.ChunkIDs)
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

type fakeIndexVectorStore struct {
	deleteCalls int
	collection  string
	sourceValue string
	keepIDs     []string
	deleted     int
	err         error
}

func (f *fakeIndexVectorStore) DeleteBySource(ctx context.Context, collection string, sourceValue string) (int, error) {
	return f.DeleteBySourceExcept(ctx, collection, sourceValue, nil)
}

func (f *fakeIndexVectorStore) DeleteBySourceExcept(_ context.Context, collection string, sourceValue string, keepIDs []string) (int, error) {
	f.deleteCalls++
	f.collection = collection
	f.sourceValue = sourceValue
	f.keepIDs = append([]string(nil), keepIDs...)
	return f.deleted, f.err
}

type fakeIndexLoader struct {
	docs []*schema.Document
	err  error
}

func (f *fakeIndexLoader) Load(context.Context, document.Source, ...document.LoaderOption) ([]*schema.Document, error) {
	return f.docs, f.err
}

type fakeIndexGraph struct {
	ids     []string
	err     error
	invoked bool
}

func (f *fakeIndexGraph) Invoke(context.Context, document.Source, ...compose.Option) ([]string, error) {
	f.invoked = true
	return f.ids, f.err
}

func (f *fakeIndexGraph) Stream(context.Context, document.Source, ...compose.Option) (*schema.StreamReader[[]string], error) {
	return nil, errors.New("not implemented")
}

func (f *fakeIndexGraph) Collect(context.Context, *schema.StreamReader[document.Source], ...compose.Option) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeIndexGraph) Transform(context.Context, *schema.StreamReader[document.Source], ...compose.Option) (*schema.StreamReader[[]string], error) {
	return nil, errors.New("not implemented")
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
