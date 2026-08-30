package evalharness

import (
	"encoding/json"
	"time"
)

const (
	ManifestSchemaVersion = "evaluation-harness/v1"
	CaseSchemaVersion     = "evaluation-case/v1"
	ReportSchemaVersion   = "evaluation-report/v1"
)

type DatasetRole string

const (
	DatasetDevelopment DatasetRole = "development"
	DatasetRegression  DatasetRole = "regression"
	DatasetHoldout     DatasetRole = "holdout"
)

type Profile string

const (
	ProfileDeterministic Profile = "deterministic"
	ProfileRecorded      Profile = "recorded"
	ProfileLive          Profile = "live"
)

type SuiteName string

const (
	SuiteRoute    SuiteName = "route"
	SuiteRAG      SuiteName = "rag"
	SuitePlan     SuiteName = "plan"
	SuiteGoS      SuiteName = "gos"
	SuiteTool     SuiteName = "tool"
	SuiteEvidence SuiteName = "evidence"
)

type SuiteStatus string

const (
	StatusSucceeded      SuiteStatus = "succeeded"
	StatusDegraded       SuiteStatus = "degraded"
	StatusFailed         SuiteStatus = "failed"
	StatusSkipped        SuiteStatus = "skipped"
	StatusBudgetExceeded SuiteStatus = "budget_exceeded"
)

type MetricValue struct {
	Available bool    `json:"available" yaml:"available"`
	Value     float64 `json:"value" yaml:"value"`
	Unit      string  `json:"unit,omitempty" yaml:"unit,omitempty"`
}

func AvailableMetric(value float64, unit string) MetricValue {
	return MetricValue{Available: true, Value: value, Unit: unit}
}

func UnavailableMetric(unit string) MetricValue {
	return MetricValue{Available: false, Unit: unit}
}

type Usage struct {
	LLMCalls  int     `json:"llm_calls" yaml:"llm_calls"`
	ToolCalls int     `json:"tool_calls" yaml:"tool_calls"`
	RAGCalls  int     `json:"rag_calls" yaml:"rag_calls"`
	Tokens    int     `json:"tokens" yaml:"tokens"`
	Cost      float64 `json:"cost" yaml:"cost"`
}

type BudgetLimits struct {
	MaxCases       int           `json:"max_cases" yaml:"max_cases"`
	Concurrency    int           `json:"concurrency" yaml:"concurrency"`
	CaseTimeout    time.Duration `json:"case_timeout" yaml:"-"`
	TotalTimeout   time.Duration `json:"total_timeout" yaml:"-"`
	CaseTimeoutMS  int64         `json:"case_timeout_ms" yaml:"case_timeout_ms"`
	TotalTimeoutMS int64         `json:"total_timeout_ms" yaml:"total_timeout_ms"`
	MaxLLMCalls    int           `json:"max_llm_calls" yaml:"max_llm_calls"`
	MaxToolCalls   int           `json:"max_tool_calls" yaml:"max_tool_calls"`
	MaxRAGCalls    int           `json:"max_rag_calls" yaml:"max_rag_calls"`
	MaxTokens      int           `json:"max_tokens" yaml:"max_tokens"`
	MaxCost        float64       `json:"max_cost" yaml:"max_cost"`
}

func (b *BudgetLimits) Normalize() {
	if b.Concurrency <= 0 {
		b.Concurrency = 1
	}
	if b.CaseTimeout <= 0 && b.CaseTimeoutMS > 0 {
		b.CaseTimeout = time.Duration(b.CaseTimeoutMS) * time.Millisecond
	}
	if b.TotalTimeout <= 0 && b.TotalTimeoutMS > 0 {
		b.TotalTimeout = time.Duration(b.TotalTimeoutMS) * time.Millisecond
	}
}

type RedactionConfig struct {
	MaxTextChars  int      `json:"max_text_chars" yaml:"max_text_chars"`
	SensitiveKeys []string `json:"sensitive_keys" yaml:"sensitive_keys"`
}

type GateSeverity string

const (
	GateBlocking GateSeverity = "blocking"
	GateWarning  GateSeverity = "warning"
)

type GateSpec struct {
	Name      string       `json:"name" yaml:"name"`
	Suite     SuiteName    `json:"suite,omitempty" yaml:"suite,omitempty"`
	Metric    string       `json:"metric" yaml:"metric"`
	Operator  string       `json:"operator" yaml:"operator"`
	Threshold float64      `json:"threshold" yaml:"threshold"`
	Severity  GateSeverity `json:"severity" yaml:"severity"`
}

type SuiteConfig struct {
	Name          SuiteName       `json:"name" yaml:"name"`
	Enabled       bool            `json:"enabled" yaml:"enabled"`
	Dataset       string          `json:"dataset" yaml:"dataset"`
	DatasetSHA256 string          `json:"dataset_sha256,omitempty" yaml:"dataset_sha256,omitempty"`
	Baseline      string          `json:"baseline,omitempty" yaml:"baseline,omitempty"`
	PayloadSchema string          `json:"payload_schema" yaml:"payload_schema"`
	Budget        BudgetLimits    `json:"budget" yaml:"budget"`
	Gates         []GateSpec      `json:"gates,omitempty" yaml:"gates,omitempty"`
	Config        json.RawMessage `json:"config,omitempty" yaml:"-"`
	ConfigMap     map[string]any  `json:"-" yaml:"config,omitempty"`
}

type RegressionCorpusConfig struct {
	MinTotal      int                    `json:"min_total,omitempty" yaml:"min_total,omitempty"`
	SuiteMinimums map[SuiteName]int      `json:"suite_minimums,omitempty" yaml:"suite_minimums,omitempty"`
	RequiredTags  map[SuiteName][]string `json:"required_tags,omitempty" yaml:"required_tags,omitempty"`
}

type Manifest struct {
	SchemaVersion          string                 `json:"schema_version" yaml:"schema_version"`
	RunName                string                 `json:"run_name" yaml:"run_name"`
	DatasetRole            DatasetRole            `json:"dataset_role" yaml:"dataset_role"`
	LabelSource            string                 `json:"label_source" yaml:"label_source"`
	Profile                Profile                `json:"profile" yaml:"profile"`
	ContinueOnError        bool                   `json:"continue_on_error" yaml:"continue_on_error"`
	ReportDir              string                 `json:"report_dir" yaml:"report_dir"`
	Budget                 BudgetLimits           `json:"budget" yaml:"budget"`
	Redaction              RedactionConfig        `json:"redaction" yaml:"redaction"`
	Dependencies           map[string]string      `json:"dependencies" yaml:"dependencies"`
	Gates                  []GateSpec             `json:"gates,omitempty" yaml:"gates,omitempty"`
	RegressionCorpus       RegressionCorpusConfig `json:"regression_corpus,omitempty" yaml:"regression_corpus,omitempty"`
	Suites                 []SuiteConfig          `json:"suites" yaml:"suites"`
	CodeScope              string                 `json:"code_scope,omitempty" yaml:"code_scope,omitempty"`
	CodePaths              []string               `json:"code_paths,omitempty" yaml:"code_paths,omitempty"`
	ModelFingerprint       string                 `json:"model_fingerprint,omitempty" yaml:"model_fingerprint,omitempty"`
	PromptFingerprint      string                 `json:"prompt_fingerprint,omitempty" yaml:"prompt_fingerprint,omitempty"`
	EvaluatorFingerprint   string                 `json:"evaluator_fingerprint,omitempty" yaml:"evaluator_fingerprint,omitempty"`
	EvidenceCorpusSHA256   string                 `json:"evidence_corpus_sha256,omitempty" yaml:"evidence_corpus_sha256,omitempty"`
	EvidenceCorpusPaths    []string               `json:"evidence_corpus_paths,omitempty" yaml:"evidence_corpus_paths,omitempty"`
	ExternalCorpusManifest string                 `json:"external_corpus_manifest,omitempty" yaml:"external_corpus_manifest,omitempty"`
	SourcePath             string                 `json:"-" yaml:"-"`
}

type CaseInput struct {
	Query  string   `json:"query"`
	Memory []string `json:"memory,omitempty"`
}

type CaseEnvelope struct {
	SchemaVersion        string          `json:"schema_version"`
	ID                   string          `json:"id"`
	Suite                SuiteName       `json:"suite"`
	Input                CaseInput       `json:"input"`
	Expectation          json.RawMessage `json:"expectation"`
	Tags                 []string        `json:"tags,omitempty"`
	PayloadSchemaVersion string          `json:"payload_schema_version"`
	Payload              json.RawMessage `json:"payload"`
}

type Fingerprints struct {
	Dataset        string `json:"dataset,omitempty"`
	Config         string `json:"config,omitempty"`
	Code           string `json:"code,omitempty"`
	CodeScope      string `json:"code_scope,omitempty"`
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	Evaluator      string `json:"evaluator,omitempty"`
	EvidenceCorpus string `json:"evidence_corpus,omitempty"`
}

type Failure struct {
	Suite       SuiteName `json:"suite"`
	CaseID      string    `json:"case_id,omitempty"`
	Phase       string    `json:"phase"`
	Reason      string    `json:"reason"`
	TraceID     string    `json:"trace_id,omitempty"`
	EvidenceIDs []string  `json:"evidence_ids,omitempty"`
}

type CaseResult struct {
	CaseID        string          `json:"case_id"`
	Status        SuiteStatus     `json:"status"`
	Matched       bool            `json:"matched"`
	Latency       time.Duration   `json:"latency"`
	Usage         Usage           `json:"usage"`
	TraceComplete bool            `json:"trace_complete"`
	EvidenceCount int             `json:"evidence_count"`
	FailurePhase  string          `json:"failure_phase,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	TraceID       string          `json:"trace_id,omitempty"`
	EvidenceIDs   []string        `json:"evidence_ids,omitempty"`
	Domain        json.RawMessage `json:"domain,omitempty"`
}

type CommonMetrics struct {
	Cases             int         `json:"cases"`
	Succeeded         int         `json:"succeeded"`
	Failed            int         `json:"failed"`
	Degraded          int         `json:"degraded"`
	Skipped           int         `json:"skipped"`
	BudgetExceeded    int         `json:"budget_exceeded"`
	SuccessRate       MetricValue `json:"success_rate"`
	FailureRate       MetricValue `json:"failure_rate"`
	DegradationRate   MetricValue `json:"degradation_rate"`
	P95LatencyMS      MetricValue `json:"p95_latency_ms"`
	TraceCompleteness MetricValue `json:"trace_completeness"`
	EvidenceCoverage  MetricValue `json:"evidence_coverage"`
	LLMCalls          MetricValue `json:"llm_calls"`
	ToolCalls         MetricValue `json:"tool_calls"`
	RAGCalls          MetricValue `json:"rag_calls"`
	Tokens            MetricValue `json:"tokens"`
	Cost              MetricValue `json:"cost"`
}

type GateResult struct {
	Name      string       `json:"name"`
	Layer     string       `json:"layer"`
	Suite     SuiteName    `json:"suite,omitempty"`
	Metric    string       `json:"metric"`
	Operator  string       `json:"operator"`
	Threshold float64      `json:"threshold"`
	Baseline  *float64     `json:"baseline,omitempty"`
	Actual    *float64     `json:"actual,omitempty"`
	Delta     *float64     `json:"delta,omitempty"`
	Severity  GateSeverity `json:"severity"`
	Passed    bool         `json:"passed"`
	CaseRefs  []string     `json:"case_refs,omitempty"`
	Reason    string       `json:"reason,omitempty"`
}

type SuiteReport struct {
	Name          SuiteName       `json:"name"`
	Status        SuiteStatus     `json:"status"`
	CommonMetrics CommonMetrics   `json:"common_metrics"`
	DomainSchema  string          `json:"domain_schema"`
	DomainMetrics json.RawMessage `json:"domain_metrics,omitempty"`
	Gates         []GateResult    `json:"gates,omitempty"`
	Cases         []CaseResult    `json:"cases"`
	Failures      []Failure       `json:"failures,omitempty"`
	Fingerprints  Fingerprints    `json:"fingerprints"`
}

type Report struct {
	SchemaVersion     string              `json:"schema_version"`
	RunID             string              `json:"run_id"`
	RunName           string              `json:"run_name"`
	StartedAt         time.Time           `json:"started_at"`
	FinishedAt        time.Time           `json:"finished_at"`
	DatasetRole       DatasetRole         `json:"dataset_role"`
	LabelSource       string              `json:"label_source"`
	Profile           Profile             `json:"profile"`
	Status            SuiteStatus         `json:"status"`
	TruthBoundary     string              `json:"truth_boundary"`
	Dependencies      map[string]string   `json:"dependencies"`
	Usage             Usage               `json:"usage"`
	Budget            BudgetLimits        `json:"budget"`
	Fingerprints      Fingerprints        `json:"fingerprints"`
	ExternalCorpus    *CorpusProvenance   `json:"external_corpus,omitempty"`
	CoverageGaps      []CorpusCoverage    `json:"coverage_gaps,omitempty"`
	Suites            []SuiteReport       `json:"suites"`
	CrossSuiteGates   []GateResult        `json:"cross_suite_gates,omitempty"`
	PlanGoSComparison []PlanGoSComparison `json:"plan_gos_comparison,omitempty"`
	Failures          []Failure           `json:"failures,omitempty"`
}
