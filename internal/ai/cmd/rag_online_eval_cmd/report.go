package main

import (
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/ai/rag/eval"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const reportSchemaVersion = 5

type report struct {
	SchemaVersion   int                     `json:"schema_version"`
	Mode            string                  `json:"mode"`
	Metadata        reportMetadata          `json:"metadata"`
	EffectiveConfig effectiveRAGConfig      `json:"effective_config"`
	Summary         eval.QuerySummary       `json:"summary"`
	Analysis        eval.StratifiedAnalysis `json:"analysis"`
	Results         []eval.QueryCaseResult  `json:"results"`
	Gate            *gateResult             `json:"gate,omitempty"`
}

type reportMetadata struct {
	DatasetRole         string `json:"dataset_role"`
	DatasetID           string `json:"dataset_id"`
	DatasetFingerprint  string `json:"dataset_fingerprint"`
	CorpusVersion       string `json:"corpus_version"`
	CodeRevision        string `json:"code_revision"`
	GeneratedAt         string `json:"generated_at"`
	EvidenceEnvironment string `json:"evidence_environment"`
	Comparable          bool   `json:"comparable"`
}

type effectiveRAGConfig struct {
	RewriteEnabled      bool             `json:"rewrite_enabled"`
	RerankEnabled       bool             `json:"rerank_enabled"`
	Hybrid              rag.HybridConfig `json:"hybrid"`
	RetrieverTopK       int              `json:"retriever_top_k"`
	RerankMaxCandidates int              `json:"rerank_max_candidates"`
	RewriteTimeoutMs    int              `json:"rewrite_timeout_ms"`
	RerankTimeoutMs     int              `json:"rerank_timeout_ms"`
	Ks                  []int            `json:"ks"`
	PerQueryTimeoutMs   int              `json:"per_query_timeout_ms"`
}

type gateConfig struct {
	PrimaryMetric                       string
	MinPrimaryDelta                     float64
	MaxQualityRegression                float64
	MaxFailureRate                      float64
	MaxEmptyRateDelta                   float64
	MaxP95LatencyRegressionRatio        float64
	MaxMultiDocumentRecallAt5Regression float64
	MaxHardNegativeRecallAt5Regression  float64
}

type gateCheck struct {
	Name      string  `json:"name"`
	Baseline  float64 `json:"baseline"`
	Candidate float64 `json:"candidate"`
	Delta     float64 `json:"delta"`
	Limit     float64 `json:"limit"`
	Passed    bool    `json:"passed"`
}

type gateResult struct {
	Passed bool        `json:"passed"`
	Checks []gateCheck `json:"checks"`
}

func validateRunInputs(evalPath, datasetRole, corpusVersion, outPath string) error {
	role := strings.ToLower(strings.TrimSpace(datasetRole))
	if role != "development" && role != "holdout" {
		return fmt.Errorf("dataset-role must be development or holdout")
	}
	if strings.TrimSpace(outPath) == "" {
		return fmt.Errorf("out is required so the evaluation is recorded")
	}
	if strings.TrimSpace(evalPath) != "" && strings.TrimSpace(corpusVersion) == "" {
		return fmt.Errorf("corpus-version is required for external evaluation datasets")
	}
	return nil
}

func datasetIdentity(evalPath string, cases []eval.EvalCase) (string, string, error) {
	id := "builtin-sample-v1"
	if strings.TrimSpace(evalPath) != "" {
		id = filepath.Base(evalPath)
	}
	raw, err := json.Marshal(cases)
	if err != nil {
		return "", "", fmt.Errorf("fingerprint dataset: %w", err)
	}
	sum := sha256.Sum256(raw)
	return id, hex.EncodeToString(sum[:]), nil
}

func captureEffectiveConfig(
	ctx context.Context,
	wantRewrite, wantRerank, useConfigDefaults bool,
	hybrid rag.HybridConfig,
	ks []int,
	perQueryTimeoutMs int,
) effectiveRAGConfig {
	if useConfigDefaults {
		wantRewrite = configBool(ctx, "rag.rewrite_enabled")
		wantRerank = configBool(ctx, "rag.rerank_enabled")
	}
	return effectiveRAGConfig{
		RewriteEnabled:      wantRewrite,
		RerankEnabled:       wantRerank,
		Hybrid:              hybrid,
		RetrieverTopK:       rag.RetrieverTopK(ctx),
		RerankMaxCandidates: rag.RerankMaxCandidates(ctx),
		RewriteTimeoutMs:    configInt(ctx, "rag.rewrite_timeout_ms"),
		RerankTimeoutMs:     configInt(ctx, "rag.rerank_timeout_ms"),
		Ks:                  append([]int(nil), ks...),
		PerQueryTimeoutMs:   perQueryTimeoutMs,
	}
}

func newReportMetadata(role, datasetID, fingerprint, corpusVersion string) reportMetadata {
	return reportMetadata{
		DatasetRole:         strings.ToLower(strings.TrimSpace(role)),
		DatasetID:           datasetID,
		DatasetFingerprint:  fingerprint,
		CorpusVersion:       corpusVersion,
		CodeRevision:        codeRevision(),
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
		EvidenceEnvironment: "offline-" + strings.ToLower(strings.TrimSpace(role)),
		Comparable:          true,
	}
}

func codeRevision() string {
	info, ok := debug.ReadBuildInfo()
	revision := "unknown"
	modified := false
	if ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if strings.TrimSpace(setting.Value) != "" {
					revision = setting.Value
				}
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
	}
	if revision == "unknown" {
		if raw, err := exec.Command("git", "rev-parse", "--verify", "HEAD").Output(); err == nil {
			revision = strings.TrimSpace(string(raw))
			if status, statusErr := exec.Command("git", "status", "--porcelain").Output(); statusErr == nil {
				modified = len(bytes.TrimSpace(status)) > 0
			}
		}
	}
	if modified {
		revision += "+dirty"
	}
	return revision
}

func loadGateConfig(ctx context.Context) gateConfig {
	return gateConfig{
		PrimaryMetric:                       configString(ctx, "rag.eval_gate.primary_metric"),
		MinPrimaryDelta:                     configFloat(ctx, "rag.eval_gate.min_primary_delta"),
		MaxQualityRegression:                configFloat(ctx, "rag.eval_gate.max_quality_regression"),
		MaxFailureRate:                      configFloat(ctx, "rag.eval_gate.max_failure_rate"),
		MaxEmptyRateDelta:                   configFloat(ctx, "rag.eval_gate.max_empty_rate_delta"),
		MaxP95LatencyRegressionRatio:        configFloat(ctx, "rag.eval_gate.max_p95_latency_regression_ratio"),
		MaxMultiDocumentRecallAt5Regression: configFloat(ctx, "rag.eval_gate.max_multi_document_recall_at_5_regression"),
		MaxHardNegativeRecallAt5Regression:  configFloat(ctx, "rag.eval_gate.max_hard_negative_recall_at_5_regression"),
	}
}

func compareReports(baseline, candidate report, cfg gateConfig) (gateResult, error) {
	if err := validateComparableReports(baseline, candidate); err != nil {
		return gateResult{}, err
	}
	primary := strings.ToLower(strings.TrimSpace(cfg.PrimaryMetric))
	if primary == "" {
		return gateResult{}, fmt.Errorf("rag.eval_gate.primary_metric is required")
	}
	secondary := "recall_at_5"
	if primary == secondary {
		secondary = "mrr"
	}
	basePrimary, err := reportMetric(baseline, primary)
	if err != nil {
		return gateResult{}, err
	}
	candidatePrimary, err := reportMetric(candidate, primary)
	if err != nil {
		return gateResult{}, err
	}
	baseSecondary, err := reportMetric(baseline, secondary)
	if err != nil {
		return gateResult{}, err
	}
	candidateSecondary, err := reportMetric(candidate, secondary)
	if err != nil {
		return gateResult{}, err
	}

	checks := []gateCheck{
		newMinDeltaCheck("primary_"+primary, basePrimary, candidatePrimary, cfg.MinPrimaryDelta),
		newMinDeltaCheck("secondary_"+secondary, baseSecondary, candidateSecondary, -cfg.MaxQualityRegression),
		newMaxValueCheck("failure_rate", baseline.Summary.FailureRate, candidate.Summary.FailureRate, cfg.MaxFailureRate),
		newMaxDeltaCheck("empty_rate", baseline.Summary.EmptyRate, candidate.Summary.EmptyRate, cfg.MaxEmptyRateDelta),
	}
	baseP95 := baseline.Summary.Latency.Total.P95Ms
	maxP95 := baseP95 * (1 + cfg.MaxP95LatencyRegressionRatio)
	checks = append(checks, newMaxValueCheck("total_latency_p95_ms", baseP95, candidate.Summary.Latency.Total.P95Ms, maxP95))
	for _, subgroup := range []struct {
		name          string
		maxRegression float64
	}{
		{name: "multi_document", maxRegression: cfg.MaxMultiDocumentRecallAt5Regression},
		{name: "hard_negative", maxRegression: cfg.MaxHardNegativeRecallAt5Regression},
	} {
		baseMetric, err := subgroupRecallAt5(baseline, subgroup.name)
		if err != nil {
			return gateResult{}, err
		}
		candidateMetric, err := subgroupRecallAt5(candidate, subgroup.name)
		if err != nil {
			return gateResult{}, err
		}
		checks = append(checks, newMinDeltaCheck("category_"+subgroup.name+"_recall_at_5", baseMetric, candidateMetric, -subgroup.maxRegression))
	}

	result := gateResult{Passed: true, Checks: checks}
	for _, check := range checks {
		if !check.Passed {
			result.Passed = false
			break
		}
	}
	return result, nil
}

func subgroupRecallAt5(item report, category string) (float64, error) {
	metrics, ok := item.Analysis.ByCategory[category]
	if !ok || metrics.Cases == 0 {
		return 0, fmt.Errorf("report does not contain category %s", category)
	}
	value, ok := metrics.AvgRecallAtK[5]
	if !ok {
		return 0, fmt.Errorf("category %s does not contain recall@5", category)
	}
	return value, nil
}

func validateComparableReports(baseline, candidate report) error {
	for label, item := range map[string]report{"baseline": baseline, "candidate": candidate} {
		if item.SchemaVersion < reportSchemaVersion || !item.Metadata.Comparable {
			return fmt.Errorf("%s report lacks comparable v%d metadata", label, reportSchemaVersion)
		}
		if item.Metadata.DatasetFingerprint == "" || item.Metadata.CorpusVersion == "" || item.Metadata.DatasetRole == "" {
			return fmt.Errorf("%s report has incomplete dataset metadata", label)
		}
	}
	if baseline.Metadata.DatasetRole != candidate.Metadata.DatasetRole {
		return fmt.Errorf("dataset roles differ: %s vs %s", baseline.Metadata.DatasetRole, candidate.Metadata.DatasetRole)
	}
	if baseline.Metadata.DatasetFingerprint != candidate.Metadata.DatasetFingerprint {
		return fmt.Errorf("dataset fingerprints differ")
	}
	if baseline.Metadata.CorpusVersion != candidate.Metadata.CorpusVersion {
		return fmt.Errorf("corpus versions differ: %s vs %s", baseline.Metadata.CorpusVersion, candidate.Metadata.CorpusVersion)
	}
	if !reflect.DeepEqual(baseline.EffectiveConfig.Ks, candidate.EffectiveConfig.Ks) {
		return fmt.Errorf("evaluation k values differ")
	}
	if baseline.EffectiveConfig.PerQueryTimeoutMs != candidate.EffectiveConfig.PerQueryTimeoutMs {
		return fmt.Errorf("per-query timeouts differ")
	}
	if baseline.EffectiveConfig.Hybrid.DenseTopK != candidate.EffectiveConfig.Hybrid.DenseTopK ||
		baseline.EffectiveConfig.Hybrid.LexicalTopK != candidate.EffectiveConfig.Hybrid.LexicalTopK ||
		baseline.EffectiveConfig.Hybrid.FusionK != candidate.EffectiveConfig.Hybrid.FusionK ||
		baseline.EffectiveConfig.Hybrid.CandidateTopK != candidate.EffectiveConfig.Hybrid.CandidateTopK ||
		baseline.EffectiveConfig.Hybrid.FinalTopK != candidate.EffectiveConfig.Hybrid.FinalTopK {
		return fmt.Errorf("hybrid retrieval budgets differ")
	}
	return nil
}

func reportMetric(item report, metric string) (float64, error) {
	switch metric {
	case "mrr":
		return item.Summary.MRR, nil
	case "recall_at_5":
		value, ok := item.Summary.AvgRecallAtK[5]
		if !ok {
			return 0, fmt.Errorf("report does not contain recall@5")
		}
		return value, nil
	default:
		return 0, fmt.Errorf("unsupported primary metric %q", metric)
	}
}

func newMinDeltaCheck(name string, baseline, candidate, minDelta float64) gateCheck {
	delta := candidate - baseline
	return gateCheck{Name: name, Baseline: baseline, Candidate: candidate, Delta: delta, Limit: minDelta, Passed: delta >= minDelta}
}

func newMaxDeltaCheck(name string, baseline, candidate, maxDelta float64) gateCheck {
	delta := candidate - baseline
	return gateCheck{Name: name, Baseline: baseline, Candidate: candidate, Delta: delta, Limit: maxDelta, Passed: delta <= maxDelta}
}

func newMaxValueCheck(name string, baseline, candidate, maxValue float64) gateCheck {
	return gateCheck{Name: name, Baseline: baseline, Candidate: candidate, Delta: candidate - baseline, Limit: maxValue, Passed: candidate <= maxValue}
}

func loadReport(path string) (report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return report{}, fmt.Errorf("read baseline report: %w", err)
	}
	var item report
	if err := json.Unmarshal(raw, &item); err != nil {
		return report{}, fmt.Errorf("decode baseline report: %w", err)
	}
	return item, nil
}

func configBool(ctx context.Context, key string) bool {
	v, err := g.Cfg().Get(ctx, key)
	return err == nil && v.Bool()
}

func configInt(ctx context.Context, key string) int {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return 0
	}
	return v.Int()
}

func configFloat(ctx context.Context, key string) float64 {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return 0
	}
	return v.Float64()
}

func configString(ctx context.Context, key string) string {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v.String())
}
