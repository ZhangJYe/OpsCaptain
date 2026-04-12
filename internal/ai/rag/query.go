package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type QueryMode string

const (
	QueryModeRetrieveOnly          QueryMode = "retrieve"
	QueryModeRewriteRetrieve       QueryMode = "rewrite"
	QueryModeRewriteRetrieveRerank QueryMode = "full"
	QueryModeHybrid                QueryMode = "hybrid"
)

type QueryTrace struct {
	Hybrid            *HybridTrace
	Mode              string
	CacheKey          string
	CacheHit          bool
	InitFailureCached bool
	InitLatencyMs     int64
	RetrieveLatencyMs int64
	RewriteLatencyMs  int64
	RerankLatencyMs   int64
	OriginalQuery     string
	RewrittenQuery    string
	RawResultCount    int
	ResultCount       int
	RerankEnabled     bool
}

func Query(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, QueryTrace, error) {
	return QueryWithMode(ctx, pool, query, DefaultQueryMode(ctx))
}

func ParseQueryMode(raw string) (QueryMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "full", "query", "rerank", "rewrite_retrieve_rerank", "rewrite-retrieve-rerank":
		return QueryModeRewriteRetrieveRerank, nil
	case "rewrite", "rewrite_retrieve", "rewrite-retrieve":
		return QueryModeRewriteRetrieve, nil
	case "retrieve", "retriever", "retrieve_only", "retrieve-only":
		return QueryModeRetrieveOnly, nil
	case "hybrid", "hybrid_retrieval", "hybrid-retrieval":
		return QueryModeHybrid, nil
	default:
		return "", fmt.Errorf("unsupported query mode: %s", raw)
	}
}

type rewriteFunc func(context.Context, string) string
type rerankFunc func(context.Context, string, []*schema.Document, int) RerankResult

func QueryWithMode(ctx context.Context, pool *RetrieverPool, query string, mode QueryMode) ([]*schema.Document, QueryTrace, error) {
	if mode == QueryModeHybrid {
		return hybridQueryWithMode(ctx, pool, query)
	}
	return queryWithMode(ctx, pool, query, mode, RewriteQuery, Rerank)
}

func hybridQueryWithMode(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, QueryTrace, error) {
	if strings.TrimSpace(query) == "" {
		return nil, QueryTrace{}, nil
	}
	trace := QueryTrace{
		Mode:           string(QueryModeHybrid),
		OriginalQuery:  query,
		RewrittenQuery: query,
	}

	lexIdx := SharedBM25Index()
	cfg := DefaultHybridConfig(ctx)

	docs, hybridTrace, err := HybridRetrieve(ctx, pool, lexIdx, query, cfg)
	trace.Hybrid = &hybridTrace
	trace.RetrieveLatencyMs = hybridTrace.DenseLatencyMs
	trace.RawResultCount = hybridTrace.FusedCount
	trace.ResultCount = len(docs)
	if err != nil {
		return nil, trace, err
	}
	return docs, trace, nil
}

func queryWithMode(
	ctx context.Context,
	pool *RetrieverPool,
	query string,
	mode QueryMode,
	rewrite rewriteFunc,
	rerank rerankFunc,
) ([]*schema.Document, QueryTrace, error) {
	if strings.TrimSpace(query) == "" {
		return nil, QueryTrace{}, nil
	}
	if mode == "" {
		mode = DefaultQueryMode(ctx)
	}

	trace := QueryTrace{
		Mode:           string(mode),
		OriginalQuery:  query,
		RewrittenQuery: query,
	}
	topK := RetrieverTopK(ctx)
	candidateTopK := RetrieverCandidateTopK(ctx)

	rewritten := query
	if mode != QueryModeRetrieveOnly {
		rewriteStart := time.Now()
		rewritten = rewrite(ctx, query)
		trace.RewriteLatencyMs = time.Since(rewriteStart).Milliseconds()
		trace.RewrittenQuery = rewritten
	}

	rr, acquisition, err := pool.GetOrCreate(ctx)
	trace.CacheKey = acquisition.CacheKey
	trace.CacheHit = acquisition.CacheHit
	trace.InitFailureCached = acquisition.InitFailureCached
	trace.InitLatencyMs = acquisition.InitLatencyMs
	if err != nil {
		return nil, trace, err
	}

	retrieveStart := time.Now()
	docs, err := rr.Retrieve(ctx, rewritten, retrieverapi.WithTopK(candidateTopK))
	trace.RetrieveLatencyMs = time.Since(retrieveStart).Milliseconds()
	trace.RawResultCount = len(docs)
	if err != nil {
		return nil, trace, err
	}
	docs = refineRetrievedDocs(query, docs)

	if mode != QueryModeRewriteRetrieveRerank {
		finalDocs := trimRetrievedDocs(docs, topK)
		trace.ResultCount = len(finalDocs)
		trace.RerankEnabled = false
		return finalDocs, trace, nil
	}

	rerankStart := time.Now()
	rerankResult := rerank(ctx, query, docs, topK)
	trace.RerankLatencyMs = time.Since(rerankStart).Milliseconds()
	trace.RerankEnabled = rerankResult.Enabled

	finalDocs := rerankResult.Docs
	trace.ResultCount = len(finalDocs)
	return finalDocs, trace, nil
}
