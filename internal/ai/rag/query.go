package rag

import (
	"context"
	"strings"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type ctxKey string

const (
	ctxOverrideRewrite ctxKey = "rag.override.rewrite"
	ctxOverrideRerank  ctxKey = "rag.override.rerank"
	ctxOverrideHybrid  ctxKey = "rag.override.hybrid"
	ctxIntentQuery     ctxKey = "rag.intent.query"
)

func WithRerankOverride(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, ctxOverrideRerank, v)
}

func WithRewriteOverride(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, ctxOverrideRewrite, v)
}

func WithHybridConfigOverride(ctx context.Context, cfg HybridConfig) context.Context {
	return context.WithValue(ctx, ctxOverrideHybrid, cfg)
}

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
	RewriteAttempted  bool
	RewriteApplied    bool
	RewriteDegraded   bool
	RewriteReason     string
	RerankAttempted   bool
	RerankEnabled     bool
	RerankDegraded    bool
	RerankReason      string
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
		rewriteResult := RewriteQueryWithResult(ctx, query)
		trace.RewriteLatencyMs = time.Since(rewriteStart).Milliseconds()
		trace.RewriteAttempted = rewriteResult.Attempted
		trace.RewriteApplied = rewriteResult.Applied
		trace.RewriteDegraded = rewriteResult.Degraded
		trace.RewriteReason = rewriteResult.Reason
		if rewriteResult.Query != "" {
			searchQuery = rewriteResult.Query
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

	hybridCtx := context.WithValue(ctx, ctxIntentQuery, query)
	docs, hybridTrace, err := HybridRetrieveWithRetriever(hybridCtx, rr, lexIdx, searchQuery, cfg)
	trace.Hybrid = &hybridTrace
	trace.RetrieveLatencyMs = hybridTrace.TotalLatencyMs
	trace.RawResultCount = hybridTrace.FusedCount
	if err != nil {
		return nil, trace, err
	}

	if wantRerank {
		topK := cfg.FinalTopK
		if topK <= 0 {
			topK = 10
		}
		maxCandidates := RerankMaxCandidates(ctx)
		if len(docs) > maxCandidates {
			docs = docs[:maxCandidates]
		}
		rerankStart := time.Now()
		rerankResult := Rerank(ctx, query, docs, topK)
		trace.RerankLatencyMs = time.Since(rerankStart).Milliseconds()
		trace.RerankAttempted = rerankResult.Attempted
		trace.RerankEnabled = rerankResult.Enabled
		trace.RerankDegraded = rerankResult.Degraded
		trace.RerankReason = rerankResult.Reason
		if rerankResult.Enabled {
			docs = rerankResult.Docs
			for i, doc := range docs {
				if i < len(rerankResult.Scores) && doc.MetaData != nil {
					doc.MetaData[metaKeyRerankScore] = rerankResult.Scores[i]
				}
			}
		}
	}
	docs = selectFinalDocs(query, docs, cfg)
	if trace.Hybrid != nil {
		trace.Hybrid.FinalIDs = traceDocumentIDs(docs)
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
		rewriteResult := RewriteQueryWithResult(ctx, query)
		trace.RewriteLatencyMs = time.Since(rewriteStart).Milliseconds()
		trace.RewriteAttempted = rewriteResult.Attempted
		trace.RewriteApplied = rewriteResult.Applied
		trace.RewriteDegraded = rewriteResult.Degraded
		trace.RewriteReason = rewriteResult.Reason
		rewritten = rewriteResult.Query
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
	if err != nil {
		return nil, trace, err
	}
	docs = filterDocsBySourceScope(docs, sourceScopeFromContext(ctx))
	trace.RawResultCount = len(docs)
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
	trace.RerankAttempted = rerankResult.Attempted
	trace.RerankEnabled = rerankResult.Enabled
	trace.RerankDegraded = rerankResult.Degraded
	trace.RerankReason = rerankResult.Reason

	finalDocs := rerankResult.Docs
	trace.ResultCount = len(finalDocs)
	return finalDocs, trace, nil
}

func ragConfigBool(ctx context.Context, key string) bool {
	var overrideKey ctxKey
	switch key {
	case "rag.rewrite_enabled":
		overrideKey = ctxOverrideRewrite
	case "rag.rerank_enabled":
		overrideKey = ctxOverrideRerank
	}
	if overrideKey != "" {
		if v, ok := ctx.Value(overrideKey).(bool); ok {
			return v
		}
	}
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return false
	}
	return v.Bool()
}
