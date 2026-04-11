package main

import (
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/ai/rag/eval"
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
)

type report struct {
	Summary eval.QuerySummary      `json:"summary"`
	Results []eval.QueryCaseResult `json:"results"`
}

func main() {
	evalPath := flag.String("eval", filepath.Join(".", "aiopschallenge2025", "baseline", "eval", "eval_cases.jsonl"), "path to eval_cases.jsonl")
	ksRaw := flag.String("ks", "1,3,5", "comma-separated k values, e.g. 1,3,5")
	limit := flag.Int("limit", 0, "optional limit on number of eval cases")
	perQueryTimeoutMs := flag.Int("timeout-ms", 15000, "per-query timeout in milliseconds")
	outPath := flag.String("out", "", "optional path to write full JSON report")
	flag.Parse()

	ks, err := parseKs(*ksRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse ks failed: %v\n", err)
		os.Exit(1)
	}

	cases, err := eval.LoadEvalCasesJSONL(*evalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load eval cases failed: %v\n", err)
		os.Exit(1)
	}
	if *limit > 0 && *limit < len(cases) {
		cases = cases[:*limit]
	}

	maxK := ks[len(ks)-1]
	retrieverTopK := common.GetRetrieverTopK(context.Background())
	if retrieverTopK < maxK {
		fmt.Fprintf(os.Stderr, "warning: retriever.top_k=%d is smaller than requested max k=%d; recall will be truncated\n", retrieverTopK, maxK)
	}

	exec := func(ctx context.Context, query string) ([]eval.RetrievedDoc, eval.QueryMetrics, error) {
		start := time.Now()
		queryCtx, cancel := context.WithTimeout(ctx, time.Duration(*perQueryTimeoutMs)*time.Millisecond)
		defer cancel()

		docs, trace, err := rag.Query(queryCtx, rag.SharedPool(), query)
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
		if err != nil {
			return nil, metrics, err
		}
		return eval.SchemaDocsToRetrievedDocs(docs), metrics, nil
	}

	summary, results, err := eval.RunQueryEval(context.Background(), exec, cases, ks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run online eval failed: %v\n", err)
		os.Exit(1)
	}

	printSummary(summary, ks)

	if strings.TrimSpace(*outPath) != "" {
		raw, err := json.MarshalIndent(report{Summary: summary, Results: results}, "", "  ")
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

func printSummary(summary eval.QuerySummary, ks []int) {
	fmt.Println("========================================")
	fmt.Println("  RAG Online Baseline Report")
	fmt.Println("========================================")
	fmt.Printf("  Cases        : %d\n", summary.Cases)
	fmt.Printf("  Avg Init ms  : %.2f\n", summary.AvgInitLatencyMs)
	fmt.Printf("  Avg Rewrite  : %.2f\n", summary.AvgRewriteLatencyMs)
	fmt.Printf("  Avg Retrieve : %.2f\n", summary.AvgRetrieveLatencyMs)
	fmt.Printf("  Avg Rerank   : %.2f\n", summary.AvgRerankLatencyMs)
	fmt.Printf("  Avg Total ms : %.2f\n", summary.AvgTotalLatencyMs)
	fmt.Printf("  Cache Hit    : %.2f\n", summary.CacheHitRate)
	fmt.Printf("  Empty Rate   : %.2f\n", summary.EmptyRate)
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
