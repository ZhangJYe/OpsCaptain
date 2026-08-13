package rag

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/common"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

func TestRRFFusionWeightedChangesChannelContribution(t *testing.T) {
	t.Parallel()
	dense := []*schema.Document{{ID: "dense"}, {ID: "both"}}
	lexical := []BM25Hit{{DocID: "lexical"}, {DocID: "both"}}
	fused := rrfFusionWeighted(dense, lexical, 60, 1, 2)
	if got := docFusionKey(fused[0].doc); got != "lexical" && got != "both" {
		t.Fatalf("lexical weighting was not applied, first=%s", got)
	}
	if fused[0].score <= 0 || len(fused) != 3 {
		t.Fatalf("unexpected weighted fusion: %+v", fused)
	}
}

func TestHybridWeightsValidationAndCompatibility(t *testing.T) {
	t.Parallel()
	dense, lexical, err := hybridWeights(HybridConfig{})
	if err != nil || dense != 1 || lexical != 1 {
		t.Fatalf("zero-value config must preserve equal-weight compatibility: dense=%v lexical=%v err=%v", dense, lexical, err)
	}
	if _, _, err := hybridWeights(HybridConfig{DenseWeight: -1, LexicalWeight: 1}); err == nil {
		t.Fatal("negative weight must be rejected")
	}
}

func TestKnowledgeFieldRefinementTraceAndDisabledCompatibility(t *testing.T) {
	t.Parallel()
	docs := []*schema.Document{
		{ID: "generic", Content: "helm operation", MetaData: map[string]any{}},
		{ID: "history", Content: "release revisions", MetaData: map[string]any{"knowledge_doc_id": "helm-history", "title": "Helm History", "tags": []string{"helm", "history"}, "provider": "helm"}},
	}
	disabled := refineRetrievedDocsWithConfig("helm history", docs, HybridConfig{})
	if disabled[0].ID != "generic" {
		t.Fatalf("disabled field refinement changed order: %s", disabled[0].ID)
	}
	enabled := refineRetrievedDocsWithConfig("helm history", docs, HybridConfig{KnowledgeFieldBoostEnabled: true, KnowledgeDocIDBoost: 8, KnowledgeTitleBoost: 6, KnowledgeTagsBoost: 4, KnowledgeProviderBoost: 2, KnowledgeFieldBoostCap: 12})
	if enabled[0].ID != "history" {
		t.Fatalf("field-aware refinement did not promote title/tag match: %s", enabled[0].ID)
	}
	if enabled[0].MetaData[metaKeyFieldBoost] == nil || enabled[0].MetaData[metaKeyRefinePosition] != 1 {
		t.Fatalf("field refinement trace missing: %+v", enabled[0].MetaData)
	}
}

func TestCoverageSelectionIsBoundedAndDeterministic(t *testing.T) {
	t.Parallel()
	docs := []*schema.Document{
		{ID: "redis-1", Content: "redis timeout", MetaData: map[string]any{}},
		{ID: "redis-2", Content: "redis timeout retry", MetaData: map[string]any{}},
		{ID: "postgres", Content: "postgres checkpoint", MetaData: map[string]any{}},
	}
	cfg := HybridConfig{FinalTopK: 2, CoverageEnabled: true, CoverageMaxPositionGain: 2}
	selected := selectFinalDocs("redis timeout postgres checkpoint", docs, cfg)
	if len(selected) != 2 || selected[0].ID != "redis-1" || selected[1].ID != "postgres" {
		t.Fatalf("unexpected bounded coverage order: %v %v", selected[0].ID, selected[1].ID)
	}
	if selected[1].MetaData[metaKeyCoverageBoost] != float64(1) || selected[1].MetaData[metaKeyFinalPosition] != 2 {
		t.Fatalf("coverage trace missing: %+v %+v", selected[0].MetaData, selected[1].MetaData)
	}
}

func TestCoverageSelectionPreservesOrderWithoutNewSignal(t *testing.T) {
	t.Parallel()
	docs := []*schema.Document{{ID: "a", Content: "redis"}, {ID: "b", Content: "redis"}, {ID: "c", Content: "redis"}}
	selected := selectFinalDocs("redis", docs, HybridConfig{FinalTopK: 2, CoverageEnabled: true, CoverageMaxPositionGain: 2})
	if len(selected) != 2 || selected[0].ID != "a" || selected[1].ID != "b" {
		t.Fatalf("insufficient signal must preserve order: %+v", selected)
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

func TestBuildBM25IndexFromDocs_PreservesChunksWithSameSource(t *testing.T) {
	t.Parallel()

	docs := []*schema.Document{
		{
			ID:      "chunk-1",
			Content: "redis failover requires checking sentinel quorum",
			MetaData: map[string]any{
				"_source": "upload://runbook.md",
				"title":   "Runbook",
			},
		},
		{
			ID:      "chunk-2",
			Content: "postgres checkpoint latency requires storage investigation",
			MetaData: map[string]any{
				"_source": "upload://runbook.md",
				"title":   "Runbook",
			},
		},
	}

	idx := BuildBM25IndexFromDocs(docs)
	if idx.Size() != 2 {
		t.Fatalf("expected 2 chunks indexed, got %d", idx.Size())
	}
	redisHits := idx.Search("redis sentinel", 5)
	if len(redisHits) == 0 || redisHits[0].DocID != "chunk-1" {
		t.Fatalf("expected chunk-1 for redis query, got %+v", redisHits)
	}
	if redisHits[0].Meta["_source"] != "upload://runbook.md" || redisHits[0].Meta["title"] != "Runbook" {
		t.Fatalf("expected source and title metadata preserved, got %+v", redisHits[0].Meta)
	}
	postgresHits := idx.Search("postgres checkpoint", 5)
	if len(postgresHits) == 0 || postgresHits[0].DocID != "chunk-2" {
		t.Fatalf("expected chunk-2 for postgres query, got %+v", postgresHits)
	}
}

func TestRRFFusion_PreservesChunksWithSameSource(t *testing.T) {
	t.Parallel()

	denseDocs := []*schema.Document{
		{ID: "chunk-1", Content: "redis failover", MetaData: map[string]any{"_source": "upload://runbook.md"}},
		{ID: "chunk-2", Content: "postgres checkpoint", MetaData: map[string]any{"_source": "upload://runbook.md"}},
	}
	lexHits := []BM25Hit{
		{DocID: "chunk-1", Content: "redis failover", Meta: map[string]string{"_source": "upload://runbook.md"}},
		{DocID: "chunk-2", Content: "postgres checkpoint", Meta: map[string]string{"_source": "upload://runbook.md"}},
	}

	fused := rrfFusion(denseDocs, lexHits, 60)
	if len(fused) != 2 {
		t.Fatalf("expected 2 fused chunks, got %d", len(fused))
	}
	ids := map[string]bool{}
	for _, item := range fused {
		ids[item.doc.ID] = true
	}
	if !ids["chunk-1"] || !ids["chunk-2"] {
		t.Fatalf("expected both chunk IDs, got %+v", ids)
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
	if len(trace.DenseIDs) != trace.DenseCount || len(trace.LexicalIDs) != trace.LexicalCount || len(trace.FusionIDs) != trace.FusedCount || len(trace.CandidateIDs) != trace.CandidateCount {
		t.Fatalf("stage trace IDs must stay within existing result bounds: %+v", trace)
	}

	firstID := docFusionKey(docs[0])
	if firstID != "case-020" {
		t.Fatalf("expected case-020 (both channels + metadata match) to rank first, got %s", firstID)
	}
}

func TestHybridRetrieve_CandidateTopKReturnsMoreThanFinalTopK(t *testing.T) {
	t.Parallel()

	denseResults := []*schema.Document{
		{ID: "d1", Content: "a", MetaData: map[string]any{"case_id": "case-001"}},
		{ID: "d2", Content: "b", MetaData: map[string]any{"case_id": "case-002"}},
		{ID: "d3", Content: "c", MetaData: map[string]any{"case_id": "case-003"}},
		{ID: "d4", Content: "d", MetaData: map[string]any{"case_id": "case-004"}},
		{ID: "d5", Content: "e", MetaData: map[string]any{"case_id": "case-005"}},
		{ID: "d6", Content: "f", MetaData: map[string]any{"case_id": "case-006"}},
	}

	fr := &fakeHybridRetriever{docs: denseResults}
	pool := NewRetrieverPool(
		func(ctx context.Context) (retrieverapi.Retriever, error) { return fr, nil },
		func(ctx context.Context) string { return "test-candidate" },
		nil,
	)

	cfg := HybridConfig{
		DenseTopK:     10,
		LexicalTopK:   10,
		FusionK:       60,
		CandidateTopK: 6,
		FinalTopK:     2,
	}

	docs, _, err := HybridRetrieve(context.Background(), pool, nil, "test query", cfg)
	if err != nil {
		t.Fatalf("HybridRetrieve returned error: %v", err)
	}
	if len(docs) > 6 {
		t.Fatalf("expected at most 6 candidates (CandidateTopK), got %d", len(docs))
	}
	if len(docs) < 2 {
		t.Fatalf("expected at least 2 docs, got %d", len(docs))
	}
}

func TestHybridRetrieveTraceIncludesWallClockAndCandidateCount(t *testing.T) {
	t.Parallel()

	fr := &fakeHybridRetriever{
		delay: 15 * time.Millisecond,
		docs: []*schema.Document{
			{ID: "d1", Content: "one"},
			{ID: "d2", Content: "two"},
			{ID: "d3", Content: "three"},
		},
	}
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) { return fr, nil },
		func(context.Context) string { return "test-hybrid-trace" },
		nil,
	)

	docs, trace, err := HybridRetrieve(context.Background(), pool, nil, "query", HybridConfig{
		DenseTopK: 10, LexicalTopK: 10, FusionK: 60, CandidateTopK: 2, FinalTopK: 1,
	})
	if err != nil {
		t.Fatalf("HybridRetrieve returned error: %v", err)
	}
	if len(docs) != 2 || trace.CandidateCount != 2 {
		t.Fatalf("candidate count mismatch: docs=%d trace=%d", len(docs), trace.CandidateCount)
	}
	if trace.TotalLatencyMs < trace.DenseLatencyMs || trace.TotalLatencyMs < 10 {
		t.Fatalf("hybrid wall clock must cover dense retrieval, got total=%d dense=%d", trace.TotalLatencyMs, trace.DenseLatencyMs)
	}
}

func TestQueryBoundsFinalResultsAfterCandidateRetrieval(t *testing.T) {
	ResetSharedBM25Index()
	defer ResetSharedBM25Index()

	fr := &fakeHybridRetriever{docs: []*schema.Document{
		{ID: "d1", Content: "one"}, {ID: "d2", Content: "two"}, {ID: "d3", Content: "three"}, {ID: "d4", Content: "four"},
	}}
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) { return fr, nil },
		func(context.Context) string { return "test-final-bound" },
		nil,
	)
	cfg := HybridConfig{DenseTopK: 10, LexicalTopK: 10, FusionK: 60, CandidateTopK: 4, FinalTopK: 2}
	ctx := WithHybridConfigOverride(context.Background(), cfg)
	ctx = WithRewriteOverride(ctx, false)
	ctx = WithRerankOverride(ctx, false)
	docs, trace, err := Query(ctx, pool, "query")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if len(docs) != 2 || trace.ResultCount != 2 {
		t.Fatalf("final result must be bounded by FinalTopK: docs=%d trace=%d", len(docs), trace.ResultCount)
	}
	if trace.Hybrid == nil || trace.Hybrid.CandidateCount != 4 {
		t.Fatalf("expected four pre-final candidates, got %+v", trace.Hybrid)
	}
}

func TestHybridRetrieve_FiltersOtherUserScopedSources(t *testing.T) {
	t.Parallel()

	alicePrefix := common.KnowledgeSourcePrefixForUser("alice@example.com")
	bobPrefix := common.KnowledgeSourcePrefixForUser("bob@example.com")
	denseResults := []*schema.Document{
		{ID: "alice-dense", Content: "redis sentinel failover", MetaData: map[string]any{"_source": alicePrefix + "runbook.md"}},
		{ID: "bob-dense", Content: "redis sentinel failover", MetaData: map[string]any{"_source": bobPrefix + "runbook.md"}},
		{ID: "shared-dense", Content: "redis sentinel shared guide", MetaData: map[string]any{"_source": "upload://shared-runbook.md"}},
	}
	fr := &fakeHybridRetriever{docs: denseResults}
	pool := NewRetrieverPool(
		func(ctx context.Context) (retrieverapi.Retriever, error) { return fr, nil },
		func(ctx context.Context) string { return "test-user-scope" },
		nil,
	)

	lexIdx := NewBM25Index()
	lexIdx.AddDocument("alice-lex", "redis sentinel quorum", map[string]string{"_source": alicePrefix + "lex.md"})
	lexIdx.AddDocument("bob-lex", "redis sentinel quorum", map[string]string{"_source": bobPrefix + "lex.md"})
	lexIdx.AddDocument("shared-lex", "redis sentinel quorum", map[string]string{"_source": "upload://shared-lex.md"})

	ctx := context.WithValue(context.Background(), consts.CtxKeyUserID, "alice@example.com")
	docs, trace, err := HybridRetrieve(ctx, pool, lexIdx, "redis sentinel", HybridConfig{
		DenseTopK:     10,
		LexicalTopK:   10,
		FusionK:       60,
		CandidateTopK: 10,
		FinalTopK:     10,
	})
	if err != nil {
		t.Fatalf("HybridRetrieve returned error: %v", err)
	}
	if trace.DenseCount != 2 {
		t.Fatalf("expected dense count after scope filter to be 2, got %d", trace.DenseCount)
	}
	if trace.LexicalCount != 2 {
		t.Fatalf("expected lexical count after scope filter to be 2, got %d", trace.LexicalCount)
	}
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		source := metadataString(doc.MetaData, "_source")
		if strings.HasPrefix(source, bobPrefix) {
			t.Fatalf("unexpected other user source returned: %s", source)
		}
	}
}

func TestQuery_HybridWithPopulatedSharedBM25(t *testing.T) {
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

	docs, trace, err := Query(context.Background(), pool, "checkoutservice rrt timeout")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if trace.Mode != "hybrid" {
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

func TestQuery_HybridWithEmptySharedBM25FallsBackToDenseOnly(t *testing.T) {
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

	docs, trace, err := Query(context.Background(), pool, "cpu high")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
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
	docs  []*schema.Document
	delay time.Duration
	err   error
}

func (f *fakeHybridRetriever) Retrieve(ctx context.Context, query string, opts ...retrieverapi.Option) ([]*schema.Document, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.docs, nil
}

func TestHybridRetrieveReturnsBaseRetrieverError(t *testing.T) {
	t.Parallel()

	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) {
			return &fakeHybridRetriever{err: errors.New("dense unavailable")}, nil
		},
		func(context.Context) string { return "test-retrieval-error" },
		nil,
	)
	_, trace, err := HybridRetrieve(context.Background(), pool, nil, "query", HybridConfig{DenseTopK: 5, CandidateTopK: 5, FinalTopK: 5})
	if err == nil {
		t.Fatal("expected base retriever error")
	}
	if trace.TotalLatencyMs < trace.DenseLatencyMs {
		t.Fatalf("error trace total latency must cover dense latency: %+v", trace)
	}
}
