package evalharness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubAdapter struct {
	name   SuiteName
	schema string
	run    func(context.Context, CaseEnvelope) CaseResult
}

func (a stubAdapter) Name() SuiteName                                        { return a.name }
func (a stubAdapter) PayloadSchema() string                                  { return a.schema }
func (a stubAdapter) Validate(SuiteConfig, DatasetRole, Profile) error       { return nil }
func (a stubAdapter) RunCase(ctx context.Context, c CaseEnvelope) CaseResult { return a.run(ctx, c) }
func (a stubAdapter) Aggregate(results []CaseResult) (string, json.RawMessage, []GateResult, error) {
	matched := 0
	for _, result := range results {
		if result.Matched {
			matched++
		}
	}
	return a.schema, MarshalDomain(map[string]any{"accuracy": float64(matched) / float64(len(results))}), nil, nil
}

func TestManifestValidationAndCaseFingerprint(t *testing.T) {
	dir := t.TempDir()
	cases := []CaseEnvelope{{
		SchemaVersion: CaseSchemaVersion, ID: "case-1", Suite: SuiteRoute,
		Input: CaseInput{Query: "payment latency"}, Expectation: json.RawMessage(`{"decision":"incident"}`),
		PayloadSchemaVersion: "route/v1", Payload: json.RawMessage(`{"decision":"incident"}`),
	}}
	data, _ := json.Marshal(cases)
	datasetPath := filepath.Join(dir, "route.json")
	if err := os.WriteFile(datasetPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	hash, _ := FileSHA256(datasetPath)
	manifest := &Manifest{
		SchemaVersion: ManifestSchemaVersion, RunName: "test", DatasetRole: DatasetRegression,
		LabelSource: "test fixture",
		Profile:     ProfileDeterministic, SourcePath: filepath.Join(dir, "manifest.yaml"),
		CodeScope: "test", CodePaths: []string{"route.json"}, ModelFingerprint: "fixture", PromptFingerprint: "fixture", EvaluatorFingerprint: "test-v1",
		Dependencies: map[string]string{"all": "fixture"},
		Suites:       []SuiteConfig{{Name: SuiteRoute, Enabled: true, Dataset: "route.json", DatasetSHA256: hash, PayloadSchema: "route/v1"}},
	}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("validate manifest: %v", err)
	}
	if _, gotHash, err := LoadCases(manifest.SourcePath, manifest.Suites[0]); err != nil || gotHash != hash {
		t.Fatalf("load cases: hash=%s err=%v", gotHash, err)
	}
	manifest.Profile = ProfileRecorded
	if err := ValidateManifest(manifest); err == nil {
		t.Fatal("expected invalid regression/recorded combination")
	}
	manifest.Profile = ProfileDeterministic
	manifest.DatasetRole = DatasetHoldout
	if err := ValidateManifest(manifest); err == nil {
		t.Fatal("expected invalid holdout/deterministic combination")
	}
	manifest.DatasetRole = DatasetRegression
	manifest.Suites[0].DatasetSHA256 = strings.Repeat("0", 64)
	if _, _, err := LoadCases(manifest.SourcePath, manifest.Suites[0]); err == nil {
		t.Fatal("expected fingerprint mismatch")
	}
}

func TestCrossSuiteGatesExposeViolatingCases(t *testing.T) {
	suites := []SuiteReport{{Name: SuitePlan, Cases: []CaseResult{{CaseID: "diagnostic", TraceComplete: false, EvidenceCount: 0, Domain: MarshalDomain(map[string]any{"diagnostic": true, "requires_evidence": true})}}}, {Name: SuiteTool, Cases: []CaseResult{{CaseID: "denied", Domain: MarshalDomain(map[string]any{"permission_denied": true, "executed": true})}}}}
	gates := EvaluateCrossSuiteGates(suites)
	for _, gate := range gates {
		if gate.Passed || len(gate.CaseRefs) == 0 {
			t.Fatalf("expected failed gate with case refs: %#v", gate)
		}
	}
}

func TestHarnessPreservesCompletedSuitesAndBudget(t *testing.T) {
	dir := t.TempDir()
	writeCases := func(name string, suite SuiteName, schema string) string {
		path := filepath.Join(dir, name)
		data, _ := json.Marshal([]CaseEnvelope{{
			SchemaVersion: CaseSchemaVersion, ID: string(suite) + "-1", Suite: suite, Input: CaseInput{Query: "q"},
			Expectation: json.RawMessage(`{"ok":true}`), PayloadSchemaVersion: schema, Payload: json.RawMessage(`{"ok":true}`),
		}})
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return name
	}
	registry := NewRegistry()
	_ = registry.Register(stubAdapter{name: SuiteRoute, schema: "route/v1", run: func(context.Context, CaseEnvelope) CaseResult {
		return CaseResult{CaseID: "route-1", Status: StatusSucceeded, Matched: true, TraceComplete: true, Latency: time.Millisecond, Usage: Usage{LLMCalls: 1}}
	}})
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("test manifest"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := &Manifest{
		SchemaVersion: ManifestSchemaVersion, RunName: "multi", DatasetRole: DatasetRegression,
		LabelSource: "test fixture",
		Profile:     ProfileDeterministic, ContinueOnError: true, SourcePath: manifestPath,
		CodeScope: "test", CodePaths: []string{"manifest.yaml"}, ModelFingerprint: "fixture", PromptFingerprint: "fixture", EvaluatorFingerprint: "test-v1",
		Dependencies: map[string]string{"all": "fixture"},
		Budget:       BudgetLimits{MaxCases: 2, MaxLLMCalls: 1},
		Suites: []SuiteConfig{
			{Name: SuiteRoute, Enabled: true, Dataset: writeCases("route.json", SuiteRoute, "route/v1"), PayloadSchema: "route/v1", Budget: BudgetLimits{MaxCases: 1}},
			{Name: SuiteRAG, Enabled: true, Dataset: writeCases("rag.json", SuiteRAG, "rag/v1"), PayloadSchema: "rag/v1"},
		},
	}
	report, err := NewHarness(registry).Run(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Suites) != 2 || report.Suites[0].Status != StatusSucceeded || report.Suites[1].Status != StatusFailed {
		t.Fatalf("unexpected suite reports: %#v", report.Suites)
	}
}

func TestHarnessCancelsPendingCasesAfterHardBudget(t *testing.T) {
	dir := t.TempDir()
	data, _ := json.Marshal([]CaseEnvelope{
		{SchemaVersion: CaseSchemaVersion, ID: "one", Suite: SuiteRoute, Input: CaseInput{Query: "q1"}, Expectation: json.RawMessage(`{"ok":true}`), PayloadSchemaVersion: "route/v1", Payload: json.RawMessage(`{"ok":true}`)},
		{SchemaVersion: CaseSchemaVersion, ID: "two", Suite: SuiteRoute, Input: CaseInput{Query: "q2"}, Expectation: json.RawMessage(`{"ok":true}`), PayloadSchemaVersion: "route/v1", Payload: json.RawMessage(`{"ok":true}`)},
	})
	if err := os.WriteFile(filepath.Join(dir, "route.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	runs := 0
	registry := NewRegistry()
	if err := registry.Register(stubAdapter{name: SuiteRoute, schema: "route/v1", run: func(_ context.Context, evalCase CaseEnvelope) CaseResult {
		runs++
		return CaseResult{CaseID: evalCase.ID, Status: StatusSucceeded, Matched: true, TraceComplete: true, Usage: Usage{LLMCalls: 2}}
	}}); err != nil {
		t.Fatal(err)
	}
	manifest := &Manifest{SchemaVersion: ManifestSchemaVersion, RunName: "budget", DatasetRole: DatasetRegression, LabelSource: "test", Profile: ProfileDeterministic, SourcePath: manifestPath, CodeScope: "test", CodePaths: []string{"manifest.yaml"}, ModelFingerprint: "fixture", PromptFingerprint: "fixture", EvaluatorFingerprint: "test-v1", Dependencies: map[string]string{"all": "fixture"}, Budget: BudgetLimits{MaxCases: 2, MaxLLMCalls: 1}, Suites: []SuiteConfig{{Name: SuiteRoute, Enabled: true, Dataset: "route.json", PayloadSchema: "route/v1", Budget: BudgetLimits{MaxCases: 2, Concurrency: 1, MaxLLMCalls: 1}}}}
	report, err := NewHarness(registry).Run(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != StatusBudgetExceeded || runs != 1 {
		t.Fatalf("expected hard budget cancellation, status=%s runs=%d", report.Status, runs)
	}
}

func TestMetricsUnavailableAndFailurePhase(t *testing.T) {
	metrics := AggregateCommon([]CaseResult{{CaseID: "1", Status: StatusSucceeded, TraceComplete: true, Latency: 2 * time.Millisecond}})
	if metrics.Tokens.Available || metrics.Cost.Available {
		t.Fatal("missing token/cost must be unavailable")
	}
	if NormalizeFailurePhase("", "tool act failed") != "act" {
		t.Fatal("expected act phase")
	}
}

func TestReportRedactsSecretsPrivateIPsAndLimitsText(t *testing.T) {
	report := &Report{
		SchemaVersion: ReportSchemaVersion, RunID: "run-1", RunName: "redaction", DatasetRole: DatasetRegression,
		LabelSource: "test fixture",
		Profile:     ProfileDeterministic, Status: StatusSucceeded, TruthBoundary: TruthBoundary(ProfileDeterministic),
		Suites: []SuiteReport{{Name: SuiteTool, Status: StatusSucceeded, Cases: []CaseResult{{
			CaseID: "tool-1", Status: StatusSucceeded, Reason: "Bearer abc.def password=secret 10.1.2.3 very-long-value",
			Domain: json.RawMessage(`{"api_key":"sk-1234567890abcdef","address":"192.168.1.5","text":"abcdefghijklmnopqrstuvwxyz"}`),
		}}}},
	}
	dir := t.TempDir()
	jsonPath, markdownPath, err := WriteReport(report, dir, RedactionConfig{MaxTextChars: 20, SensitiveKeys: []string{"api_key", "password", "token"}})
	if err != nil {
		t.Fatal(err)
	}
	jsonData, _ := os.ReadFile(jsonPath)
	markdownData, _ := os.ReadFile(markdownPath)
	combined := string(jsonData) + string(markdownData)
	for _, forbidden := range []string{"abc.def", "secret", "10.1.2.3", "192.168.1.5", "sk-1234567890abcdef"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("report leaked %q", forbidden)
		}
	}
}

func TestFingerprintCompatibilityAllowsCandidateChangesButRejectsProtocolDrift(t *testing.T) {
	baseline := Fingerprints{Dataset: "dataset", Config: "config-a", Code: "code-a", CodeScope: "scope", Model: "model-a", Prompt: "prompt-a", Evaluator: "eval", EvidenceCorpus: "corpus"}
	candidate := Fingerprints{Dataset: "dataset", Config: "config-b", Code: "code-b", CodeScope: "scope", Model: "model-b", Prompt: "prompt-b", Evaluator: "eval", EvidenceCorpus: "corpus"}
	if err := CompareFingerprints(baseline, candidate); err != nil {
		t.Fatalf("candidate implementation changes should remain comparable: %v", err)
	}
	candidate.Dataset = "other"
	if err := CompareFingerprints(baseline, candidate); err == nil {
		t.Fatal("expected dataset incompatibility")
	}
}

func TestComparisonUsesMetricDirection(t *testing.T) {
	baseline := &Report{SchemaVersion: ReportSchemaVersion, DatasetRole: DatasetRegression, Profile: ProfileDeterministic, Fingerprints: Fingerprints{Dataset: "d", CodeScope: "s", Evaluator: "e", EvidenceCorpus: "c"}, Suites: []SuiteReport{{Name: SuitePlan, DomainSchema: "plan/v1", CommonMetrics: CommonMetrics{FailureRate: AvailableMetric(0.2, "ratio")}, DomainMetrics: MarshalDomain(map[string]float64{"completion_rate": 0.8, "premature_stop_rate": 0.2})}}}
	candidate := &Report{SchemaVersion: ReportSchemaVersion, DatasetRole: DatasetRegression, Profile: ProfileDeterministic, Fingerprints: baseline.Fingerprints, Suites: []SuiteReport{{Name: SuitePlan, DomainSchema: "plan/v1", CommonMetrics: CommonMetrics{FailureRate: AvailableMetric(0.1, "ratio")}, DomainMetrics: MarshalDomain(map[string]float64{"completion_rate": 0.9, "premature_stop_rate": 0.1})}}}
	results, err := CompareReports(baseline, candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if !result.Passed {
			t.Fatalf("expected improvement to pass: %#v", result)
		}
	}
}

func TestLoadRuntimeConfigDefaults(t *testing.T) {
	cfg := LoadRuntimeConfig(context.Background())
	if !cfg.Enabled || cfg.Budget.Concurrency <= 0 || cfg.Redaction.MaxTextChars <= 0 || len(cfg.DefaultSuites) != 6 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
