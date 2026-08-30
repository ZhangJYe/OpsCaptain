package evalharness

import (
	"encoding/json"
	"fmt"
	"strings"
)

func EvaluateGate(spec GateSpec, metrics map[string]MetricValue, layer string) GateResult {
	result := GateResult{
		Name: spec.Name, Layer: layer, Suite: spec.Suite, Metric: spec.Metric,
		Operator: spec.Operator, Threshold: spec.Threshold, Severity: spec.Severity,
	}
	metric, ok := metrics[spec.Metric]
	if !ok || !metric.Available {
		result.Reason = "metric unavailable"
		return result
	}
	actual := metric.Value
	result.Actual = &actual
	result.Passed = compareValue(actual, spec.Operator, spec.Threshold)
	if !result.Passed {
		result.Reason = fmt.Sprintf("actual %.6g does not satisfy %s %.6g", actual, spec.Operator, spec.Threshold)
	}
	return result
}

func CommonMetricMap(metrics CommonMetrics) map[string]MetricValue {
	return map[string]MetricValue{
		"success_rate":       metrics.SuccessRate,
		"failure_rate":       metrics.FailureRate,
		"degradation_rate":   metrics.DegradationRate,
		"p95_latency_ms":     metrics.P95LatencyMS,
		"trace_completeness": metrics.TraceCompleteness,
		"evidence_coverage":  metrics.EvidenceCoverage,
		"llm_calls":          metrics.LLMCalls,
		"tool_calls":         metrics.ToolCalls,
		"rag_calls":          metrics.RAGCalls,
		"tokens":             metrics.Tokens,
		"cost":               metrics.Cost,
	}
}

func DomainMetricMap(raw json.RawMessage) map[string]MetricValue {
	if len(raw) == 0 {
		return nil
	}
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	metrics := make(map[string]MetricValue)
	flattenMetrics("", decoded, metrics)
	return metrics
}

func CompareReports(baseline, candidate *Report) ([]GateResult, error) {
	if baseline == nil || candidate == nil {
		return nil, fmt.Errorf("baseline and candidate are required")
	}
	if baseline.SchemaVersion != candidate.SchemaVersion || baseline.DatasetRole != candidate.DatasetRole || baseline.Profile != candidate.Profile {
		return nil, fmt.Errorf("baseline and candidate report contracts are incompatible")
	}
	if err := CompareFingerprints(baseline.Fingerprints, candidate.Fingerprints); err != nil {
		return nil, err
	}
	baselineSuites := make(map[SuiteName]SuiteReport, len(baseline.Suites))
	for _, suite := range baseline.Suites {
		baselineSuites[suite.Name] = suite
	}
	var results []GateResult
	for _, candidateSuite := range candidate.Suites {
		baselineSuite, ok := baselineSuites[candidateSuite.Name]
		if !ok || baselineSuite.DomainSchema != candidateSuite.DomainSchema {
			return nil, fmt.Errorf("suite %s is not comparable", candidateSuite.Name)
		}
		metricLayers := []struct {
			name      string
			baseline  map[string]MetricValue
			candidate map[string]MetricValue
		}{
			{name: "common", baseline: CommonMetricMap(baselineSuite.CommonMetrics), candidate: CommonMetricMap(candidateSuite.CommonMetrics)},
			{name: "domain", baseline: DomainMetricMap(baselineSuite.DomainMetrics), candidate: DomainMetricMap(candidateSuite.DomainMetrics)},
		}
		for _, layer := range metricLayers {
			for metric, base := range layer.baseline {
				candidateValue, exists := layer.candidate[metric]
				if !exists || !base.Available || !candidateValue.Available {
					continue
				}
				baseValue := base.Value
				actualValue := candidateValue.Value
				delta := actualValue - baseValue
				operator := comparisonOperator(metric)
				results = append(results, GateResult{
					Name:  "compare_" + strings.ReplaceAll(string(candidateSuite.Name)+"_"+layer.name+"_"+metric, ".", "_"),
					Layer: "comparison_" + layer.name, Suite: candidateSuite.Name, Metric: metric, Operator: operator,
					Baseline: &baseValue, Actual: &actualValue, Delta: &delta, Severity: GateWarning, Passed: compareValue(actualValue, operator, baseValue),
				})
			}
		}
	}
	return results, nil
}

func comparisonOperator(metric string) string {
	lowerIsBetter := []string{"failure", "degradation", "latency", "premature_stop", "low_confidence", "calls", "tokens", "cost"}
	for _, marker := range lowerIsBetter {
		if strings.Contains(metric, marker) {
			return "<="
		}
	}
	return ">="
}

func EvaluateCrossSuiteGates(suites []SuiteReport) []GateResult {
	var cases []CaseResult
	routeIncidents := make(map[string]struct{})
	diagnosticsByCase := make(map[string][]CaseResult)
	hasRouteSuite := false
	hasDiagnosticSuite := false
	for _, suite := range suites {
		cases = append(cases, suite.Cases...)
		switch suite.Name {
		case SuiteRoute:
			hasRouteSuite = true
			for _, result := range suite.Cases {
				var route struct {
					ActualDecision string `json:"actual_decision"`
				}
				_ = json.Unmarshal(result.Domain, &route)
				if route.ActualDecision == "incident" {
					routeIncidents[result.CaseID] = struct{}{}
				}
			}
		case SuitePlan, SuiteGoS:
			hasDiagnosticSuite = true
			for _, result := range suite.Cases {
				diagnosticsByCase[result.CaseID] = append(diagnosticsByCase[result.CaseID], result)
			}
		}
	}
	totalDiagnostic := 0
	traceComplete := 0
	evidenceComplete := 0
	permissionViolations := 0
	var traceRefs, evidenceRefs, permissionRefs []string
	for _, result := range cases {
		var metadata struct {
			Diagnostic       bool `json:"diagnostic"`
			PermissionDenied bool `json:"permission_denied"`
			Executed         bool `json:"executed"`
			RequiresEvidence bool `json:"requires_evidence"`
		}
		_ = json.Unmarshal(result.Domain, &metadata)
		if metadata.Diagnostic && isDiagnosticCase(result.CaseID, diagnosticsByCase) {
			totalDiagnostic++
			if result.TraceComplete {
				traceComplete++
			} else {
				traceRefs = append(traceRefs, result.CaseID)
			}
			if !metadata.RequiresEvidence || result.EvidenceCount > 0 {
				evidenceComplete++
			} else {
				evidenceRefs = append(evidenceRefs, result.CaseID)
			}
		}
		if metadata.PermissionDenied && metadata.Executed {
			permissionViolations++
			permissionRefs = append(permissionRefs, result.CaseID)
		}
	}
	permissionActual := float64(permissionViolations)
	results := []GateResult{{
		Name: "permission_denial_not_executed", Layer: "cross_suite", Metric: "permission_violations",
		Operator: "==", Threshold: 0, Actual: &permissionActual, Severity: GateBlocking, Passed: permissionViolations == 0, CaseRefs: permissionRefs,
	}}
	if hasRouteSuite && hasDiagnosticSuite {
		covered := 0
		var missing []string
		for caseID := range routeIncidents {
			if len(diagnosticsByCase[caseID]) > 0 {
				covered++
			} else {
				missing = append(missing, caseID)
			}
		}
		coverage := ratioInt(covered, len(routeIncidents))
		results = append(results, GateResult{Name: "incident_route_has_diagnosis", Layer: "cross_suite", Metric: "route_diagnosis_coverage", Operator: "==", Threshold: 1, Actual: &coverage, Severity: GateBlocking, Passed: coverage == 1, CaseRefs: missing})
	}
	if totalDiagnostic > 0 {
		traceRatio := float64(traceComplete) / float64(totalDiagnostic)
		evidenceRatio := float64(evidenceComplete) / float64(totalDiagnostic)
		results = append(results,
			GateResult{Name: "diagnostic_trace_complete", Layer: "cross_suite", Metric: "trace_completeness", Operator: "==", Threshold: 1, Actual: &traceRatio, Severity: GateBlocking, Passed: traceRatio == 1, CaseRefs: traceRefs},
			GateResult{Name: "diagnostic_evidence_complete", Layer: "cross_suite", Metric: "evidence_completeness", Operator: "==", Threshold: 1, Actual: &evidenceRatio, Severity: GateBlocking, Passed: evidenceRatio == 1, CaseRefs: evidenceRefs},
		)
	}
	return results
}

func isDiagnosticCase(caseID string, diagnostics map[string][]CaseResult) bool {
	return len(diagnostics[caseID]) > 0
}

func compareValue(actual float64, operator string, expected float64) bool {
	switch operator {
	case ">=":
		return actual >= expected
	case ">":
		return actual > expected
	case "<=":
		return actual <= expected
	case "<":
		return actual < expected
	case "==":
		return actual == expected
	default:
		return false
	}
}

func flattenMetrics(prefix string, value map[string]any, out map[string]MetricValue) {
	for key, raw := range value {
		name := key
		if prefix != "" {
			name = prefix + "." + key
		}
		switch typed := raw.(type) {
		case float64:
			out[name] = AvailableMetric(typed, "")
		case map[string]any:
			flattenMetrics(name, typed, out)
		}
	}
}
