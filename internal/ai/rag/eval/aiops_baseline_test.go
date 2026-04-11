package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestGenerateAIOPSBaselineArtifacts(t *testing.T) {
	root := t.TempDir()
	datasetRoot := filepath.Join(root, "aiopschallenge2025")
	if err := os.MkdirAll(datasetRoot, 0o755); err != nil {
		t.Fatalf("mkdir dataset root: %v", err)
	}

	inputs := []AIOPSInputCase{
		{UUID: "case-a", AnomalyDescription: "service a anomaly"},
		{UUID: "case-b", AnomalyDescription: "service b anomaly"},
	}
	inputRaw, err := json.Marshal(inputs)
	if err != nil {
		t.Fatalf("marshal inputs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(datasetRoot, "input.json"), inputRaw, 0o644); err != nil {
		t.Fatalf("write input.json: %v", err)
	}

	gtLines := []string{
		`{"uuid":"case-a","fault_category":"stress","fault_type":"cpu stress","instance_type":"service","service":"svc-a","instance":"svc-a","source":"","destination":"","start_time":"2025-06-05T16:10:02Z","end_time":"2025-06-05T16:31:02Z","key_observations":[{"type":"metric","keyword":["rrt","cpu_usage"]}],"key_metrics":["cpu_usage"],"fault_description":["high cpu"]}`,
		`{"uuid":"case-b","fault_category":"network","fault_type":"network delay","instance_type":"service","service":"svc-b","instance":"svc-b","source":"frontend","destination":"svc-b","start_time":"2025-06-06T10:00:00Z","end_time":"2025-06-06T10:10:00Z","key_observations":[{"type":"trace","keyword":["timeout","latency"]}],"key_metrics":["latency"],"fault_description":["network delay"]}`,
	}
	if err := os.WriteFile(filepath.Join(datasetRoot, "groundtruth.jsonl"), []byte(gtLines[0]+"\n"+gtLines[1]+"\n"), 0o644); err != nil {
		t.Fatalf("write groundtruth: %v", err)
	}

	outputRoot := filepath.Join(root, "baseline")
	summary, err := GenerateAIOPSBaselineArtifacts(context.Background(), AIOPSPrepOptions{
		DatasetRoot: datasetRoot,
		OutputRoot:  outputRoot,
		EvalRatio:   0.5,
	})
	if err != nil {
		t.Fatalf("GenerateAIOPSBaselineArtifacts: %v", err)
	}

	if summary.Cases != 2 || summary.EvidenceDocs != 2 || summary.HistoryDocs != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.EvalCases != 4 {
		t.Fatalf("expected 4 eval cases, got %+v", summary)
	}
	if summary.BuildCases != 1 || summary.HoldoutCases != 1 {
		t.Fatalf("unexpected split summary: %+v", summary)
	}

	for _, path := range []string{
		filepath.Join(outputRoot, "docs_evidence", "case-a.md"),
		filepath.Join(outputRoot, "docs_history", "case-b.md"),
		filepath.Join(outputRoot, "eval", "eval_cases.jsonl"),
		filepath.Join(outputRoot, "eval", "eval_cases_holdout.jsonl"),
		filepath.Join(outputRoot, "eval", "build_split.json"),
		filepath.Join(outputRoot, "eval", "eval_split.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated file %s: %v", path, err)
		}
	}

	historyRaw, err := os.ReadFile(filepath.Join(outputRoot, "docs_history", "case-a.md"))
	if err != nil {
		t.Fatalf("read history doc: %v", err)
	}
	if string(historyRaw) == "" || !containsAll(string(historyRaw), "历史案例标签", "cpu stress", "high cpu") {
		t.Fatalf("unexpected history doc content: %s", string(historyRaw))
	}

	evalCases, err := LoadEvalCasesJSONL(filepath.Join(outputRoot, "eval", "eval_cases_holdout.jsonl"))
	if err != nil {
		t.Fatalf("load holdout eval cases: %v", err)
	}
	if len(evalCases) != 2 {
		t.Fatalf("expected 2 holdout eval cases, got %d", len(evalCases))
	}
}

func TestCanonicalSchemaDocIDPrefersSourceBasename(t *testing.T) {
	doc := &schema.Document{
		ID: "chunk-01",
		MetaData: map[string]any{
			"_source": `D:\Agent\OpsCaption\aiopschallenge2025\baseline\docs_evidence\case-123.md`,
		},
	}
	if got := CanonicalSchemaDocID(doc); got != "case-123" {
		t.Fatalf("expected case-123, got %q", got)
	}
}

func TestRunQueryEvalDeduplicatesChunkHits(t *testing.T) {
	cases := []EvalCase{
		{
			ID:          "case-123-obs",
			Query:       "svc-a rrt timeout",
			RelevantIDs: []string{"case-123"},
		},
	}
	exec := func(context.Context, string) ([]RetrievedDoc, QueryMetrics, error) {
		return []RetrievedDoc{
			{ID: "case-123"},
			{ID: "case-123"},
			{ID: "case-999"},
		}, QueryMetrics{TotalLatencyMs: 12, ResultCount: 3}, nil
	}

	summary, results, err := RunQueryEval(context.Background(), exec, cases, []int{1, 3})
	if err != nil {
		t.Fatalf("RunQueryEval: %v", err)
	}
	if got := summary.AvgRecallAtK[1]; got != 1 {
		t.Fatalf("expected recall@1 1, got %v", got)
	}
	if len(results) != 1 || len(results[0].RankedIDs) != 2 {
		t.Fatalf("expected deduplicated ranked ids, got %+v", results)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
