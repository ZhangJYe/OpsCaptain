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
			RewriteAttempted:  trace.RewriteAttempted,
			RewriteApplied:    trace.RewriteApplied,
			RewriteDegraded:   trace.RewriteDegraded,
			RewriteReason:     trace.RewriteReason,
			RerankAttempted:   trace.RerankAttempted,
			RerankApplied:     trace.RerankEnabled,
			RerankDegraded:    trace.RerankDegraded,
			RerankReason:      trace.RerankReason,
			RerankLatencyMs:   trace.RerankLatencyMs,
			TotalLatencyMs:    totalMs,
		}
		if trace.Hybrid != nil {
			metrics.RetrieveLatencyMs = trace.Hybrid.TotalLatencyMs
			metrics.DenseLatencyMs = trace.Hybrid.DenseLatencyMs
			metrics.LexicalLatencyMs = trace.Hybrid.LexicalLatencyMs
			metrics.FusionLatencyMs = trace.Hybrid.FusionLatencyMs
			metrics.DenseCount = trace.Hybrid.DenseCount
			metrics.LexicalCount = trace.Hybrid.LexicalCount
			metrics.FusedCount = trace.Hybrid.FusedCount
			metrics.CandidateCount = trace.Hybrid.CandidateCount
			metrics.Stages = RetrievalStages{
				Dense: append([]string(nil), trace.Hybrid.DenseIDs...), Lexical: append([]string(nil), trace.Hybrid.LexicalIDs...),
				Fusion: append([]string(nil), trace.Hybrid.FusionIDs...), Candidate: append([]string(nil), trace.Hybrid.CandidateIDs...),
				Final: append([]string(nil), trace.Hybrid.FinalIDs...),
			}
			metrics.Intent = IntentMetrics{Parsed: trace.Hybrid.Intent.Parsed, Applied: trace.Hybrid.Intent.Applied, Rule: trace.Hybrid.Intent.Rule, PenalizedDocs: trace.Hybrid.Intent.PenalizedDocs}
		}
		return SchemaDocsToRetrievedDocs(docs), metrics, nil
	}
}
