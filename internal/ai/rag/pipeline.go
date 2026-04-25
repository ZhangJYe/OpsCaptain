package rag

import (
	"context"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type Pipeline interface {
	Retrieve(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, QueryTrace, error)
	Name() string
}

type HybridPipeline struct{}

func (p *HybridPipeline) Name() string { return "hybrid" }

func (p *HybridPipeline) Retrieve(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, QueryTrace, error) {
	trace := QueryTrace{
		Mode:          p.Name(),
		OriginalQuery: query,
	}

	lexIdx := SharedBM25Index()
	cfg := DefaultHybridConfig(ctx)

	docs, hybridTrace, err := HybridRetrieve(ctx, pool, lexIdx, query, cfg)
	trace.Hybrid = &hybridTrace
	trace.CacheKey = hybridTrace.CacheKey
	trace.CacheHit = hybridTrace.CacheHit
	trace.InitFailureCached = hybridTrace.InitFailureCached
	trace.InitLatencyMs = hybridTrace.InitLatencyMs
	trace.RetrieveLatencyMs = hybridTrace.DenseLatencyMs
	trace.RawResultCount = hybridTrace.FusedCount
	if err != nil {
		trace.ResultCount = len(docs)
		return nil, trace, err
	}

	rerankStart := time.Now()
	rerankResult := Rerank(ctx, query, docs, cfg.FinalTopK)
	trace.RerankLatencyMs = time.Since(rerankStart).Milliseconds()
	trace.RerankEnabled = rerankResult.Enabled
	docs = rerankResult.Docs

	trace.ResultCount = len(docs)
	return docs, trace, nil
}

type DenseOnlyPipeline struct{}

func (p *DenseOnlyPipeline) Name() string { return "dense" }

func (p *DenseOnlyPipeline) Retrieve(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, QueryTrace, error) {
	trace := QueryTrace{
		Mode:          p.Name(),
		OriginalQuery: query,
	}

	cfg := DefaultHybridConfig(ctx)
	rr, acq, err := pool.GetOrCreate(ctx)
	trace.CacheKey = acq.CacheKey
	trace.CacheHit = acq.CacheHit
	trace.InitFailureCached = acq.InitFailureCached
	trace.InitLatencyMs = acq.InitLatencyMs
	if err != nil {
		return nil, trace, err
	}

	start := time.Now()
	docs, err := rr.Retrieve(ctx, query, retrieverapi.WithTopK(cfg.DenseTopK))
	trace.RetrieveLatencyMs = time.Since(start).Milliseconds()
	trace.RawResultCount = len(docs)
	if err != nil {
		return nil, trace, err
	}

	docs = refineRetrievedDocs(query, docs)
	finalTopK := cfg.FinalTopK
	if finalTopK <= 0 {
		finalTopK = 10
	}
	docs = trimRetrievedDocs(docs, finalTopK)

	rerankStart := time.Now()
	rerankResult := Rerank(ctx, query, docs, finalTopK)
	trace.RerankLatencyMs = time.Since(rerankStart).Milliseconds()
	trace.RerankEnabled = rerankResult.Enabled
	docs = rerankResult.Docs

	trace.ResultCount = len(docs)
	return docs, trace, nil
}
