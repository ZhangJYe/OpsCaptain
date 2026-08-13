package main

import (
	"SuperBizAgent/internal/ai/rag/eval"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gogf/gf/v2/frame/g"
)

func main() {
	developmentPath := flag.String("development", "evals/rag/retrieval_development.jsonl", "development dataset JSONL path")
	holdoutPath := flag.String("holdout", "evals/rag/retrieval_holdout.jsonl", "sealed holdout dataset JSONL path")
	manifestPath := flag.String("manifest", "evals/rag/corpus_manifest.json", "versioned corpus manifest path")
	outPath := flag.String("out", "evals/rag/reports/dataset-validation.json", "validation report output path")
	flag.Parse()

	development, err := eval.LoadEvalCasesJSONL(*developmentPath)
	if err != nil {
		fail("load development dataset", err)
	}
	holdout, err := eval.LoadEvalCasesJSONL(*holdoutPath)
	if err != nil {
		fail("load holdout dataset", err)
	}
	manifest, err := eval.LoadCorpusManifest(*manifestPath)
	if err != nil {
		fail("load corpus manifest", err)
	}

	report := eval.ValidateDatasetPair(development, holdout, manifest, loadValidationConfig(context.Background()))
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fail("marshal validation report", err)
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fail("create report directory", err)
	}
	if err := os.WriteFile(*outPath, append(raw, '\n'), 0o644); err != nil {
		fail("write validation report", err)
	}

	fmt.Printf("RAG dataset validation: valid=%t corpus=%s\n", report.Valid, report.CorpusVersion)
	fmt.Printf("development: cases=%d adequate=%t coverage=%.2f%% fingerprint=%s\n",
		report.Development.Cases, report.Development.Adequate, report.Development.CoverageRate*100, report.Development.Fingerprint)
	fmt.Printf("holdout: cases=%d adequate=%t coverage=%.2f%% fingerprint=%s\n",
		report.Holdout.Cases, report.Holdout.Adequate, report.Holdout.CoverageRate*100, report.Holdout.Fingerprint)
	fmt.Printf("near_duplicate_threshold=%.2f issues=%d report=%s\n",
		report.Config.NearDuplicateThreshold, len(report.Issues), *outPath)
	if !report.Valid {
		for _, issue := range report.Issues {
			fmt.Fprintf(os.Stderr, "- %s\n", issue)
		}
		os.Exit(1)
	}
}

func loadValidationConfig(ctx context.Context) eval.DatasetValidationConfig {
	cfg := eval.DefaultDatasetValidationConfig()
	if value, err := g.Cfg().Get(ctx, "rag.eval_dataset.development_min_cases"); err == nil && value.Int() > 0 {
		cfg.DevelopmentMinCases = value.Int()
	}
	if value, err := g.Cfg().Get(ctx, "rag.eval_dataset.holdout_min_cases"); err == nil && value.Int() > 0 {
		cfg.HoldoutMinCases = value.Int()
	}
	if value, err := g.Cfg().Get(ctx, "rag.eval_dataset.near_duplicate_threshold"); err == nil && value.Float64() > 0 {
		cfg.NearDuplicateThreshold = value.Float64()
	}
	if value, err := g.Cfg().Get(ctx, "rag.eval_dataset.category_tolerance"); err == nil && value.Float64() >= 0 {
		cfg.CategoryTolerance = value.Float64()
	}
	for category := range cfg.CategoryTargets {
		key := "rag.eval_dataset.category_targets." + category
		if value, err := g.Cfg().Get(ctx, key); err == nil && value.Float64() >= 0 {
			cfg.CategoryTargets[category] = value.Float64()
		}
	}
	return cfg
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s failed: %v\n", action, err)
	os.Exit(1)
}
