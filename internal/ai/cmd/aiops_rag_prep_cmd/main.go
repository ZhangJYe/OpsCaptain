package main

import (
	"SuperBizAgent/internal/ai/rag/eval"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	datasetRoot := flag.String("dataset-root", filepath.Join(".", "aiopschallenge2025"), "path to aiopschallenge2025 dataset root")
	outputRoot := flag.String("output-root", "", "path to write baseline artifacts; defaults to <dataset-root>/baseline")
	evalRatio := flag.Float64("eval-ratio", eval.DefaultAIOPSEvalRatio, "holdout ratio for generated split manifest")
	flag.Parse()

	summary, err := eval.GenerateAIOPSBaselineArtifacts(context.Background(), eval.AIOPSPrepOptions{
		DatasetRoot: *datasetRoot,
		OutputRoot:  *outputRoot,
		EvalRatio:   *evalRatio,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare baseline artifacts failed: %v\n", err)
		os.Exit(1)
	}

	raw, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal summary failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(raw))
}
