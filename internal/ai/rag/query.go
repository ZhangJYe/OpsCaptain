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
)

type QueryTrace struct {
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
	return QueryWithMode(ctx, pool, query, QueryModeRewriteRetrieveRerank)
}

func ParseQueryMode(raw string) (QueryMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "full", "query", "rerank", "rewrite_retrieve_rerank", "rewrite-retrieve-rerank":
		return QueryModeRewriteRetrieveRerank, nil
	case "rewrite", "rewrite_retrieve", "rewrite-retrieve":
		return QueryModeRewriteRetrieve, nil
	case "retrieve", "retriever", "retrieve_only", "retrieve-only":
		return QueryModeRetrieveOnly, nil
	default:
		return "", fmt.Errorf("unsupported query mode: %s", raw)
	}
}

type rewriteFunc func(context.Context, string) string
type rerankFunc func(context.Context, string, []*schema.Document, int) RerankResult

func QueryWithMode(ctx context.Context, pool *RetrieverPool, query string, mode QueryMode) ([]*schema.Document, QueryTrace, error) {
	return queryWithMode(ctx, pool, query, mode, RewriteQuery, Rerank)
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
		mode = QueryModeRewriteRetrieveRerank
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
