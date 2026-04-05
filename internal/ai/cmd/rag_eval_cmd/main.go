package main

import (
	"SuperBizAgent/internal/ai/rag/eval"
	"context"
	"fmt"
	"sort"
)

func main() {
	ctx := context.Background()
	searcher := eval.NewInMemoryRetriever(eval.SampleCorpus())
	summary, results, err := eval.Run(ctx, searcher, eval.SampleCases(), []int{1, 3})
	if err != nil {
		panic(err)
	}

	fmt.Println("RAG Recall Harness")
	fmt.Printf("cases=%d\n", summary.Cases)

	ks := make([]int, 0, len(summary.AvgRecallAtK))
	for k := range summary.AvgRecallAtK {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	for _, k := range ks {
		fmt.Printf(
			"avg_recall@%d=%.2f hit_rate@%d=%.2f full_recall_cases@%d=%d\n",
			k,
			summary.AvgRecallAtK[k],
			k,
			summary.HitRateAtK[k],
			k,
			summary.FullRecallAtK[k],
		)
	}

	fmt.Println("\nCase Breakdown")
	for _, result := range results {
		fmt.Printf(
			"%s recall@1=%.2f recall@3=%.2f ranked=%v relevant=%v\n",
			result.CaseID,
			result.RecallAtK[1],
			result.RecallAtK[3],
			result.RankedIDs,
			result.RelevantIDs,
		)
	}
}
