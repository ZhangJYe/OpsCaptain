package main

import (
	"SuperBizAgent/internal/ai/rag"
	"os"
	"path/filepath"
	"testing"
)

func TestParseEvalModeUsesProductionHybridForRetrievalAblations(t *testing.T) {
	tests := []struct {
		mode                    string
		wantRewrite, wantRerank bool
	}{
		{mode: "hybrid-retrieve"},
		{mode: "hybrid-rewrite", wantRewrite: true},
		{mode: "hybrid-rerank", wantRerank: true},
		{mode: "hybrid-full", wantRewrite: true, wantRerank: true},
		{mode: "retrieve"},
		{mode: "rewrite", wantRewrite: true},
		{mode: "rerank", wantRerank: true},
		{mode: "full", wantRewrite: true, wantRerank: true},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			gotRewrite, gotRerank, isHybrid, useConfigDefaults, err := parseEvalMode(tt.mode)
			if err != nil {
				t.Fatalf("parseEvalMode returned error: %v", err)
			}
			if !isHybrid {
				t.Fatal("retrieval ablation must use the production hybrid query path")
			}
			if useConfigDefaults {
				t.Fatal("retrieval ablation must override the corresponding feature flags")
			}
			if gotRewrite != tt.wantRewrite || gotRerank != tt.wantRerank {
				t.Fatalf("flags = rewrite:%t rerank:%t, want rewrite:%t rerank:%t", gotRewrite, gotRerank, tt.wantRewrite, tt.wantRerank)
			}
		})
	}
}

func TestParseEvalModeRejectsUnknownMode(t *testing.T) {
	_, _, _, _, err := parseEvalMode("typo-mode")
	if err == nil {
		t.Fatal("expected unknown mode error")
	}
}

func TestApplyRankingOverrides(t *testing.T) {
	cfg := rag.HybridConfig{DenseWeight: 1, LexicalWeight: 1}
	if err := applyRankingOverrides(&cfg, 1, 1.25, true, true, true); err != nil {
		t.Fatalf("applyRankingOverrides returned error: %v", err)
	}
	if cfg.DenseWeight != 1 || cfg.LexicalWeight != 1.25 || !cfg.KnowledgeFieldBoostEnabled || !cfg.CoverageEnabled {
		t.Fatalf("unexpected ranking override: %+v", cfg)
	}
	if err := applyRankingOverrides(&cfg, 0, 0, false, false, false); err == nil {
		t.Fatal("zero/zero weights must be rejected")
	}
}

func TestValidateRunInputs(t *testing.T) {
	tests := []struct {
		name          string
		evalPath      string
		role          string
		corpusVersion string
		outPath       string
		wantErr       bool
	}{
		{name: "builtin development", role: "development", outPath: "report.json"},
		{name: "external complete", evalPath: "cases.jsonl", role: "holdout", corpusVersion: "kb-v1", outPath: "report.json"},
		{name: "missing role", outPath: "report.json", wantErr: true},
		{name: "bad role", role: "test", outPath: "report.json", wantErr: true},
		{name: "missing output", role: "development", wantErr: true},
		{name: "external missing corpus", evalPath: "cases.jsonl", role: "development", outPath: "report.json", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRunInputs(tt.evalPath, tt.role, tt.corpusVersion, tt.outPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateRunInputs error = %v, wantErr=%t", err, tt.wantErr)
			}
		})
	}
}

func TestMarkdownFilesIncludesNestedDocuments(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "upstream", "prometheus")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(root, "runbook.md"):    "root",
		filepath.Join(nested, "alerting.md"): "nested",
		filepath.Join(nested, "ignore.txt"):  "not markdown",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := markdownFiles(root)
	if len(got) != 2 {
		t.Fatalf("markdownFiles returned %d files, want 2: %v", len(got), got)
	}
	if got[0] != filepath.Join(root, "runbook.md") || got[1] != filepath.Join(nested, "alerting.md") {
		t.Fatalf("markdownFiles = %v", got)
	}
}
