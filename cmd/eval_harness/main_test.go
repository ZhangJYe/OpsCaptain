package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"SuperBizAgent/internal/ai/evalharness"
)

func TestRunCLIStableExitCodes(t *testing.T) {
	if code := runCLI(nil); code != exitInvalid {
		t.Fatalf("empty args exit=%d", code)
	}
	if code := runCLI([]string{"unknown"}); code != exitInvalid {
		t.Fatalf("unknown command exit=%d", code)
	}
	if code := runCLI([]string{"validate"}); code != exitInvalid {
		t.Fatalf("missing manifest exit=%d", code)
	}
}

func TestRunCLIValidatesAndRunsDeterministicGate(t *testing.T) {
	manifest := filepath.Join("..", "..", "evals", "harness", "manifests", "pr-regression.yaml")
	if code := runCLI([]string{"validate", "--manifest", manifest}); code != exitSuccess {
		t.Fatalf("validate exit=%d", code)
	}
	if code := runCLI([]string{"gate", "--manifest", manifest, "--output-dir", t.TempDir()}); code != exitSuccess {
		t.Fatalf("gate exit=%d", code)
	}
}

func TestRunCLIGateFailureHasStableExitCode(t *testing.T) {
	dir := t.TempDir()
	caseData := `{"schema_version":"evaluation-case/v1","id":"route-wrong","suite":"route","input":{"query":"故障"},"expectation":{"decision":"chat"},"payload_schema_version":"route-eval/v1","payload":{"expected_decision":"chat","high_confidence_keywords":["故障"]}}` + "\n"
	casePath := filepath.Join(dir, "route.jsonl")
	if err := os.WriteFile(casePath, []byte(caseData), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `schema_version: evaluation-harness/v1
run_name: failing-gate
dataset_role: regression
label_source: test fixture
profile: deterministic
dependencies: {all: fixture}
code_scope: test
code_paths: [route.jsonl]
evidence_corpus_paths: [route.jsonl]
model_fingerprint: fixture
prompt_fingerprint: fixture
evaluator_fingerprint: test-v1
budget: {max_cases: 1, concurrency: 1, case_timeout_ms: 1000, total_timeout_ms: 5000}
redaction: {max_text_chars: 128, sensitive_keys: [token]}
suites:
  - name: route
    enabled: true
    dataset: route.jsonl
    payload_schema: route-eval/v1
    gates:
      - {name: exact_route, metric: macro_f1, operator: ">=", threshold: 1, severity: blocking}
`
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runCLI([]string{"gate", "--manifest", manifestPath, "--output-dir", dir}); code != exitGateFailed {
		t.Fatalf("gate failure exit=%d", code)
	}
}

func TestRunCLICompareRejectsIncompatibleReports(t *testing.T) {
	dir := t.TempDir()
	report := evalharness.Report{SchemaVersion: evalharness.ReportSchemaVersion, DatasetRole: evalharness.DatasetRegression, Profile: evalharness.ProfileDeterministic, Fingerprints: evalharness.Fingerprints{Dataset: "dataset-a", CodeScope: "scope", Evaluator: "eval", EvidenceCorpus: "corpus"}}
	baselinePath := writeTestReport(t, dir, "baseline.json", report)
	report.Fingerprints.Dataset = "dataset-b"
	candidatePath := writeTestReport(t, dir, "candidate.json", report)
	if code := runCLI([]string{"compare", "--baseline", baselinePath, "--candidate", candidatePath}); code != exitInvalid {
		t.Fatalf("incompatible compare exit=%d", code)
	}
}

func TestRunCLICompareCompatibleReports(t *testing.T) {
	dir := t.TempDir()
	fingerprints := evalharness.Fingerprints{Dataset: "dataset", CodeScope: "scope", Evaluator: "eval", EvidenceCorpus: "corpus"}
	report := evalharness.Report{SchemaVersion: evalharness.ReportSchemaVersion, DatasetRole: evalharness.DatasetRegression, Profile: evalharness.ProfileDeterministic, Fingerprints: fingerprints, Suites: []evalharness.SuiteReport{{Name: evalharness.SuiteRAG, DomainSchema: "rag-metrics/v1", CommonMetrics: evalharness.CommonMetrics{FailureRate: evalharness.AvailableMetric(0, "ratio")}, DomainMetrics: evalharness.MarshalDomain(map[string]float64{"mrr": 0.75})}}}
	baselinePath := writeTestReport(t, dir, "baseline.json", report)
	report.Suites[0].DomainMetrics = evalharness.MarshalDomain(map[string]float64{"mrr": 0.8})
	candidatePath := writeTestReport(t, dir, "candidate.json", report)
	if code := runCLI([]string{"compare", "--baseline", baselinePath, "--candidate", candidatePath}); code != exitSuccess {
		t.Fatalf("compatible compare exit=%d", code)
	}
}

func writeTestReport(t *testing.T, dir, name string, report evalharness.Report) string {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
