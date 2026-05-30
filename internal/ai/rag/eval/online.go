package eval

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type QueryMetrics struct {
	CacheHit          bool  `json:"cache_hit"`
	InitFailureCached bool  `json:"init_failure_cached"`
	InitLatencyMs     int64 `json:"init_latency_ms"`
	RewriteLatencyMs  int64 `json:"rewrite_latency_ms"`
	RetrieveLatencyMs int64 `json:"retrieve_latency_ms"`
	RerankLatencyMs   int64 `json:"rerank_latency_ms"`
	ResultCount       int   `json:"result_count"`
	TotalLatencyMs    int64 `json:"total_latency_ms"`
}

type QueryExecutor func(context.Context, string) ([]RetrievedDoc, QueryMetrics, error)

type QueryCaseResult struct {
	CaseResult
	Metrics QueryMetrics `json:"metrics"`
}

type QuerySummary struct {
	Summary
	AvgInitLatencyMs     float64 `json:"avg_init_latency_ms"`
	AvgRewriteLatencyMs  float64 `json:"avg_rewrite_latency_ms"`
	AvgRetrieveLatencyMs float64 `json:"avg_retrieve_latency_ms"`
	AvgRerankLatencyMs   float64 `json:"avg_rerank_latency_ms"`
	AvgTotalLatencyMs    float64 `json:"avg_total_latency_ms"`
	CacheHitRate         float64 `json:"cache_hit_rate"`
	EmptyRate            float64 `json:"empty_rate"`
	CitationCoverage     float64 `json:"citation_coverage"`
}

func RunQueryEval(ctx context.Context, exec QueryExecutor, cases []EvalCase, ks []int) (QuerySummary, []QueryCaseResult, error) {
	return RunQueryEvalWithOpts(ctx, exec, cases, ks, defaultRunOptions())
}

func RunQueryEvalWithOpts(ctx context.Context, exec QueryExecutor, cases []EvalCase, ks []int, opts RunOptions) (QuerySummary, []QueryCaseResult, error) {
	if exec == nil {
		return QuerySummary{}, nil, fmt.Errorf("query executor is nil")
	}
	ks = normalizeKs(ks)
	if len(ks) == 0 {
		return QuerySummary{}, nil, fmt.Errorf("ks is empty")
	}

	results := make([]QueryCaseResult, 0, len(cases))
	qSummary := QuerySummary{
		Summary: newSummary(len(cases), ks),
	}

	for _, evalCase := range cases {
		rankedDocs, metrics, err := exec(ctx, evalCase.Query)
		if err != nil {
			if opts.ContinueOnError {
				qSummary.Failures = append(qSummary.Failures, CaseFailure{
					CaseID: evalCase.ID,
					Error:  err.Error(),
				})
				qSummary.Failed++
				continue
			}
			return QuerySummary{}, nil, fmt.Errorf("case %s query failed: %w", evalCase.ID, err)
		}

		rankedIDs := uniqueOrderedRetrievedIDs(rankedDocs)
		caseResult := buildCaseResult(evalCase, rankedIDs, ks)
		accumulateMetrics(&qSummary.Summary, caseResult, evalCase, ks)
		qSummary.Succeeded++

		accumulateQueryMetrics(&qSummary, metrics, rankedIDs)

		results = append(results, QueryCaseResult{
			CaseResult: caseResult,
			Metrics:    metrics,
		})
	}

	finalizeQuerySummary(&qSummary, ks)
	return qSummary, results, nil
}

func accumulateQueryMetrics(qSummary *QuerySummary, metrics QueryMetrics, rankedIDs []string) {
	qSummary.AvgInitLatencyMs += float64(metrics.InitLatencyMs)
	qSummary.AvgRewriteLatencyMs += float64(metrics.RewriteLatencyMs)
	qSummary.AvgRetrieveLatencyMs += float64(metrics.RetrieveLatencyMs)
	qSummary.AvgRerankLatencyMs += float64(metrics.RerankLatencyMs)
	qSummary.AvgTotalLatencyMs += float64(metrics.TotalLatencyMs)
	if metrics.CacheHit {
		qSummary.CacheHitRate++
	}
	if len(rankedIDs) == 0 {
		qSummary.EmptyRate++
	} else {
		qSummary.CitationCoverage++
	}
}

func finalizeQuerySummary(qSummary *QuerySummary, ks []int) {
	finalizeSummary(&qSummary.Summary, ks)
	if qSummary.Succeeded == 0 {
		return
	}
	caseCount := float64(qSummary.Succeeded)
	qSummary.AvgInitLatencyMs /= caseCount
	qSummary.AvgRewriteLatencyMs /= caseCount
	qSummary.AvgRetrieveLatencyMs /= caseCount
	qSummary.AvgRerankLatencyMs /= caseCount
	qSummary.AvgTotalLatencyMs /= caseCount
	qSummary.CacheHitRate /= caseCount
	qSummary.EmptyRate /= caseCount
	qSummary.CitationCoverage /= caseCount
}

const (
	traceKeyDenseRank     = "_trace_dense_rank"
	traceKeyLexicalRank   = "_trace_lexical_rank"
	traceKeyFusionScore   = "_trace_fusion_score"
	traceKeyMetadataBoost = "_trace_metadata_boost"
	traceKeyRerankScore   = "_trace_rerank_score"
)

func SchemaDocsToRetrievedDocs(docs []*schema.Document) []RetrievedDoc {
	if len(docs) == 0 {
		return nil
	}
	results := make([]RetrievedDoc, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		rd := RetrievedDoc{
			ID:      CanonicalSchemaDocID(doc),
			Title:   metadataTitle(doc.MetaData),
			Content: doc.Content,
			Score:   doc.Score(),
			Trace:   docTraceFromMeta(doc.MetaData),
		}
		results = append(results, rd)
	}
	return results
}

func docTraceFromMeta(meta map[string]any) *DocTrace {
	if meta == nil {
		return nil
	}
	t := &DocTrace{}
	hasAny := false
	if v, ok := meta[traceKeyDenseRank].(int); ok {
		t.DenseRank = v
		hasAny = true
	}
	if v, ok := meta[traceKeyLexicalRank].(int); ok {
		t.LexicalRank = v
		hasAny = true
	}
	if v, ok := meta[traceKeyFusionScore].(float64); ok {
		t.FusionScore = v
		hasAny = true
	}
	if v, ok := meta[traceKeyMetadataBoost].(float64); ok {
		t.MetadataBoost = v
		hasAny = true
	}
	if v, ok := meta[traceKeyRerankScore].(float64); ok {
		t.RerankScore = v
		hasAny = true
	}
	if !hasAny {
		return nil
	}
	return t
}

func CanonicalSchemaDocID(doc *schema.Document) string {
	if doc == nil {
		return ""
	}
	if doc.MetaData != nil {
		for _, key := range []string{"case_id", "caseid", "doc_id"} {
			if value, ok := doc.MetaData[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		for _, key := range []string{"_source", "source", "file_name", "filename", "title"} {
			if value, ok := doc.MetaData[key].(string); ok && strings.TrimSpace(value) != "" {
				return canonicalSourceID(value)
			}
		}
	}
	if strings.TrimSpace(doc.ID) != "" {
		return canonicalSourceID(doc.ID)
	}
	return ""
}

func canonicalSourceID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	base := path.Base(normalized)
	ext := path.Ext(base)
	if ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func uniqueOrderedRetrievedIDs(docs []RetrievedDoc) []string {
	if len(docs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(docs))
	out := make([]string, 0, len(docs))
	for _, doc := range docs {
		id := strings.TrimSpace(doc.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
