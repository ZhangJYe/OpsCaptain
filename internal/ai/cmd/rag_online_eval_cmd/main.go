package main

import (
	"SuperBizAgent/internal/ai/loader"
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/ai/rag/eval"
	"SuperBizAgent/internal/ai/retriever"
	inframv "SuperBizAgent/internal/infra/milvus"
	"SuperBizAgent/utility/common"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

func main() {
	if err := common.LoadPreferredEnvFile(); err != nil {
		fmt.Fprintf(os.Stderr, "load environment failed: %v\n", err)
		os.Exit(1)
	}

	evalPath := flag.String("eval", "", "path to eval_cases.jsonl (default: built-in sample cases)")
	datasetRole := flag.String("dataset-role", "", "required dataset role: development or holdout")
	corpusVersion := flag.String("corpus-version", "", "required corpus/index version for external datasets")
	ksRaw := flag.String("ks", "1,3,5", "comma-separated k values, e.g. 1,3,5")
	modeRaw := flag.String("mode", "hybrid", "eval mode: hybrid, hybrid-retrieve, hybrid-rewrite, hybrid-rerank, or hybrid-full")
	limit := flag.Int("limit", 0, "optional limit on number of eval cases")
	perQueryTimeoutMs := flag.Int("timeout-ms", 15000, "per-query timeout in milliseconds")
	continueOnError := flag.Bool("continue-on-error", true, "record failed cases and continue the evaluation")
	outPath := flag.String("out", "", "required path to write the full JSON report")
	baselineReportPath := flag.String("baseline-report", "", "optional comparable baseline report used by the configured gate")
	denseTopK := flag.Int("dense-top-k", 0, "optional hybrid dense top-k override")
	lexicalTopK := flag.Int("lexical-top-k", 0, "optional hybrid lexical top-k override")
	fusionK := flag.Int("fusion-k", 0, "optional RRF fusion k override")
	candidateTopK := flag.Int("candidate-top-k", 0, "optional hybrid candidate top-k override")
	finalTopK := flag.Int("final-top-k", 0, "optional hybrid final top-k override")
	denseWeight := flag.Float64("dense-weight", -1, "optional non-negative dense RRF weight override")
	lexicalWeight := flag.Float64("lexical-weight", -1, "optional non-negative lexical RRF weight override")
	fieldBoostEnabled := flag.Bool("field-boost-enabled", false, "enable knowledge field-aware BM25 and refinement")
	coverageEnabled := flag.Bool("coverage-enabled", false, "enable bounded final result coverage selection")
	intentRefinementEnabled := flag.Bool("intent-refinement-enabled", false, "enable bounded contrast-intent candidate refinement")
	flag.Parse()

	ks, err := parseKs(*ksRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse ks failed: %v\n", err)
		os.Exit(1)
	}

	wantRewrite, wantRerank, isHybrid, useConfigDefaults, err := parseEvalMode(*modeRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse mode failed: %v\n", err)
		os.Exit(1)
	}
	if err := validateRunInputs(*evalPath, *datasetRole, *corpusVersion, *outPath); err != nil {
		fmt.Fprintf(os.Stderr, "invalid evaluation input: %v\n", err)
		os.Exit(1)
	}
	if *perQueryTimeoutMs <= 0 {
		fmt.Fprintln(os.Stderr, "invalid evaluation input: timeout-ms must be positive")
		os.Exit(1)
	}
	ctx := context.Background()

	var cases []eval.EvalCase
	if strings.TrimSpace(*evalPath) == "" {
		cases = eval.SampleCases()
		fmt.Fprintf(os.Stderr, "using %d built-in sample cases\n", len(cases))
	} else {
		cases, err = eval.LoadEvalCasesJSONL(*evalPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load eval cases failed: %v\n", err)
			os.Exit(1)
		}
	}
	if *limit > 0 && *limit < len(cases) {
		cases = cases[:*limit]
	}
	datasetID, datasetFingerprint, err := datasetIdentity(*evalPath, cases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "identify eval dataset failed: %v\n", err)
		os.Exit(1)
	}
	resolvedCorpusVersion := strings.TrimSpace(*corpusVersion)
	if resolvedCorpusVersion == "" {
		resolvedCorpusVersion = "builtin-sample-v1"
	}

	hybridCfg := rag.DefaultHybridConfig(ctx)
	if err := applyHybridOverrides(&hybridCfg, *denseTopK, *lexicalTopK, *fusionK, *candidateTopK, *finalTopK); err != nil {
		fmt.Fprintf(os.Stderr, "invalid hybrid override: %v\n", err)
		os.Exit(1)
	}
	if err := applyRankingOverrides(&hybridCfg, *denseWeight, *lexicalWeight, *fieldBoostEnabled, *coverageEnabled, *intentRefinementEnabled); err != nil {
		fmt.Fprintf(os.Stderr, "invalid ranking override: %v\n", err)
		os.Exit(1)
	}
	effectiveConfig := captureEffectiveConfig(ctx, wantRewrite, wantRerank, useConfigDefaults, hybridCfg, ks, *perQueryTimeoutMs)

	milvusTimeoutMs := configInt(ctx, "milvus.startup_timeout_ms")
	if milvusTimeoutMs <= 0 {
		milvusTimeoutMs = 5000
	}
	milvusCtx, milvusCancel := context.WithTimeout(ctx, time.Duration(milvusTimeoutMs)*time.Millisecond)
	milvusClient, milvusErr := inframv.OpenExistingMilvusClient(milvusCtx)
	milvusCancel()
	if milvusErr != nil {
		fmt.Fprintf(os.Stderr, "open existing milvus failed within %dms: %v\n", milvusTimeoutMs, milvusErr)
		os.Exit(1)
	}
	rag.NewRetrieverFunc = retriever.NewMilvusRetrieverWithClient(milvusClient)

	maxK := ks[len(ks)-1]
	retrieverTopK := common.GetRetrieverTopK(context.Background())
	if retrieverTopK < maxK {
		fmt.Fprintf(os.Stderr, "warning: retriever.top_k=%d is smaller than requested max k=%d; recall will be truncated\n", retrieverTopK, maxK)
	}

	if isHybrid {
		warmupBM25(context.Background(), *evalPath)
	}

	exec := func(ctx context.Context, query string) ([]eval.RetrievedDoc, eval.QueryMetrics, error) {
		start := time.Now()
		queryCtx, cancel := context.WithTimeout(ctx, time.Duration(*perQueryTimeoutMs)*time.Millisecond)
		defer cancel()

		var docs []*schema.Document
		var trace rag.QueryTrace
		var queryErr error

		if !useConfigDefaults {
			queryCtx = rag.WithRewriteOverride(queryCtx, wantRewrite)
			queryCtx = rag.WithRerankOverride(queryCtx, wantRerank)
		}
		queryCtx = rag.WithHybridConfigOverride(queryCtx, hybridCfg)
		docs, trace, queryErr = rag.Query(queryCtx, rag.SharedPool(), query)

		metrics := eval.QueryMetrics{
			CacheHit:          trace.CacheHit,
			InitFailureCached: trace.InitFailureCached,
			InitLatencyMs:     trace.InitLatencyMs,
			RewriteLatencyMs:  trace.RewriteLatencyMs,
			RetrieveLatencyMs: trace.RetrieveLatencyMs,
			RerankLatencyMs:   trace.RerankLatencyMs,
			ResultCount:       trace.ResultCount,
			RewriteAttempted:  trace.RewriteAttempted,
			RewriteApplied:    trace.RewriteApplied,
			RewriteDegraded:   trace.RewriteDegraded,
			RewriteReason:     trace.RewriteReason,
			RerankAttempted:   trace.RerankAttempted,
			RerankApplied:     trace.RerankEnabled,
			RerankDegraded:    trace.RerankDegraded,
			RerankReason:      trace.RerankReason,
			TotalLatencyMs:    time.Since(start).Milliseconds(),
		}
		if trace.Hybrid != nil {
			metrics.DenseLatencyMs = trace.Hybrid.DenseLatencyMs
			metrics.LexicalLatencyMs = trace.Hybrid.LexicalLatencyMs
			metrics.FusionLatencyMs = trace.Hybrid.FusionLatencyMs
			metrics.DenseCount = trace.Hybrid.DenseCount
			metrics.LexicalCount = trace.Hybrid.LexicalCount
			metrics.FusedCount = trace.Hybrid.FusedCount
			metrics.CandidateCount = trace.Hybrid.CandidateCount
			metrics.Stages = eval.RetrievalStages{
				Dense: append([]string(nil), trace.Hybrid.DenseIDs...), Lexical: append([]string(nil), trace.Hybrid.LexicalIDs...),
				Fusion: append([]string(nil), trace.Hybrid.FusionIDs...), Candidate: append([]string(nil), trace.Hybrid.CandidateIDs...),
				Final: append([]string(nil), trace.Hybrid.FinalIDs...),
			}
			metrics.Intent = eval.IntentMetrics{Parsed: trace.Hybrid.Intent.Parsed, Applied: trace.Hybrid.Intent.Applied, Rule: trace.Hybrid.Intent.Rule, PenalizedDocs: trace.Hybrid.Intent.PenalizedDocs}
		}
		if queryErr != nil {
			return nil, metrics, queryErr
		}
		return eval.SchemaDocsToRetrievedDocs(docs), metrics, nil
	}

	if *modeRaw == "planner" {
		plannerCfg := rag.LoadPlannerConfig(context.Background())
		plannerCfg.Enabled = true
		exec = func(ctx context.Context, query string) ([]eval.RetrievedDoc, eval.QueryMetrics, error) {
			start := time.Now()
			queryCtx, cancel := context.WithTimeout(ctx, time.Duration(*perQueryTimeoutMs)*time.Millisecond)
			defer cancel()

			docs, merged, err := rag.QueryWithPlanner(queryCtx, rag.SharedPool(), query, plannerCfg)
			if err != nil {
				return nil, eval.QueryMetrics{}, err
			}

			metrics := eval.QueryMetrics{
				TotalLatencyMs: time.Since(start).Milliseconds(),
				ResultCount:    len(docs),
				Decomposed:     merged.Trace.Analyzed && merged.Trace.SubQueryCount > 0,
				SubQueryCount:  merged.Trace.SubQueryCount,
				PlanLatencyMs:  merged.Trace.PlanLatencyMs,
				MergeLatencyMs: merged.Trace.MergeLatencyMs,
			}
			return eval.SchemaDocsToRetrievedDocs(docs), metrics, nil
		}
	}

	if *modeRaw == "agent" {
		agentCfg := rag.LoadAgentConfig(context.Background())
		agentCfg.Enabled = true
		agent := rag.NewAgentRAG(agentCfg)
		exec = func(ctx context.Context, query string) ([]eval.RetrievedDoc, eval.QueryMetrics, error) {
			start := time.Now()
			queryCtx, cancel := context.WithTimeout(ctx, time.Duration(*perQueryTimeoutMs)*time.Millisecond)
			defer cancel()
			docs, agentTrace, err := agent.Query(queryCtx, rag.SharedPool(), query)
			if err != nil {
				return nil, eval.QueryMetrics{}, err
			}
			metrics := eval.QueryMetrics{
				TotalLatencyMs:  time.Since(start).Milliseconds(),
				ResultCount:     len(docs),
				AgentRounds:     agentTrace.Rounds,
				FinalConfidence: agentTrace.FinalConfidence,
				AgentLatencyMs:  agentTrace.TotalLatencyMs,
			}
			return eval.SchemaDocsToRetrievedDocs(docs), metrics, nil
		}
	}

	summary, results, err := eval.RunQueryEvalWithOpts(context.Background(), exec, cases, ks, eval.RunOptions{ContinueOnError: *continueOnError})
	if err != nil {
		fmt.Fprintf(os.Stderr, "run online eval failed: %v\n", err)
		os.Exit(1)
	}

	itemReport := report{
		SchemaVersion:   reportSchemaVersion,
		Mode:            strings.ToLower(strings.TrimSpace(*modeRaw)),
		Metadata:        newReportMetadata(*datasetRole, datasetID, datasetFingerprint, resolvedCorpusVersion),
		EffectiveConfig: effectiveConfig,
		Summary:         summary,
		Analysis:        eval.BuildStratifiedAnalysis(cases, results, summary.Failures, ks),
		Results:         results,
	}
	if strings.TrimSpace(*baselineReportPath) != "" {
		baseline, err := loadReport(*baselineReportPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load baseline failed: %v\n", err)
			os.Exit(1)
		}
		gate, err := compareReports(baseline, itemReport, loadGateConfig(context.Background()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "compare reports failed: %v\n", err)
			os.Exit(1)
		}
		itemReport.Gate = &gate
	}

	printSummary(*modeRaw, summary, ks)

	if summary.Failed > 0 {
		fmt.Fprintf(os.Stderr, "\n--- %d failures ---\n", summary.Failed)
		for _, f := range summary.Failures {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.CaseID, f.Error)
		}
	}

	raw, err := json.MarshalIndent(itemReport, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal report failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir report dir failed: %v\n", err)
		os.Exit(1)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write report failed: %v\n", err)
		os.Exit(1)
	}
	if itemReport.Gate != nil {
		printGate(*itemReport.Gate)
		if !itemReport.Gate.Passed {
			os.Exit(2)
		}
	}
}

func applyRankingOverrides(cfg *rag.HybridConfig, denseWeight, lexicalWeight float64, fieldBoostEnabled, coverageEnabled, intentRefinementEnabled bool) error {
	if denseWeight < -1 || lexicalWeight < -1 {
		return fmt.Errorf("RRF weights must be non-negative")
	}
	if denseWeight >= 0 {
		cfg.DenseWeight = denseWeight
	}
	if lexicalWeight >= 0 {
		cfg.LexicalWeight = lexicalWeight
	}
	if cfg.DenseWeight < 0 || cfg.LexicalWeight < 0 || cfg.DenseWeight == 0 && cfg.LexicalWeight == 0 {
		return fmt.Errorf("RRF weights must be non-negative and at least one must be positive")
	}
	if fieldBoostEnabled {
		cfg.KnowledgeFieldBoostEnabled = true
	}
	if coverageEnabled {
		cfg.CoverageEnabled = true
	}
	if intentRefinementEnabled {
		cfg.IntentRefinementEnabled = true
	}
	return nil
}

func parseEvalMode(raw string) (wantRewrite, wantRerank, isHybrid, useConfigDefaults bool, err error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "hybrid":
		return false, false, true, true, nil
	case "hybrid-retrieve":
		return false, false, true, false, nil
	case "hybrid-rewrite":
		return true, false, true, false, nil
	case "hybrid-rerank":
		return false, true, true, false, nil
	case "hybrid-full":
		return true, true, true, false, nil
	case "retrieve":
		return false, false, true, false, nil
	case "rewrite":
		return true, false, true, false, nil
	case "rerank":
		return false, true, true, false, nil
	case "full":
		return true, true, true, false, nil
	case "planner":
		return false, false, false, true, nil
	case "agent":
		return false, false, false, true, nil
	default:
		return false, false, false, false, fmt.Errorf("unknown eval mode %q", raw)
	}
}

func applyHybridOverrides(cfg *rag.HybridConfig, denseTopK, lexicalTopK, fusionK, candidateTopK, finalTopK int) error {
	values := []struct {
		name  string
		value int
		apply func(int)
	}{
		{name: "dense-top-k", value: denseTopK, apply: func(v int) { cfg.DenseTopK = v }},
		{name: "lexical-top-k", value: lexicalTopK, apply: func(v int) { cfg.LexicalTopK = v }},
		{name: "fusion-k", value: fusionK, apply: func(v int) { cfg.FusionK = v }},
		{name: "candidate-top-k", value: candidateTopK, apply: func(v int) { cfg.CandidateTopK = v }},
		{name: "final-top-k", value: finalTopK, apply: func(v int) { cfg.FinalTopK = v }},
	}
	for _, item := range values {
		if item.value < 0 {
			return fmt.Errorf("%s cannot be negative", item.name)
		}
		if item.value > 0 {
			item.apply(item.value)
		}
	}
	if cfg.CandidateTopK < cfg.FinalTopK {
		return fmt.Errorf("candidate-top-k (%d) must be >= final-top-k (%d)", cfg.CandidateTopK, cfg.FinalTopK)
	}
	return nil
}

func warmupBM25(ctx context.Context, evalPath string) {
	var dirs []string
	if strings.TrimSpace(evalPath) != "" {
		docsDir := filepath.Dir(filepath.Dir(evalPath))
		dirs = []string{filepath.Join(docsDir, "evidence"), filepath.Join(docsDir, "history")}
	}
	if fileDir := strings.TrimSpace(common.FileDir); fileDir != "" {
		dirs = append(dirs, fileDir)
	}
	idx := rag.SharedBM25Index()
	count := 0
	for _, dir := range dirs {
		for _, path := range markdownFiles(dir) {
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			doc := &schema.Document{Content: string(raw)}
			loader.EnrichMarkdownDocument(path, raw, doc)
			rag.AddDocToBM25Index(idx, doc)
			count++
		}
	}
	fmt.Fprintf(os.Stderr, "BM25 warm-up: indexed %d docs\n", count)
}

func markdownFiles(root string) []string {
	var paths []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	return paths
}

func parseKs(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	ks := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid k %q: %w", part, err)
		}
		if value <= 0 {
			return nil, fmt.Errorf("k must be positive, got %d", value)
		}
		ks = append(ks, value)
	}
	sort.Ints(ks)
	return ks, nil
}

func printSummary(mode string, summary eval.QuerySummary, ks []int) {
	fmt.Println("========================================")
	fmt.Println("  RAG Online Baseline Report")
	fmt.Println("========================================")
	fmt.Printf("  Mode         : %s\n", mode)
	fmt.Printf("  Cases        : %d (ok=%d, fail=%d)\n", summary.Cases, summary.Succeeded, summary.Failed)
	fmt.Printf("  MRR          : %.4f\n", summary.MRR)
	fmt.Printf("  Citation Cov : %.2f%%\n", summary.CitationCoverage*100)
	fmt.Printf("  Empty Rate   : %.2f%%\n", summary.EmptyRate*100)
	fmt.Printf("  Failure Rate : %.2f%%\n", summary.FailureRate*100)
	fmt.Printf("  Cache Hit    : %.2f%%\n", summary.CacheHitRate*100)
	fmt.Printf("  Avg Init ms  : %.2f\n", summary.AvgInitLatencyMs)
	fmt.Printf("  Avg Rewrite  : %.2f\n", summary.AvgRewriteLatencyMs)
	fmt.Printf("  Avg Retrieve : %.2f\n", summary.AvgRetrieveLatencyMs)
	fmt.Printf("  Avg Rerank   : %.2f\n", summary.AvgRerankLatencyMs)
	fmt.Printf("  Avg Total ms : %.2f\n", summary.AvgTotalLatencyMs)
	fmt.Printf("  P50/P95 ms   : %.2f / %.2f\n", summary.Latency.Total.P50Ms, summary.Latency.Total.P95Ms)
	if summary.RewriteAttempted > 0 {
		fmt.Printf("  Rewrite      : applied=%d/%d degraded=%d (%.2f%%)\n", summary.RewriteApplied, summary.RewriteAttempted, summary.RewriteDegraded, summary.RewriteDegradedRate*100)
	}
	if summary.RerankAttempted > 0 {
		fmt.Printf("  Rerank       : applied=%d/%d degraded=%d (%.2f%%)\n", summary.RerankApplied, summary.RerankAttempted, summary.RerankDegraded, summary.RerankDegradedRate*100)
	}
	fmt.Printf("  Decomposed   : %d/%d (%.1f%%)\n", summary.DecomposedCount, summary.Cases, float64(summary.DecomposedCount)/float64(summary.Cases)*100)
	if summary.DecomposedCount > 0 {
		fmt.Printf("  Avg Plan ms  : %.2f\n", summary.AvgPlanLatencyMs)
	}
	fmt.Println("========================================")

	fmt.Printf("%-12s", "Metric")
	for _, k := range ks {
		fmt.Printf("  @%-5d", k)
	}
	fmt.Println()

	fmt.Printf("%-12s", "Avg Recall")
	for _, k := range ks {
		fmt.Printf("  %-6.2f", summary.AvgRecallAtK[k])
	}
	fmt.Println()

	fmt.Printf("%-12s", "Hit Rate")
	for _, k := range ks {
		fmt.Printf("  %-6.2f", summary.HitRateAtK[k])
	}
	fmt.Println()

	fmt.Printf("%-12s", "Full Recall")
	for _, k := range ks {
		fmt.Printf("  %-3d/%d ", summary.FullRecallAtK[k], summary.Cases)
	}
	fmt.Println()
}

func printGate(result gateResult) {
	fmt.Println("========================================")
	fmt.Printf("  RAG Gate     : %t\n", result.Passed)
	for _, check := range result.Checks {
		fmt.Printf("  %-24s passed=%-5t baseline=%.4f candidate=%.4f delta=%.4f limit=%.4f\n",
			check.Name, check.Passed, check.Baseline, check.Candidate, check.Delta, check.Limit)
	}
	fmt.Println("========================================")
}
