package evalharness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteReport(report *Report, dir string, redaction RedactionConfig) (string, string, error) {
	if report == nil {
		return "", "", fmt.Errorf("report is required")
	}
	if strings.TrimSpace(report.RunID) == "" {
		return "", "", fmt.Errorf("run_id is required")
	}
	RedactReport(report, redaction)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", "", fmt.Errorf("create report dir: %w", err)
	}
	jsonPath := filepath.Join(dir, report.RunID+".json")
	markdownPath := filepath.Join(dir, report.RunID+".md")
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("encode report: %w", err)
	}
	markdown := RenderMarkdown(report)
	if err := atomicWrite(jsonPath, append(jsonData, '\n')); err != nil {
		return "", "", err
	}
	if err := atomicWrite(markdownPath, []byte(markdown)); err != nil {
		return "", "", err
	}
	return jsonPath, markdownPath, nil
}

func RenderMarkdown(report *Report) string {
	var output bytes.Buffer
	fmt.Fprintf(&output, "# Evaluation Harness: %s\n\n", report.RunName)
	fmt.Fprintf(&output, "- Run ID: `%s`\n", report.RunID)
	fmt.Fprintf(&output, "- Status: `%s`\n", report.Status)
	fmt.Fprintf(&output, "- Dataset: `%s`\n", report.DatasetRole)
	fmt.Fprintf(&output, "- Label source: %s\n", report.LabelSource)
	fmt.Fprintf(&output, "- Profile: `%s`\n", report.Profile)
	fmt.Fprintf(&output, "- Evidence boundary: %s\n\n", report.TruthBoundary)
	if report.ExternalCorpus != nil {
		output.WriteString("## External corpus\n\n")
		fmt.Fprintf(&output, "- Source: `%s`\n- Version: `%s`\n- License: %s\n- Split: `%s`\n- Split fingerprint: `%s`\n\n", report.ExternalCorpus.Source, report.ExternalCorpus.Version, report.ExternalCorpus.License, report.ExternalCorpus.SplitStrategy, report.ExternalCorpus.SplitFingerprint)
	}
	if len(report.CoverageGaps) > 0 {
		output.WriteString("## Coverage gaps\n\n")
		for _, gap := range report.CoverageGaps {
			fmt.Fprintf(&output, "- `%s`: %d available / %d target; gap %d. No synthetic fill was used.\n", gap.Role, gap.Cases, gap.Target, gap.Gap)
		}
		output.WriteByte('\n')
	}
	output.WriteString("## Dependencies\n\n")
	for _, name := range sortedMapKeys(report.Dependencies) {
		fmt.Fprintf(&output, "- `%s`: %s\n", name, report.Dependencies[name])
	}
	output.WriteString("\n## Fingerprints\n\n")
	fmt.Fprintf(&output, "- Dataset: `%s`\n", report.Fingerprints.Dataset)
	fmt.Fprintf(&output, "- Config: `%s`\n", report.Fingerprints.Config)
	fmt.Fprintf(&output, "- Code: `%s` (`%s`)\n", report.Fingerprints.Code, report.Fingerprints.CodeScope)
	fmt.Fprintf(&output, "- Model: `%s`\n", report.Fingerprints.Model)
	fmt.Fprintf(&output, "- Prompt: `%s`\n", report.Fingerprints.Prompt)
	fmt.Fprintf(&output, "- Evaluator: `%s`\n", report.Fingerprints.Evaluator)
	fmt.Fprintf(&output, "- Evidence corpus: `%s`\n\n", report.Fingerprints.EvidenceCorpus)
	output.WriteString("| Suite | Status | Cases | Failure rate | P95 ms | Trace |\n")
	output.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, suite := range report.Suites {
		fmt.Fprintf(&output, "| %s | %s | %d | %s | %s | %s |\n",
			suite.Name, suite.Status, suite.CommonMetrics.Cases,
			formatMetric(suite.CommonMetrics.FailureRate), formatMetric(suite.CommonMetrics.P95LatencyMS),
			formatMetric(suite.CommonMetrics.TraceCompleteness))
	}
	output.WriteString("\n## Gates\n\n")
	for _, suite := range report.Suites {
		for _, gate := range suite.Gates {
			fmt.Fprintf(&output, "- [%s] `%s/%s`: %s\n", gateStatus(gate.Passed), suite.Name, gate.Name, gateSummary(gate))
		}
	}
	for _, gate := range report.CrossSuiteGates {
		fmt.Fprintf(&output, "- [%s] `cross/%s`: %s\n", gateStatus(gate.Passed), gate.Name, gateSummary(gate))
	}
	if len(report.PlanGoSComparison) > 0 {
		fmt.Fprintf(&output, "\n## Plan / GoS comparison\n\n- Shared cases: %d\n- Scope: common fields are comparable; domain metrics remain separate.\n", len(report.PlanGoSComparison))
	}
	if len(report.Failures) > 0 {
		output.WriteString("\n## Failures\n\n")
		for _, failure := range report.Failures {
			fmt.Fprintf(&output, "- `%s/%s` at `%s`: %s%s\n", failure.Suite, failure.CaseID, failure.Phase, failure.Reason, failureRefs(failure))
		}
	}
	return output.String()
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func failureRefs(failure Failure) string {
	parts := make([]string, 0, 2)
	if failure.TraceID != "" {
		parts = append(parts, "trace="+failure.TraceID)
	}
	if len(failure.EvidenceIDs) > 0 {
		parts = append(parts, "evidence="+strings.Join(failure.EvidenceIDs, ","))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".eval-report-*")
	if err != nil {
		return fmt.Errorf("create temporary report: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary report: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temporary report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary report: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	return nil
}

func formatMetric(metric MetricValue) string {
	if !metric.Available {
		return "N/A"
	}
	return fmt.Sprintf("%.4g", metric.Value)
}

func gateStatus(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

func gateSummary(gate GateResult) string {
	if gate.Reason != "" {
		return gate.Reason
	}
	if gate.Actual != nil {
		return fmt.Sprintf("actual %.6g satisfies %s %.6g", *gate.Actual, gate.Operator, gate.Threshold)
	}
	return "passed"
}
