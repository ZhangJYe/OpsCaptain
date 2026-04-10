package main

import (
	"SuperBizAgent/internal/ai/rag/eval"
	"context"
	"fmt"
	"sort"
	"strings"
)

func main() {
	ctx := context.Background()
	searcher := eval.NewInMemoryRetriever(eval.SampleCorpus())
	ks := []int{1, 3, 5}
	summary, results, err := eval.Run(ctx, searcher, eval.SampleCases(), ks)
	if err != nil {
		panic(err)
	}

	fmt.Println("========================================")
	fmt.Println("  RAG Recall Evaluation Report")
	fmt.Println("========================================")
	fmt.Printf("  Searcher : InMemory (lexical)\n")
	fmt.Printf("  Corpus   : %d documents\n", len(eval.SampleCorpus()))
	fmt.Printf("  Cases    : %d\n", summary.Cases)
	fmt.Println("========================================")

	fmt.Println("\n--- Summary ---")
	sortedKs := make([]int, 0, len(summary.AvgRecallAtK))
	for k := range summary.AvgRecallAtK {
		sortedKs = append(sortedKs, k)
	}
	sort.Ints(sortedKs)

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
		fmt.Printf("  %-3d/%d ", summary.FullRecallAtK[k], summary.Cases)
	}
	fmt.Println()

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
	fmt.Printf("  PASS:    %d/%d\n", passCount, summary.Cases)
	fmt.Printf("  PARTIAL: %d/%d\n", partialCount, summary.Cases)
	fmt.Printf("  MISS:    %d/%d\n", missCount, summary.Cases)
	if len(failedCases) > 0 {
		fmt.Printf("  Failed:  %s\n", strings.Join(failedCases, ", "))
	}

	fmt.Println("\n========================================")
	avgR1 := summary.AvgRecallAtK[1]
	avgR5 := summary.AvgRecallAtK[5]
	if avgR1 >= 0.8 && avgR5 >= 0.95 {
		fmt.Println("  VERDICT: BASELINE MET")
	} else {
		fmt.Println("  VERDICT: BASELINE NOT MET")
		if avgR1 < 0.8 {
			fmt.Printf("  -> Recall@1 (%.2f) < 0.80 target\n", avgR1)
		}
		if avgR5 < 0.95 {
			fmt.Printf("  -> Recall@5 (%.2f) < 0.95 target\n", avgR5)
		}
	}
	fmt.Println("========================================")
}
