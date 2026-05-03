package main

import (
	"SuperBizAgent/internal/ai/rag/eval"
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	casesPath := flag.String("cases", "", "path to eval cases JSONL file (empty = use built-in samples)")
	corpusPath := flag.String("corpus", "", "path to corpus JSONL file (empty = use built-in samples)")
	ksStr := flag.String("ks", "1,3,5", "comma-separated K values for Recall@K")
	r1Threshold := flag.Float64("r1-threshold", 0.8, "Recall@1 pass threshold")
	r5Threshold := flag.Float64("r5-threshold", 0.95, "Recall@5 pass threshold")
	continueOnError := flag.Bool("continue-on-error", false, "continue evaluation when a case fails")
	flag.Parse()

	ks, err := parseKs(*ksStr)
	if err != nil {
		fatal("invalid -ks: %v", err)
	}

	ctx := context.Background()
	var searcher eval.Searcher
	var corpus []eval.RetrievedDoc
	var cases []eval.EvalCase

	if *corpusPath != "" {
		fatal("-corpus JSONL loading not yet implemented")
	}
	if *casesPath != "" {
		cases, err = eval.LoadEvalCasesJSONL(*casesPath)
		if err != nil {
			fatal("load cases: %v", err)
		}
	} else {
		corpus = eval.SampleCorpus()
		cases = eval.SampleCases()
	}
	searcher = eval.NewInMemoryRetriever(corpus)

	summary, results, err := eval.RunWithOpts(ctx, searcher, cases, ks, eval.RunOptions{
		ContinueOnError: *continueOnError,
	})
	if err != nil {
		fatal("eval failed: %v", err)
	}

	printReport(summary, results, ks, corpus, *r1Threshold, *r5Threshold)
}

func parseKs(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("empty ks")
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid k %q: %w", p, err)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid ks")
	}
	return out, nil
}

func printReport(summary eval.Summary, results []eval.CaseResult, ks []int, corpus []eval.RetrievedDoc, r1Thresh, r5Thresh float64) {
	sortedKs := make([]int, 0, len(summary.AvgRecallAtK))
	for k := range summary.AvgRecallAtK {
		sortedKs = append(sortedKs, k)
	}
	sort.Ints(sortedKs)

	fmt.Println("========================================")
	fmt.Println("  RAG Recall Evaluation Report")
	fmt.Println("========================================")
	fmt.Printf("  Searcher : InMemory (lexical)\n")
	if len(corpus) > 0 {
		fmt.Printf("  Corpus   : %d documents\n", len(corpus))
	}
	fmt.Printf("  Cases    : %d (succeeded: %d, failed: %d)\n", summary.Cases, summary.Succeeded, summary.Failed)
	fmt.Println("========================================")

	fmt.Println("\n--- Summary ---")
	fmt.Printf("%-12s", "Metric")
	for _, k := range sortedKs {
		fmt.Printf("  @%-5d", k)
	}
	fmt.Println()

	fmt.Printf("%-12s", "Avg Recall")
	for _, k := range sortedKs {
		fmt.Printf("  %-6.2f", summary.AvgRecallAtK[k])
	}
	fmt.Println()

	fmt.Printf("%-12s", "Hit Rate")
	for _, k := range sortedKs {
		fmt.Printf("  %-6.2f", summary.HitRateAtK[k])
	}
	fmt.Println()

	fmt.Printf("%-12s", "Full Recall")
	for _, k := range sortedKs {
		fmt.Printf("  %-3d/%d ", summary.FullRecallAtK[k], summary.Succeeded)
	}
	fmt.Println()

	if summary.Failed > 0 {
		fmt.Printf("\n  ⚠ %d case(s) failed:\n", summary.Failed)
		for _, f := range summary.Failures {
			fmt.Printf("    - %s: %s\n", f.CaseID, f.Error)
		}
	}

	fmt.Println("\n--- Case Breakdown ---")
	fmt.Printf("%-10s %-45s", "CaseID", "Query")
	for _, k := range sortedKs {
		fmt.Printf("  R@%-3d", k)
	}
	fmt.Printf("  %s\n", "Status")

	for _, result := range results {
		query := result.Query
		if len(query) > 40 {
			query = query[:40] + "..."
		}
		fmt.Printf("%-10s %-45s", result.CaseID, query)

		allHit := true
		anyHit := false
		for _, k := range sortedKs {
			r := result.RecallAtK[k]
			fmt.Printf("  %-6.2f", r)
			if r < 1.0 {
				allHit = false
			}
			if r > 0 {
				anyHit = true
			}
		}

		status := "PASS"
		if !anyHit {
			status = "MISS"
		} else if !allHit {
			status = "PARTIAL"
		}
		fmt.Printf("  %s\n", status)
	}

	fmt.Println("\n--- Failure Analysis ---")
	var missCount, partialCount, passCount int
	var failedCases []string
	for _, result := range results {
		maxRecall := 0.0
		for _, k := range sortedKs {
			if result.RecallAtK[k] > maxRecall {
				maxRecall = result.RecallAtK[k]
			}
		}
		if maxRecall == 0 {
			missCount++
			failedCases = append(failedCases, result.CaseID+" (MISS)")
		} else if maxRecall < 1.0 {
			partialCount++
			failedCases = append(failedCases, result.CaseID+" (PARTIAL)")
		} else {
			passCount++
		}
	}
	fmt.Printf("  PASS:    %d/%d\n", passCount, summary.Succeeded)
	fmt.Printf("  PARTIAL: %d/%d\n", partialCount, summary.Succeeded)
	fmt.Printf("  MISS:    %d/%d\n", missCount, summary.Succeeded)
	if len(failedCases) > 0 {
		fmt.Printf("  Failed:  %s\n", strings.Join(failedCases, ", "))
	}

	fmt.Println("\n========================================")
	avgR1 := summary.AvgRecallAtK[1]
	avgR5 := summary.AvgRecallAtK[5]
	if avgR1 >= r1Thresh && avgR5 >= r5Thresh {
		fmt.Println("  VERDICT: BASELINE MET")
	} else {
		fmt.Println("  VERDICT: BASELINE NOT MET")
		if avgR1 < r1Thresh {
			fmt.Printf("  -> Recall@1 (%.2f) < %.2f target\n", avgR1, r1Thresh)
		}
		if avgR5 < r5Thresh {
			fmt.Printf("  -> Recall@5 (%.2f) < %.2f target\n", avgR5, r5Thresh)
		}
	}
	fmt.Println("========================================")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
