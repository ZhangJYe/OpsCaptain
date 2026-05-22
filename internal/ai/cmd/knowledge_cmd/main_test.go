package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnowledgeIndexerDryRunScansMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runbook.md"), []byte("# Runbook\n"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("write text: %v", err)
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-dir", dir, "-collection", "opscaption_knowledge_v2", "-dry-run"}, nil, &out)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "dry_run=true collection=opscaption_knowledge_v2 files=1") {
		t.Fatalf("unexpected dry-run output: %s", got)
	}
	if !strings.Contains(got, "runbook.md") {
		t.Fatalf("expected markdown path in output: %s", got)
	}
	if strings.Contains(got, "notes.txt") {
		t.Fatalf("expected non-markdown file to be skipped: %s", got)
	}
}
