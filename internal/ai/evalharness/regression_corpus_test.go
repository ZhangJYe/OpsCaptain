package evalharness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRegressionCorpus(t *testing.T) {
	dir := t.TempDir()
	casePath := filepath.Join(dir, "route.jsonl")
	evalCase := CaseEnvelope{
		SchemaVersion: CaseSchemaVersion, ID: "route-1", Suite: SuiteRoute,
		Input: CaseInput{Query: "q"}, Expectation: json.RawMessage(`{"decision":"chat"}`),
		Tags: []string{"chat"}, PayloadSchemaVersion: "route-eval/v1", Payload: json.RawMessage(`{"expected_decision":"chat","model_output":"fixture"}`),
	}
	encoded, err := json.Marshal(evalCase)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(casePath, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := &Manifest{
		SchemaVersion: ManifestSchemaVersion, RunName: "corpus", DatasetRole: DatasetRegression, Profile: ProfileDeterministic,
		SourcePath: filepath.Join(dir, "manifest.yaml"), RegressionCorpus: RegressionCorpusConfig{
			MinTotal: 1, SuiteMinimums: map[SuiteName]int{SuiteRoute: 1}, RequiredTags: map[SuiteName][]string{SuiteRoute: {"chat"}},
		},
		Suites: []SuiteConfig{{Name: SuiteRoute, Enabled: true, Dataset: "route.jsonl", PayloadSchema: "route-eval/v1"}},
	}
	if err := ValidateRegressionCorpus(manifest); err != nil {
		t.Fatalf("expected valid corpus: %v", err)
	}
	manifest.RegressionCorpus.MinTotal = 2
	if err := ValidateRegressionCorpus(manifest); err == nil || !strings.Contains(err.Error(), "at least 2") {
		t.Fatalf("expected size validation error, got %v", err)
	}
	manifest.RegressionCorpus.MinTotal = 1
	manifest.RegressionCorpus.RequiredTags[SuiteRoute] = []string{"incident"}
	if err := ValidateRegressionCorpus(manifest); err == nil || !strings.Contains(err.Error(), "missing required regression tag") {
		t.Fatalf("expected tag validation error, got %v", err)
	}
}

func TestValidateRegressionCorpusRejectsWrongProfile(t *testing.T) {
	manifest := &Manifest{DatasetRole: DatasetDevelopment, Profile: ProfileDeterministic, RegressionCorpus: RegressionCorpusConfig{MinTotal: 1}}
	if err := ValidateRegressionCorpus(manifest); err == nil || !strings.Contains(err.Error(), "regression + deterministic") {
		t.Fatalf("expected role/profile validation error, got %v", err)
	}
}
