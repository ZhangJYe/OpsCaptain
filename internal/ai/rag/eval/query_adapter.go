package eval

import (
	"SuperBizAgent/internal/ai/rag"
	"context"
	"time"
)

func NewQueryExecutor(pool *rag.RetrieverPool) QueryExecutor {
	return func(ctx context.Context, query string) ([]RetrievedDoc, QueryMetrics, error) {
		start := time.Now()
		docs, trace, err := rag.Query(ctx, pool, query)
		totalMs := time.Since(start).Milliseconds()
		if err != nil {
			return nil, QueryMetrics{}, err
		}
		metrics := QueryMetrics{
			CacheHit:          trace.CacheHit,
			InitFailureCached: trace.InitFailureCached,
			InitLatencyMs:     trace.InitLatencyMs,
			RewriteLatencyMs:  trace.RewriteLatencyMs,
			RetrieveLatencyMs: trace.RetrieveLatencyMs,
			ResultCount:       trace.ResultCount,
			TotalLatencyMs:    totalMs,
		}
		if trace.Hybrid != nil {
			metrics.RetrieveLatencyMs = trace.Hybrid.DenseLatencyMs
		}
		if trace.RerankEnabled {
			metrics.RerankLatencyMs = trace.RerankLatencyMs
		}
		return SchemaDocsToRetrievedDocs(docs), metrics, nil
	}
}
