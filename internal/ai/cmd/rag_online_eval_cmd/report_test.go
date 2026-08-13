package main

import (
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/ai/rag/eval"
	"strings"
	"testing"
)

func TestDatasetIdentityFingerprintsEvaluatedCases(t *testing.T) {
	all := []eval.EvalCase{{ID: "1", Query: "q1"}, {ID: "2", Query: "q2"}}
	_, fullFingerprint, err := datasetIdentity("cases.jsonl", all)
	if err != nil {
		t.Fatalf("datasetIdentity returned error: %v", err)
	}
	_, limitedFingerprint, err := datasetIdentity("cases.jsonl", all[:1])
	if err != nil {
		t.Fatalf("datasetIdentity returned error: %v", err)
	}
	if fullFingerprint == limitedFingerprint {
		t.Fatal("limited evaluation must have a distinct dataset fingerprint")
	}
}

func TestCompareReportsRejectsLatencyRegression(t *testing.T) {
	baseline := comparableReport()
	candidate := comparableReport()
	candidate.Summary.MRR = 0.8
	candidate.Summary.AvgRecallAtK[5] = 0.9
	candidate.Summary.Latency.Total.P95Ms = 130

	result, err := compareReports(baseline, candidate, gateConfig{
		PrimaryMetric:                "mrr",
		MinPrimaryDelta:              0.05,
		MaxQualityRegression:         0,
		MaxFailureRate:               0,
		MaxEmptyRateDelta:            0,
		MaxP95LatencyRegressionRatio: 0.2,
	})
	if err != nil {
		t.Fatalf("compareReports returned error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected candidate to fail the latency gate")
	}
	if got := findGateCheck(result, "total_latency_p95_ms"); got == nil || got.Passed {
		t.Fatalf("expected failed latency check, got %+v", got)
	}
}

func TestCompareReportsPassesWithinConfiguredBudgets(t *testing.T) {
	baseline := comparableReport()
	candidate := comparableReport()
	candidate.Summary.MRR = 0.76
	candidate.Summary.AvgRecallAtK[5] = 0.9
	candidate.Summary.Latency.Total.P95Ms = 115

	result, err := compareReports(baseline, candidate, gateConfig{
		PrimaryMetric:                "mrr",
		MinPrimaryDelta:              0.01,
		MaxQualityRegression:         0,
		MaxFailureRate:               0,
		MaxEmptyRateDelta:            0,
		MaxP95LatencyRegressionRatio: 0.2,
	})
	if err != nil {
		t.Fatalf("compareReports returned error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected gate pass, got %+v", result.Checks)
	}
}

func TestCompareReportsRejectsIncomparableMetadata(t *testing.T) {
	baseline := comparableReport()
	candidate := comparableReport()
	candidate.Metadata.DatasetFingerprint = "different"
	_, err := compareReports(baseline, candidate, gateConfig{PrimaryMetric: "mrr"})
	if err == nil {
		t.Fatal("expected incomparable metadata error")
	}

	legacy := comparableReport()
	legacy.SchemaVersion = 0
	_, err = compareReports(legacy, candidate, gateConfig{PrimaryMetric: "mrr"})
	if err == nil {
		t.Fatal("expected legacy report rejection")
	}
}

func TestCompareReportsRejectsHoldoutAsDevelopmentBaseline(t *testing.T) {
	baseline := comparableReport()
	baseline.Metadata.DatasetRole = "holdout"
	_, err := compareReports(baseline, comparableReport(), gateConfig{PrimaryMetric: "mrr"})
	if err == nil || !strings.Contains(err.Error(), "dataset roles differ") {
		t.Fatalf("expected holdout isolation error, got %v", err)
	}
}

func TestCompareReportsRejectsSubgroupRegression(t *testing.T) {
	baseline := comparableReport()
	candidate := comparableReport()
	candidate.Summary.MRR = .8
	candidate.Analysis.ByCategory["hard_negative"] = eval.StratumMetrics{Cases: 15, AvgRecallAtK: map[int]float64{5: .5}}
	result, err := compareReports(baseline, candidate, gateConfig{PrimaryMetric: "mrr", MaxP95LatencyRegressionRatio: .2})
	if err != nil {
		t.Fatalf("compareReports returned error: %v", err)
	}
	if result.Passed || findGateCheck(result, "category_hard_negative_recall_at_5").Passed {
		t.Fatalf("hard-negative regression must fail gate: %+v", result)
	}
}

func TestValidateComparableReportsRejectsRetrievalBudgetMismatch(t *testing.T) {
	baseline := comparableReport()
	candidate := comparableReport()
	baseline.EffectiveConfig.Hybrid = rag.HybridConfig{DenseTopK: 50, LexicalTopK: 50, FusionK: 60, CandidateTopK: 20, FinalTopK: 5}
	candidate.EffectiveConfig.Hybrid = baseline.EffectiveConfig.Hybrid
	candidate.EffectiveConfig.Hybrid.FinalTopK = 10
	if err := validateComparableReports(baseline, candidate); err == nil {
		t.Fatal("expected hybrid budget mismatch rejection")
	}
}

func comparableReport() report {
	return report{
		SchemaVersion: reportSchemaVersion,
		Metadata: reportMetadata{
			DatasetRole:        "development",
			DatasetFingerprint: "dataset-sha",
			CorpusVersion:      "corpus-v1",
			Comparable:         true,
		},
		EffectiveConfig: effectiveRAGConfig{Ks: []int{1, 3, 5}, PerQueryTimeoutMs: 15000},
		Summary: eval.QuerySummary{
			Summary: eval.Summary{
				MRR:          0.75,
				AvgRecallAtK: map[int]float64{5: 0.9},
			},
			FailureRate: 0,
			EmptyRate:   0,
			Latency: eval.QueryLatencySummary{
				Total: eval.LatencyStats{P95Ms: 100},
			},
		},
		Analysis: eval.StratifiedAnalysis{ByCategory: map[string]eval.StratumMetrics{
			"multi_document": {Cases: 12, AvgRecallAtK: map[int]float64{5: .7}},
			"hard_negative":  {Cases: 15, AvgRecallAtK: map[int]float64{5: .6}},
		}},
	}
}

func findGateCheck(result gateResult, name string) *gateCheck {
	for i := range result.Checks {
		if result.Checks[i].Name == name {
			return &result.Checks[i]
		}
	}
	return nil
}
