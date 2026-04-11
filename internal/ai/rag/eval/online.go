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
}

func RunQueryEval(ctx context.Context, exec QueryExecutor, cases []EvalCase, ks []int) (QuerySummary, []QueryCaseResult, error) {
	if exec == nil {
		return QuerySummary{}, nil, fmt.Errorf("query executor is nil")
	}
	ks = normalizeKs(ks)
	if len(ks) == 0 {
		return QuerySummary{}, nil, fmt.Errorf("ks is empty")
	}

	results := make([]QueryCaseResult, 0, len(cases))
	summary := QuerySummary{
		Summary: Summary{
			Cases:         len(cases),
			AvgRecallAtK:  make(map[int]float64, len(ks)),
			HitRateAtK:    make(map[int]float64, len(ks)),
			FullRecallAtK: make(map[int]int, len(ks)),
		},
	}

	for _, evalCase := range cases {
		rankedDocs, metrics, err := exec(ctx, evalCase.Query)
		if err != nil {
			return QuerySummary{}, nil, fmt.Errorf("case %s query failed: %w", evalCase.ID, err)
		}
		rankedIDs := uniqueOrderedRetrievedIDs(rankedDocs)

		result := QueryCaseResult{
			CaseResult: CaseResult{
				CaseID:      evalCase.ID,
				Query:       evalCase.Query,
				RelevantIDs: append([]string(nil), evalCase.RelevantIDs...),
				RankedIDs:   rankedIDs,
				HitIDsByK:   make(map[int][]string, len(ks)),
				RecallAtK:   make(map[int]float64, len(ks)),
			},
			Metrics: metrics,
		}

		for _, k := range ks {
			hits := hitIDs(evalCase.RelevantIDs, rankedIDs, k)
			recall := computeRecall(evalCase.RelevantIDs, hits)
			result.HitIDsByK[k] = hits
			result.RecallAtK[k] = recall
			summary.AvgRecallAtK[k] += recall
			if len(hits) > 0 {
				summary.HitRateAtK[k]++
			}
			if len(hits) == len(uniqueIDs(evalCase.RelevantIDs)) && len(evalCase.RelevantIDs) > 0 {
				summary.FullRecallAtK[k]++
			}
		}

		summary.AvgInitLatencyMs += float64(metrics.InitLatencyMs)
		summary.AvgRewriteLatencyMs += float64(metrics.RewriteLatencyMs)
		summary.AvgRetrieveLatencyMs += float64(metrics.RetrieveLatencyMs)
		summary.AvgRerankLatencyMs += float64(metrics.RerankLatencyMs)
		summary.AvgTotalLatencyMs += float64(metrics.TotalLatencyMs)
		if metrics.CacheHit {
			summary.CacheHitRate++
		}
		if len(rankedIDs) == 0 {
			summary.EmptyRate++
		}

		results = append(results, result)
	}

	if len(cases) == 0 {
		return summary, results, nil
	}

	caseCount := float64(len(cases))
	for _, k := range ks {
		summary.AvgRecallAtK[k] /= caseCount
		summary.HitRateAtK[k] /= caseCount
	}
	summary.AvgInitLatencyMs /= caseCount
	summary.AvgRewriteLatencyMs /= caseCount
	summary.AvgRetrieveLatencyMs /= caseCount
	summary.AvgRerankLatencyMs /= caseCount
	summary.AvgTotalLatencyMs /= caseCount
	summary.CacheHitRate /= caseCount
	summary.EmptyRate /= caseCount

	return summary, results, nil
}

func SchemaDocsToRetrievedDocs(docs []*schema.Document) []RetrievedDoc {
	if len(docs) == 0 {
		return nil
	}
	results := make([]RetrievedDoc, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		results = append(results, RetrievedDoc{
			ID:      CanonicalSchemaDocID(doc),
			Title:   metadataTitle(doc.MetaData),
			Content: doc.Content,
			Score:   doc.Score(),
		})
	}
	return results
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
