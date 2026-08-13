package eval

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type QueryMetrics struct {
	CacheHit          bool            `json:"cache_hit"`
	InitFailureCached bool            `json:"init_failure_cached"`
	InitLatencyMs     int64           `json:"init_latency_ms"`
	RewriteLatencyMs  int64           `json:"rewrite_latency_ms"`
	RetrieveLatencyMs int64           `json:"retrieve_latency_ms"`
	DenseLatencyMs    int64           `json:"dense_latency_ms"`
	LexicalLatencyMs  int64           `json:"lexical_latency_ms"`
	FusionLatencyMs   int64           `json:"fusion_latency_ms"`
	RerankLatencyMs   int64           `json:"rerank_latency_ms"`
	DenseCount        int             `json:"dense_count"`
	LexicalCount      int             `json:"lexical_count"`
	FusedCount        int             `json:"fused_count"`
	CandidateCount    int             `json:"candidate_count"`
	ResultCount       int             `json:"result_count"`
	RewriteAttempted  bool            `json:"rewrite_attempted"`
	RewriteApplied    bool            `json:"rewrite_applied"`
	RewriteDegraded   bool            `json:"rewrite_degraded"`
	RewriteReason     string          `json:"rewrite_reason,omitempty"`
	RerankAttempted   bool            `json:"rerank_attempted"`
	RerankApplied     bool            `json:"rerank_applied"`
	RerankDegraded    bool            `json:"rerank_degraded"`
	RerankReason      string          `json:"rerank_reason,omitempty"`
	TotalLatencyMs    int64           `json:"total_latency_ms"`
	Decomposed        bool            `json:"decomposed"`
	SubQueryCount     int             `json:"sub_query_count"`
	PlanLatencyMs     int64           `json:"plan_latency_ms"`
	MergeLatencyMs    int64           `json:"merge_latency_ms"`
	AgentRounds       int             `json:"agent_rounds"`
	FinalConfidence   float64         `json:"final_confidence"`
	AgentLatencyMs    int64           `json:"agent_latency_ms"`
	Stages            RetrievalStages `json:"stages"`
	Intent            IntentMetrics   `json:"intent"`
}

type RetrievalStages struct {
	Dense     []string `json:"dense,omitempty"`
	Lexical   []string `json:"lexical,omitempty"`
	Fusion    []string `json:"fusion,omitempty"`
	Candidate []string `json:"candidate,omitempty"`
	Final     []string `json:"final,omitempty"`
}

type IntentMetrics struct {
	Parsed        bool   `json:"parsed"`
	Applied       bool   `json:"applied"`
	Rule          string `json:"rule,omitempty"`
	PenalizedDocs int    `json:"penalized_docs"`
}

type QueryExecutor func(context.Context, string) ([]RetrievedDoc, QueryMetrics, error)

type QueryCaseResult struct {
	CaseResult
	Metrics          QueryMetrics         `json:"metrics"`
	StageDiagnostics CaseStageDiagnostics `json:"stage_diagnostics"`
}

type LatencyStats struct {
	AvgMs float64 `json:"avg_ms"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
}

type QueryLatencySummary struct {
	Init     LatencyStats `json:"init"`
	Rewrite  LatencyStats `json:"rewrite"`
	Retrieve LatencyStats `json:"retrieve"`
	Dense    LatencyStats `json:"dense"`
	Lexical  LatencyStats `json:"lexical"`
	Fusion   LatencyStats `json:"fusion"`
	Rerank   LatencyStats `json:"rerank"`
	Total    LatencyStats `json:"total"`
}

type QuerySummary struct {
	Summary
	AvgInitLatencyMs     float64                    `json:"avg_init_latency_ms"`
	AvgRewriteLatencyMs  float64                    `json:"avg_rewrite_latency_ms"`
	AvgRetrieveLatencyMs float64                    `json:"avg_retrieve_latency_ms"`
	AvgRerankLatencyMs   float64                    `json:"avg_rerank_latency_ms"`
	AvgTotalLatencyMs    float64                    `json:"avg_total_latency_ms"`
	CacheHitRate         float64                    `json:"cache_hit_rate"`
	EmptyRate            float64                    `json:"empty_rate"`
	FailureRate          float64                    `json:"failure_rate"`
	CitationCoverage     float64                    `json:"citation_coverage"`
	RewriteAttempted     int                        `json:"rewrite_attempted"`
	RewriteApplied       int                        `json:"rewrite_applied"`
	RewriteDegraded      int                        `json:"rewrite_degraded"`
	RewriteApplyRate     float64                    `json:"rewrite_apply_rate"`
	RewriteDegradedRate  float64                    `json:"rewrite_degraded_rate"`
	RerankAttempted      int                        `json:"rerank_attempted"`
	RerankApplied        int                        `json:"rerank_applied"`
	RerankDegraded       int                        `json:"rerank_degraded"`
	RerankApplyRate      float64                    `json:"rerank_apply_rate"`
	RerankDegradedRate   float64                    `json:"rerank_degraded_rate"`
	Latency              QueryLatencySummary        `json:"latency"`
	AvgPlanLatencyMs     float64                    `json:"avg_plan_latency_ms"`
	DecomposedCount      int                        `json:"decomposed_count"`
	AvgAgentRounds       float64                    `json:"avg_agent_rounds"`
	AvgFinalConfidence   float64                    `json:"avg_final_confidence"`
	AvgAgentLatencyMs    float64                    `json:"avg_agent_latency_ms"`
	StageRecallAtK       map[string]map[int]float64 `json:"stage_recall_at_k"`
	StageGapCounts       map[string]int             `json:"stage_gap_counts"`
	IntentParsed         int                        `json:"intent_parsed"`
	IntentApplied        int                        `json:"intent_applied"`
	IntentPenalizedDocs  int                        `json:"intent_penalized_docs"`
	IntentParseRate      float64                    `json:"intent_parse_rate"`
	IntentApplyRate      float64                    `json:"intent_apply_rate"`
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
	metricSamples := make([]QueryMetrics, 0, len(cases))
	qSummary := QuerySummary{
		Summary:        newSummary(len(cases), ks),
		StageRecallAtK: newStageRecallSummary(),
		StageGapCounts: make(map[string]int),
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
		stageDiagnostics := BuildCaseStageDiagnostics(evalCase.RelevantIDs, metrics.Stages)
		accumulateMetrics(&qSummary.Summary, caseResult, evalCase, ks)
		accumulateStageDiagnostics(&qSummary, stageDiagnostics)
		qSummary.Succeeded++

		accumulateQueryMetrics(&qSummary, metrics, rankedIDs)
		metricSamples = append(metricSamples, metrics)

		results = append(results, QueryCaseResult{
			CaseResult:       caseResult,
			Metrics:          metrics,
			StageDiagnostics: stageDiagnostics,
		})
	}

	finalizeQuerySummary(&qSummary, ks, metricSamples)
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
	if metrics.RewriteAttempted {
		qSummary.RewriteAttempted++
	}
	if metrics.RewriteApplied {
		qSummary.RewriteApplied++
	}
	if metrics.RewriteDegraded {
		qSummary.RewriteDegraded++
	}
	if metrics.RerankAttempted {
		qSummary.RerankAttempted++
	}
	if metrics.RerankApplied {
		qSummary.RerankApplied++
	}
	if metrics.RerankDegraded {
		qSummary.RerankDegraded++
	}
	if metrics.Intent.Parsed {
		qSummary.IntentParsed++
	}
	if metrics.Intent.Applied {
		qSummary.IntentApplied++
	}
	qSummary.IntentPenalizedDocs += metrics.Intent.PenalizedDocs
	if len(rankedIDs) == 0 {
		qSummary.EmptyRate++
	} else {
		qSummary.CitationCoverage++
	}
	if metrics.Decomposed {
		qSummary.AvgPlanLatencyMs += float64(metrics.PlanLatencyMs)
		qSummary.DecomposedCount++
	}
	qSummary.AvgAgentRounds += float64(metrics.AgentRounds)
	qSummary.AvgFinalConfidence += metrics.FinalConfidence
	qSummary.AvgAgentLatencyMs += float64(metrics.AgentLatencyMs)
}

func finalizeQuerySummary(qSummary *QuerySummary, ks []int, samples []QueryMetrics) {
	finalizeSummary(&qSummary.Summary, ks)
	if qSummary.Cases > 0 {
		qSummary.FailureRate = float64(qSummary.Failed) / float64(qSummary.Cases)
	}
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
	if qSummary.RewriteAttempted > 0 {
		qSummary.RewriteApplyRate = float64(qSummary.RewriteApplied) / float64(qSummary.RewriteAttempted)
		qSummary.RewriteDegradedRate = float64(qSummary.RewriteDegraded) / float64(qSummary.RewriteAttempted)
	}
	if qSummary.RerankAttempted > 0 {
		qSummary.RerankApplyRate = float64(qSummary.RerankApplied) / float64(qSummary.RerankAttempted)
		qSummary.RerankDegradedRate = float64(qSummary.RerankDegraded) / float64(qSummary.RerankAttempted)
	}
	if qSummary.DecomposedCount > 0 {
		qSummary.AvgPlanLatencyMs /= float64(qSummary.DecomposedCount)
	}
	qSummary.AvgAgentRounds /= caseCount
	qSummary.AvgFinalConfidence /= caseCount
	qSummary.AvgAgentLatencyMs /= caseCount
	for _, recallByK := range qSummary.StageRecallAtK {
		for k, value := range recallByK {
			recallByK[k] = value / caseCount
		}
	}
	qSummary.IntentParseRate = float64(qSummary.IntentParsed) / caseCount
	if qSummary.IntentParsed > 0 {
		qSummary.IntentApplyRate = float64(qSummary.IntentApplied) / float64(qSummary.IntentParsed)
	}

	qSummary.Latency = QueryLatencySummary{
		Init:     latencyStats(samples, func(m QueryMetrics) int64 { return m.InitLatencyMs }),
		Rewrite:  latencyStats(samples, func(m QueryMetrics) int64 { return m.RewriteLatencyMs }),
		Retrieve: latencyStats(samples, func(m QueryMetrics) int64 { return m.RetrieveLatencyMs }),
		Dense:    latencyStats(samples, func(m QueryMetrics) int64 { return m.DenseLatencyMs }),
		Lexical:  latencyStats(samples, func(m QueryMetrics) int64 { return m.LexicalLatencyMs }),
		Fusion:   latencyStats(samples, func(m QueryMetrics) int64 { return m.FusionLatencyMs }),
		Rerank:   latencyStats(samples, func(m QueryMetrics) int64 { return m.RerankLatencyMs }),
		Total:    latencyStats(samples, func(m QueryMetrics) int64 { return m.TotalLatencyMs }),
	}
}

func latencyStats(samples []QueryMetrics, value func(QueryMetrics) int64) LatencyStats {
	if len(samples) == 0 {
		return LatencyStats{}
	}
	values := make([]int64, 0, len(samples))
	var total int64
	for _, sample := range samples {
		v := value(sample)
		values = append(values, v)
		total += v
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return LatencyStats{
		AvgMs: float64(total) / float64(len(values)),
		P50Ms: float64(nearestRank(values, 50)),
		P95Ms: float64(nearestRank(values, 95)),
	}
}

func nearestRank(sortedValues []int64, percentile int) int64 {
	if len(sortedValues) == 0 {
		return 0
	}
	index := (len(sortedValues)*percentile+99)/100 - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedValues) {
		index = len(sortedValues) - 1
	}
	return sortedValues[index]
}

const (
	traceKeyDenseRank          = "_trace_dense_rank"
	traceKeyLexicalRank        = "_trace_lexical_rank"
	traceKeyFusionScore        = "_trace_fusion_score"
	traceKeyMetadataBoost      = "_trace_metadata_boost"
	traceKeyRerankScore        = "_trace_rerank_score"
	traceKeyFusionPosition     = "_trace_fusion_position"
	traceKeyRefinePosition     = "_trace_refine_position"
	traceKeyFinalPosition      = "_trace_final_position"
	traceKeyFieldBoost         = "_trace_field_boost"
	traceKeyFieldMatches       = "_trace_field_matches"
	traceKeyCoverageBoost      = "_trace_coverage_boost"
	traceKeyIntentRule         = "_trace_intent_rule"
	traceKeyIntentPositiveHits = "_trace_intent_positive_hits"
	traceKeyIntentExcludedHits = "_trace_intent_excluded_hits"
	traceKeyIntentBonus        = "_trace_intent_bonus"
	traceKeyIntentPenalty      = "_trace_intent_penalty"
	traceKeyIntentNetScore     = "_trace_intent_net_score"
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
	if v, ok := meta[traceKeyFusionPosition].(int); ok {
		t.FusionPosition = v
		hasAny = true
	}
	if v, ok := meta[traceKeyRefinePosition].(int); ok {
		t.RefinePosition = v
		hasAny = true
	}
	if v, ok := meta[traceKeyFinalPosition].(int); ok {
		t.FinalPosition = v
		hasAny = true
	}
	if v, ok := meta[traceKeyFieldBoost].(float64); ok {
		t.FieldBoost = v
		hasAny = true
	}
	if v, ok := meta[traceKeyFieldMatches].([]string); ok {
		t.FieldMatches = append([]string(nil), v...)
		hasAny = true
	}
	if v, ok := meta[traceKeyCoverageBoost].(float64); ok {
		t.CoverageBoost = v
		hasAny = true
	}
	if v, ok := meta[traceKeyIntentRule].(string); ok && v != "" {
		t.IntentRule = v
		hasAny = true
	}
	if v, ok := meta[traceKeyIntentPositiveHits].([]string); ok {
		t.IntentPositiveHits = append([]string(nil), v...)
		hasAny = true
	}
	if v, ok := meta[traceKeyIntentExcludedHits].([]string); ok {
		t.IntentExcludedHits = append([]string(nil), v...)
		hasAny = true
	}
	if v, ok := meta[traceKeyIntentBonus].(float64); ok {
		t.IntentBonus = v
		hasAny = true
	}
	if v, ok := meta[traceKeyIntentPenalty].(float64); ok {
		t.IntentPenalty = v
		hasAny = true
	}
	if v, ok := meta[traceKeyIntentNetScore].(float64); ok {
		t.IntentNetScore = v
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
		for _, key := range []string{"case_id", "caseid", "doc_id", "knowledge_doc_id"} {
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
