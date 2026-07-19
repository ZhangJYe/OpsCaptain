package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	goseval "SuperBizAgent/internal/ai/agent/gos_engine/eval"
	"SuperBizAgent/internal/ai/protocol"
)

type batchFakeEngine struct {
	calls *int
}

func (e *batchFakeEngine) Run(_ context.Context, _ string) *protocol.TaskResult {
	*e.calls++
	return &protocol.TaskResult{
		Status:  protocol.ResultStatusSucceeded,
		Summary: "database pool exhausted",
		Evidence: []protocol.EvidenceItem{{
			SourceType: "logs",
			SourceID:   "recorded-case",
			Snippet:    "database pool exhausted",
		}},
		Metadata: map[string]any{
			"graph_valid": true,
			"llm_calls":   1,
			"tool_calls":  1,
		},
	}
}

func TestExecuteGoSBatchResumesCompletedCases(t *testing.T) {
	dataset := batchTestDataset()
	outputDir := t.TempDir()
	identity := batchTestIdentity()

	firstCalls := 0
	firstRunner := goseval.NewRunner(&batchFakeEngine{calls: &firstCalls})
	artifact, err := executeGoSBatch(
		context.Background(),
		dataset,
		firstRunner,
		identity,
		batchOptions{
			OutputDir:   outputDir,
			CaseTimeout: time.Second,
			Resume:      true,
			MaxNewCases: 1,
		},
	)
	if err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if artifact.Status != "partial" || artifact.CompletedCases != 1 || firstCalls != 1 {
		t.Fatalf("unexpected first artifact: status=%s completed=%d calls=%d", artifact.Status, artifact.CompletedCases, firstCalls)
	}

	secondCalls := 0
	secondRunner := goseval.NewRunner(&batchFakeEngine{calls: &secondCalls})
	artifact, err = executeGoSBatch(
		context.Background(),
		dataset,
		secondRunner,
		identity,
		batchOptions{
			OutputDir:   outputDir,
			CaseTimeout: time.Second,
			Resume:      true,
		},
	)
	if err != nil {
		t.Fatalf("resume batch: %v", err)
	}
	if artifact.Status != "completed" || artifact.CompletedCases != 3 || secondCalls != 2 {
		t.Fatalf("unexpected resumed artifact: status=%s completed=%d calls=%d", artifact.Status, artifact.CompletedCases, secondCalls)
	}
	if artifact.Metrics.TotalCases != 3 || artifact.Metrics.Matched != 3 || artifact.Metrics.AvgLLMCalls != 1 {
		t.Fatalf("unexpected resumed metrics: %+v", artifact.Metrics)
	}
}

func TestExecuteGoSBatchRejectsIdentityDrift(t *testing.T) {
	dataset := batchTestDataset()
	outputDir := t.TempDir()
	identity := batchTestIdentity()
	calls := 0
	runner := goseval.NewRunner(&batchFakeEngine{calls: &calls})

	_, err := executeGoSBatch(
		context.Background(),
		dataset,
		runner,
		identity,
		batchOptions{OutputDir: outputDir, CaseTimeout: time.Second, Resume: true, MaxNewCases: 1},
	)
	if err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	identity.CodeSHA256 = "changed"
	_, err = executeGoSBatch(
		context.Background(),
		dataset,
		runner,
		identity,
		batchOptions{OutputDir: outputDir, CaseTimeout: time.Second, Resume: true},
	)
	if err == nil || !strings.Contains(err.Error(), "identity does not match") {
		t.Fatalf("expected identity drift error, got %v", err)
	}
}

func TestExecuteGoSBatchCleansCheckpointsAfterCompletion(t *testing.T) {
	dataset := batchTestDataset()
	outputDir := t.TempDir()
	calls := 0
	runner := goseval.NewRunner(&batchFakeEngine{calls: &calls})

	artifact, err := executeGoSBatch(
		context.Background(),
		dataset,
		runner,
		batchTestIdentity(),
		batchOptions{
			OutputDir:          outputDir,
			CaseTimeout:        time.Second,
			Resume:             true,
			CleanupCheckpoints: true,
		},
	)
	if err != nil {
		t.Fatalf("complete batch: %v", err)
	}
	if artifact.Status != "completed" {
		t.Fatalf("expected completed artifact, got %s", artifact.Status)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "cases")); !os.IsNotExist(err) {
		t.Fatalf("expected checkpoint directory cleanup, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "run.json")); err != nil {
		t.Fatalf("expected aggregate run artifact: %v", err)
	}
}

func TestRescoreGoSBatchUsesCurrentStructuredLabels(t *testing.T) {
	dataset := batchTestDataset()
	for index := range dataset.Cases {
		dataset.Cases[index].Symptom += " case " + dataset.Cases[index].ID
	}
	dataset.Cases[0].ExpectedCauseKeywords = []string{"database pool exhausted"}
	dataset.Cases[0].ExpectedEntityKeywords = []string{"orderservice"}
	datasetPath := filepath.Join(t.TempDir(), "holdout.json")
	if err := writeJSONAtomic(datasetPath, dataset); err != nil {
		t.Fatalf("write dataset: %v", err)
	}
	inputPath := filepath.Join(t.TempDir(), "run.json")
	artifact := buildBatchArtifact(batchTestIdentity(), time.Now().Format(time.RFC3339), dataset.Cases, map[int]goseval.EvalResult{
		0: {
			CaseID:              dataset.Cases[0].ID,
			GroundTruth:         dataset.Cases[0].GroundTruth,
			Prediction:          "orderservice is slow because of network latency",
			Matched:             true,
			Status:              "succeeded",
			StatusMatched:       true,
			TraceComplete:       true,
			GraphValid:          true,
			FailurePhaseMatched: true,
			ContractMatched:     true,
		},
	})
	if err := writeJSONAtomic(inputPath, artifact); err != nil {
		t.Fatalf("write input artifact: %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "rescored.json")
	rescored, err := rescoreGoSBatch(datasetPath, inputPath, outputPath)
	if err != nil {
		t.Fatalf("rescore batch: %v", err)
	}
	if rescored.Metrics.Matched != 0 || rescored.Results[0].Matched {
		t.Fatalf("expected incorrect cause to be rejected: %+v", rescored.Results[0])
	}
	if rescored.Results[0].FailurePhase != "report" {
		t.Fatalf("expected report failure after rescore, got %q", rescored.Results[0].FailurePhase)
	}
	if rescored.EvaluationDatasetSHA256 == "" || rescored.EvaluationCodeSHA256 == "" || rescored.RescoredAt == "" {
		t.Fatalf("expected rescore provenance: %+v", rescored)
	}
}

func batchTestDataset() *goseval.EvalDataset {
	cases := make([]goseval.EvalCase, 3)
	for index := range cases {
		cases[index] = goseval.EvalCase{
			ID:                       "case-" + string(rune('1'+index)),
			Domain:                   "database",
			Scenario:                 "support_evidence",
			Symptom:                  "database requests are waiting",
			GroundTruth:              "database pool exhausted",
			ExpectedKeywords:         []string{"database", "pool", "exhausted"},
			ExpectedEvidenceKeywords: []string{"database", "pool"},
			ExpectedStatus:           "succeeded",
		}
	}
	return &goseval.EvalDataset{
		SchemaVersion: goseval.DatasetSchemaVersion,
		Role:          goseval.DatasetRoleHoldout,
		Cases:         cases,
	}
}

func batchTestIdentity() batchIdentity {
	return batchIdentity{
		SchemaVersion:        batchSchemaVersion,
		Profile:              "recorded",
		DatasetPath:          "holdout.json",
		DatasetRole:          goseval.DatasetRoleHoldout,
		DatasetSHA256:        "dataset",
		ConfigPath:           evalConfigPath,
		ConfigSHA256:         "config",
		CodeSHA256:           "code",
		CodeFingerprintScope: runtimeCodeFingerprintScope,
		EvidenceCorpusSHA256: "evidence",
		EvidenceProvenance:   "recorded_blind",
		ModelProvider:        "test",
		ModelName:            "test",
		ModelEndpointSHA256:  "endpoint",
		CaseTimeoutMS:        1000,
		RecordedTimeoutMS:    2000,
	}
}
