package main

import (
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

type report struct {
	Mode    string                 `json:"mode"`
	Summary eval.QuerySummary      `json:"summary"`
	Results []eval.QueryCaseResult `json:"results"`
}

func main() {
	evalPath := flag.String("eval", "", "path to eval_cases.jsonl (default: built-in sample cases)")
	ksRaw := flag.String("ks", "1,3,5", "comma-separated k values, e.g. 1,3,5")
	modeRaw := flag.String("mode", "hybrid", "eval mode: hybrid, retrieve, rewrite, or full")
	limit := flag.Int("limit", 0, "optional limit on number of eval cases")
	perQueryTimeoutMs := flag.Int("timeout-ms", 15000, "per-query timeout in milliseconds")
	outPath := flag.String("out", "", "optional path to write full JSON report")
	flag.Parse()

	ks, err := parseKs(*ksRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse ks failed: %v\n", err)
		os.Exit(1)
	}

	wantRewrite, wantRerank, isHybrid, useConfigDefaults := parseEvalMode(*modeRaw)
	ctx := context.Background()
	milvusClient, milvusErr := inframv.NewMilvusClient(ctx)
	if milvusErr != nil {
		fmt.Fprintf(os.Stderr, "milvus client init failed: %v\n", milvusErr)
		os.Exit(1)
	}
	rag.NewRetrieverFunc = retriever.NewMilvusRetrieverWithClient(milvusClient)

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

		if isHybrid {
			if !useConfigDefaults {
				queryCtx = rag.WithRewriteOverride(queryCtx, wantRewrite)
				queryCtx = rag.WithRerankOverride(queryCtx, wantRerank)
			}
			docs, trace, queryErr = rag.Query(queryCtx, rag.SharedPool(), query)
		} else {
			docs, trace, queryErr = rag.QueryForEval(queryCtx, rag.SharedPool(), query, wantRewrite, wantRerank)
		}

		metrics := eval.QueryMetrics{
			CacheHit:          trace.CacheHit,
			InitFailureCached: trace.InitFailureCached,
			InitLatencyMs:     trace.InitLatencyMs,
			RewriteLatencyMs:  trace.RewriteLatencyMs,
			RetrieveLatencyMs: trace.RetrieveLatencyMs,
			RerankLatencyMs:   trace.RerankLatencyMs,
			ResultCount:       trace.ResultCount,
			TotalLatencyMs:    time.Since(start).Milliseconds(),
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

	summary, results, err := eval.RunQueryEval(context.Background(), exec, cases, ks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run online eval failed: %v\n", err)
		os.Exit(1)
	}

	printSummary(*modeRaw, summary, ks)

	if summary.Failed > 0 {
		fmt.Fprintf(os.Stderr, "\n--- %d failures ---\n", summary.Failed)
		for _, f := range summary.Failures {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.CaseID, f.Error)
		}
	}

	if strings.TrimSpace(*outPath) != "" {
		raw, err := json.MarshalIndent(report{Mode: *modeRaw, Summary: summary, Results: results}, "", "  ")
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
	}
}

func parseEvalMode(raw string) (wantRewrite, wantRerank, isHybrid, useConfigDefaults bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "hybrid":
		return false, false, true, true
	case "hybrid-retrieve":
		return false, false, true, false
	case "hybrid-rerank":
		return false, true, true, false
	case "retrieve":
		return false, false, false, false
	case "rewrite":
		return true, false, false, false
	case "rerank":
		return false, true, false, false
	case "full":
		return true, true, false, false
	case "planner":
		return false, false, false, true
	case "agent":
		return false, false, false, true
	default:
		fmt.Fprintf(os.Stderr, "unknown eval mode %q, falling back to hybrid\n", raw)
		return false, false, true, true
	}
}

func warmupBM25(ctx context.Context, evalPath string) {
	var dirs []string
	if strings.TrimSpace(evalPath) != "" {
		docsDir := filepath.Dir(filepath.Dir(evalPath))
		dirs = []string{filepath.Join(docsDir, "evidence"), filepath.Join(docsDir, "history")}
	} else {
		fileDir := common.FileDir
		if fileDir != "" {
			dirs = []string{fileDir}
		}
	}
	idx := rag.SharedBM25Index()
	count := 0
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			docID := strings.TrimSuffix(entry.Name(), ".md")
			idx.AddDocument(docID, string(raw), map[string]string{"_source": entry.Name()})
			count++
		}
	}
	fmt.Fprintf(os.Stderr, "BM25 warm-up: indexed %d docs\n", count)
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
	fmt.Printf("  Cache Hit    : %.2f%%\n", summary.CacheHitRate*100)
	fmt.Printf("  Avg Init ms  : %.2f\n", summary.AvgInitLatencyMs)
	fmt.Printf("  Avg Rewrite  : %.2f\n", summary.AvgRewriteLatencyMs)
	fmt.Printf("  Avg Retrieve : %.2f\n", summary.AvgRetrieveLatencyMs)
	fmt.Printf("  Avg Rerank   : %.2f\n", summary.AvgRerankLatencyMs)
	fmt.Printf("  Avg Total ms : %.2f\n", summary.AvgTotalLatencyMs)
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
