package rag

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

type QueryTrace struct {
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
	if strings.TrimSpace(query) == "" {
		return nil, QueryTrace{}, nil
	}

	trace := QueryTrace{OriginalQuery: query}

	rewriteStart := time.Now()
	rewritten := RewriteQuery(ctx, query)
	trace.RewriteLatencyMs = time.Since(rewriteStart).Milliseconds()
	trace.RewrittenQuery = rewritten

	rr, acquisition, err := pool.GetOrCreate(ctx)
	trace.CacheKey = acquisition.CacheKey
	trace.CacheHit = acquisition.CacheHit
	trace.InitFailureCached = acquisition.InitFailureCached
	trace.InitLatencyMs = acquisition.InitLatencyMs
	if err != nil {
		return nil, trace, err
	}

	retrieveStart := time.Now()
	docs, err := rr.Retrieve(ctx, rewritten)
	trace.RetrieveLatencyMs = time.Since(retrieveStart).Milliseconds()
	trace.RawResultCount = len(docs)
	if err != nil {
		return nil, trace, err
	}

	rerankStart := time.Now()
	rerankResult := Rerank(ctx, query, docs, RetrieverTopK(ctx))
	trace.RerankLatencyMs = time.Since(rerankStart).Milliseconds()
	trace.RerankEnabled = rerankResult.Enabled

	finalDocs := rerankResult.Docs
	trace.ResultCount = len(finalDocs)
	return finalDocs, trace, nil
}
