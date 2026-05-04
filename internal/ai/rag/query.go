package rag

import (
	"context"
	"strings"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
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
	if strings.TrimSpace(query) == "" {
		return nil, QueryTrace{}, nil
	}
	trace := QueryTrace{
		Mode:           "hybrid",
		OriginalQuery:  query,
		RewrittenQuery: query,
	}

	rr, acquisition, err := pool.GetOrCreate(ctx)
	trace.CacheKey = acquisition.CacheKey
	trace.CacheHit = acquisition.CacheHit
	trace.InitFailureCached = acquisition.InitFailureCached
	trace.InitLatencyMs = acquisition.InitLatencyMs
	if err != nil {
		return nil, trace, err
	}

	lexIdx := SharedBM25Index()
	cfg := DefaultHybridConfig(ctx)

	docs, hybridTrace, err := HybridRetrieveWithRetriever(ctx, rr, lexIdx, query, cfg)
	trace.Hybrid = &hybridTrace
	trace.RetrieveLatencyMs = hybridTrace.DenseLatencyMs
	trace.RawResultCount = hybridTrace.FusedCount
	trace.ResultCount = len(docs)
	if err != nil {
		return nil, trace, err
	}
	return docs, trace, nil
}

func QueryForEval(ctx context.Context, pool *RetrieverPool, query string, wantRewrite, wantRerank bool) ([]*schema.Document, QueryTrace, error) {
	if strings.TrimSpace(query) == "" {
		return nil, QueryTrace{}, nil
	}

	trace := QueryTrace{
		Mode:           "eval",
		OriginalQuery:  query,
		RewrittenQuery: query,
	}
	topK := RetrieverTopK(ctx)
	candidateTopK := RetrieverCandidateTopK(ctx)

	rewritten := query
	if wantRewrite {
		rewriteStart := time.Now()
		rewritten = RewriteQuery(ctx, query)
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

	if !wantRerank {
		finalDocs := trimRetrievedDocs(docs, topK)
		trace.ResultCount = len(finalDocs)
		trace.RerankEnabled = false
		return finalDocs, trace, nil
	}

	rerankStart := time.Now()
	rerankResult := Rerank(ctx, query, docs, topK)
	trace.RerankLatencyMs = time.Since(rerankStart).Milliseconds()
	trace.RerankEnabled = rerankResult.Enabled

	finalDocs := rerankResult.Docs
	trace.ResultCount = len(finalDocs)
	return finalDocs, trace, nil
}
