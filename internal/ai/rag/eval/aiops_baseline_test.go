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
		{UUID: "case-a", AnomalyDescription: "service a timeout anomaly"},
		{UUID: "case-b", AnomalyDescription: "service b cpu anomaly"},
		{UUID: "case-c", AnomalyDescription: "service a timeout with retries"},
	}
	inputRaw, err := json.Marshal(inputs)
	if err != nil {
		t.Fatalf("marshal inputs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(datasetRoot, "input.json"), inputRaw, 0o644); err != nil {
		t.Fatalf("write input.json: %v", err)
	}

	gtLines := []string{
		`{"uuid":"case-a","fault_category":"network","fault_type":"rpc timeout","instance_type":"service","service":"svc-a","instance":"svc-a","source":"frontend","destination":"svc-a","start_time":"2025-06-05T16:10:02Z","end_time":"2025-06-05T16:31:02Z","key_observations":[{"type":"trace","keyword":["timeout","latency"]}],"key_metrics":["latency"],"fault_description":["rpc timeout"]}`,
		`{"uuid":"case-b","fault_category":"stress","fault_type":"cpu stress","instance_type":"service","service":"svc-b","instance":"svc-b","source":"worker","destination":"svc-b","start_time":"2025-06-06T10:00:00Z","end_time":"2025-06-06T10:10:00Z","key_observations":[{"type":"metric","keyword":["cpu_usage","load"]}],"key_metrics":["cpu_usage"],"fault_description":["high cpu"]}`,
		`{"uuid":"case-c","fault_category":"stress","fault_type":"cpu stress","instance_type":"service","service":"svc-a","instance":"svc-a","source":"frontend","destination":"svc-a","start_time":"2025-06-07T12:00:00Z","end_time":"2025-06-07T12:10:00Z","key_observations":[{"type":"trace","keyword":["timeout","retries"]}],"key_metrics":["retries"],"fault_description":["timeout with retries"]}`,
	}
	if err := os.WriteFile(filepath.Join(datasetRoot, "groundtruth.jsonl"), []byte(strings.Join(gtLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write groundtruth: %v", err)
	}

	outputRoot := filepath.Join(root, "baseline")
	summary, err := GenerateAIOPSBaselineArtifacts(context.Background(), AIOPSPrepOptions{
		DatasetRoot: datasetRoot,
		OutputRoot:  outputRoot,
		EvalRatio:   0.34,
	})
	if err != nil {
		t.Fatalf("GenerateAIOPSBaselineArtifacts: %v", err)
	}

	if summary.Cases != 3 || summary.EvidenceDocs != 3 || summary.HistoryDocs != 3 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.BuildEvidenceDocs != 2 || summary.BuildHistoryDocs != 2 {
		t.Fatalf("unexpected build-only summary: %+v", summary)
	}
	if summary.EvalCases != 6 || summary.HoldoutEvalCases != 2 {
		t.Fatalf("unexpected eval summary: %+v", summary)
	}
	if summary.HoldoutRelatedEvalCases != 2 || summary.HoldoutSymptomEvalCases != 2 || summary.HoldoutCombinedEvalCases != 2 {
		t.Fatalf("unexpected related summary: %+v", summary)
	}
	if summary.BuildCases != 2 || summary.HoldoutCases != 1 {
		t.Fatalf("unexpected split summary: %+v", summary)
	}

	for _, path := range []string{
		filepath.Join(outputRoot, "docs_evidence", "case-a.md"),
		filepath.Join(outputRoot, "docs_history", "case-c.md"),
		filepath.Join(outputRoot, "docs_evidence_build", "case-a.md"),
		filepath.Join(outputRoot, "docs_history_build", "case-b.md"),
		filepath.Join(outputRoot, "eval", "eval_cases.jsonl"),
		filepath.Join(outputRoot, "eval", "eval_cases_holdout.jsonl"),
		filepath.Join(outputRoot, "eval", "eval_cases_holdout_related.jsonl"),
		filepath.Join(outputRoot, "eval", "eval_cases_holdout_symptom.jsonl"),
		filepath.Join(outputRoot, "eval", "eval_cases_holdout_combined.jsonl"),
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
	if string(historyRaw) == "" || !containsAll(string(historyRaw), "fault_type: rpc timeout", "rpc timeout") {
		t.Fatalf("unexpected history doc content: %s", string(historyRaw))
	}

	evalCases, err := LoadEvalCasesJSONL(filepath.Join(outputRoot, "eval", "eval_cases_holdout.jsonl"))
	if err != nil {
		t.Fatalf("load holdout eval cases: %v", err)
	}
	if len(evalCases) != 2 {
		t.Fatalf("expected 2 holdout eval cases, got %d", len(evalCases))
	}

	faultCases, err := LoadEvalCasesJSONL(filepath.Join(outputRoot, "eval", "eval_cases_holdout_related.jsonl"))
	if err != nil {
		t.Fatalf("load related holdout eval cases: %v", err)
	}
	assertAllRelevantIDs(t, faultCases, []string{"case-b"})

	symptomCases, err := LoadEvalCasesJSONL(filepath.Join(outputRoot, "eval", "eval_cases_holdout_symptom.jsonl"))
	if err != nil {
		t.Fatalf("load symptom holdout eval cases: %v", err)
	}
	assertAllRelevantIDs(t, symptomCases, []string{"case-a"})

	combinedCases, err := LoadEvalCasesJSONL(filepath.Join(outputRoot, "eval", "eval_cases_holdout_combined.jsonl"))
	if err != nil {
		t.Fatalf("load combined holdout eval cases: %v", err)
	}
	assertAllRelevantIDs(t, combinedCases, []string{"case-b", "case-a"})

	if _, err := os.Stat(filepath.Join(outputRoot, "docs_evidence_build", "case-c.md")); !os.IsNotExist(err) {
		t.Fatalf("expected holdout case to be absent from build evidence dir, got err=%v", err)
	}
}

func TestRelatedBuildCaseIDsBySymptomPrefersServiceAndKeywordOverlap(t *testing.T) {
	buildIDs := []string{"case-a", "case-b"}
	groundtruth := map[string]AIOPSGroundTruth{
		"case-a": {
			UUID:         "case-a",
			Service:      "svc-a",
			InstanceType: "service",
			Source:       "frontend",
			Destination:  "svc-a",
			KeyObservations: []AIOPSKeyObservation{
				{Type: "trace", Keyword: []string{"timeout", "latency"}},
			},
		},
		"case-b": {
			UUID:         "case-b",
			Service:      "svc-b",
			InstanceType: "service",
			Source:       "worker",
			Destination:  "svc-b",
			KeyObservations: []AIOPSKeyObservation{
				{Type: "metric", Keyword: []string{"cpu_usage"}},
			},
		},
		"case-holdout": {
			UUID:         "case-holdout",
			Service:      "svc-a",
			InstanceType: "service",
			Source:       "frontend",
			Destination:  "svc-a",
			KeyObservations: []AIOPSKeyObservation{
				{Type: "trace", Keyword: []string{"timeout", "retries"}},
			},
		},
	}

	item := EvalCase{ID: "holdout-obs", RelevantIDs: []string{"case-holdout"}}
	got := relatedBuildCaseIDsBySymptom(item, buildIDs, groundtruth)
	if len(got) != 1 || got[0] != "case-a" {
		t.Fatalf("expected symptom related case-a only, got %+v", got)
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

func assertAllRelevantIDs(t *testing.T, cases []EvalCase, want []string) {
	t.Helper()
	if len(cases) == 0 {
		t.Fatal("expected non-empty eval cases")
	}
	for _, item := range cases {
		if strings.Join(item.RelevantIDs, ",") != strings.Join(want, ",") {
			t.Fatalf("expected relevant ids %v, got %+v", want, item)
		}
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
