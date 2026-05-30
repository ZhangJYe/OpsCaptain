package rag

import (
	"context"
	"strings"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
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

	wantRewrite := ragConfigBool(ctx, "rag.rewrite_enabled")
	wantRerank := ragConfigBool(ctx, "rag.rerank_enabled")

	searchQuery := query
	if wantRewrite {
		rewriteStart := time.Now()
		rewritten := RewriteQuery(ctx, query)
		trace.RewriteLatencyMs = time.Since(rewriteStart).Milliseconds()
		if rewritten != "" {
			searchQuery = rewritten
		}
		trace.RewrittenQuery = searchQuery
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
	if wantRerank {
		cfg.CandidateTopK = RetrieverCandidateTopK(ctx)
	}

	docs, hybridTrace, err := HybridRetrieveWithRetriever(ctx, rr, lexIdx, searchQuery, cfg)
	trace.Hybrid = &hybridTrace
	trace.RetrieveLatencyMs = hybridTrace.DenseLatencyMs
	trace.RawResultCount = hybridTrace.FusedCount
	if err != nil {
		return nil, trace, err
	}

	if wantRerank {
		topK := cfg.FinalTopK
		if topK <= 0 {
			topK = 10
		}
		rerankStart := time.Now()
		rerankResult := Rerank(ctx, query, docs, topK)
		trace.RerankLatencyMs = time.Since(rerankStart).Milliseconds()
		trace.RerankEnabled = rerankResult.Enabled
		if rerankResult.Enabled {
			docs = rerankResult.Docs
		} else {
			docs = trimRetrievedDocs(docs, topK)
		}
	} else {
		topK := cfg.FinalTopK
		if topK <= 0 {
			topK = 10
		}
		docs = trimRetrievedDocs(docs, topK)
	}

	trace.ResultCount = len(docs)
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

func ragConfigBool(ctx context.Context, key string) bool {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return false
	}
	return v.Bool()
}
