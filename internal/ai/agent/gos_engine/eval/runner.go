package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/protocol"
)

type EngineRunner interface {
	Run(ctx context.Context, symptom string) *protocol.TaskResult
}

type EngineFactory func(caseID string) (EngineRunner, error)

type Runner struct {
	engine  EngineRunner
	factory EngineFactory
}

func NewRunner(engine EngineRunner) *Runner {
	return &Runner{
		engine: engine,
	}
}

func NewCaseRunner(factory EngineFactory) *Runner {
	return &Runner{factory: factory}
}

func (r *Runner) RunFromFile(ctx context.Context, filePath string) (*EvalMetrics, []EvalResult, error) {
	dataset, err := LoadDataset(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load dataset: %w", err)
	}

	return r.RunFromCases(ctx, dataset.Cases)
}

func (r *Runner) RunFromCases(ctx context.Context, cases []EvalCase) (*EvalMetrics, []EvalResult, error) {
	metrics := NewEvalMetrics()
	results := make([]EvalResult, 0, len(cases))

	for _, c := range cases {
		engine, err := r.engineForCase(c.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("build engine for case %q: %w", c.ID, err)
		}
		result := r.runCase(ctx, engine, c)
		results = append(results, result)
		metrics.AddResult(&result)
	}

	metrics.Finalize()
	return metrics, results, nil
}

func (r *Runner) engineForCase(caseID string) (EngineRunner, error) {
	if r.factory != nil {
		return r.factory(caseID)
	}
	if r.engine == nil {
		return nil, fmt.Errorf("engine is required")
	}
	return r.engine, nil
}

func (r *Runner) runCase(ctx context.Context, engine EngineRunner, c EvalCase) EvalResult {
	start := time.Now()

	taskResult := engine.Run(ctx, c.Symptom)

	latency := time.Since(start)
	if taskResult == nil {
		status := string(protocol.ResultStatusFailed)
		phase := "report"
		statusMatched := c.ExpectedStatus == "" || c.ExpectedStatus == status
		phaseMatched := c.ExpectedFailurePhase == "" || c.ExpectedFailurePhase == phase
		return EvalResult{
			CaseID:               c.ID,
			Scenario:             c.Scenario,
			Symptom:              c.Symptom,
			GroundTruth:          c.GroundTruth,
			Prediction:           "[FAILED] engine returned nil result",
			Status:               status,
			ExpectedStatus:       c.ExpectedStatus,
			StatusMatched:        statusMatched,
			Latency:              latency,
			FailurePhase:         phase,
			ExpectedFailurePhase: c.ExpectedFailurePhase,
			FailurePhaseMatched:  phaseMatched,
			ContractMatched:      statusMatched && phaseMatched && !c.RequireRefine && !c.RequireBacktrack,
		}
	}

	llmCalls := metadataInt(taskResult.Metadata, "llm_calls")
	toolCalls := metadataInt(taskResult.Metadata, "tool_calls")
	ragCalls := metadataInt(taskResult.Metadata, "rag_calls")
	evidenceSourceCounts := make(map[string]int)
	for _, item := range taskResult.Evidence {
		evidenceSourceCounts[item.SourceType]++
	}
	diagnosticEvidence := diagnosticEvidenceItems(taskResult.Evidence)
	evidenceCount := len(diagnosticEvidence)
	relevantEvidence, expectedEvidence, coveredEvidence := evaluateEvidence(diagnosticEvidence, c.ExpectedEvidenceKeywords)

	matched := MatchPrediction(taskResult.Summary, c.GroundTruth, c.ExpectedKeywords)

	traceComplete := checkTraceComplete(taskResult)
	refined, backtracked := transitionFlags(taskResult.Metadata["fsm_history"])
	graphValid, _ := taskResult.Metadata["graph_valid"].(bool)
	statusMatches := c.ExpectedStatus == "" || c.ExpectedStatus == string(taskResult.Status)
	prematureStop := IsPrematureStop(
		string(taskResult.Status),
		statusMatches,
		c.RequireRefine,
		refined,
		c.RequireBacktrack,
		backtracked,
	)
	failurePhase := failurePhase(taskResult, matched && statusMatches, evidenceCount, graphValid, c, refined, backtracked)
	failurePhaseMatches := c.ExpectedFailurePhase == "" || c.ExpectedFailurePhase == failurePhase
	contractMatches := statusMatches && failurePhaseMatches && (!c.RequireRefine || refined) && (!c.RequireBacktrack || backtracked)

	prediction := taskResult.Summary
	if taskResult.Status == protocol.ResultStatusDegraded {
		prediction = fmt.Sprintf("[DEGRADED] %s", taskResult.Summary)
	}

	return EvalResult{
		CaseID:               c.ID,
		Symptom:              c.Symptom,
		Prediction:           prediction,
		GroundTruth:          c.GroundTruth,
		Matched:              matched,
		Latency:              latency,
		LLMCalls:             llmCalls,
		ToolCalls:            toolCalls,
		RAGCalls:             ragCalls,
		Status:               string(taskResult.Status),
		ExpectedStatus:       c.ExpectedStatus,
		StatusMatched:        statusMatches,
		EvidenceCount:        evidenceCount,
		RawEvidenceCount:     len(taskResult.Evidence),
		EvidenceSourceCounts: evidenceSourceCounts,
		RelevantEvidence:     relevantEvidence,
		ExpectedEvidence:     expectedEvidence,
		CoveredEvidence:      coveredEvidence,
		TraceComplete:        traceComplete,
		GraphValid:           graphValid,
		Refined:              refined,
		Backtracked:          backtracked,
		BacktrackRequired:    c.RequireBacktrack,
		PrematureStop:        prematureStop,
		FailurePhase:         failurePhase,
		ExpectedFailurePhase: c.ExpectedFailurePhase,
		FailurePhaseMatched:  failurePhaseMatches,
		ContractMatched:      contractMatches,
		Scenario:             c.Scenario,
		DegradationReason:    taskResult.DegradationReason,
		FailureCategories:    metadataStringIntMap(taskResult.Metadata, "failure_categories"),
	}
}

func metadataStringIntMap(metadata map[string]any, key string) map[string]int {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata[key].(map[string]int)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]int, len(raw))
	for category, count := range raw {
		out[category] = count
	}
	return out
}

func countDiagnosticEvidence(evidence []protocol.EvidenceItem) int {
	return len(diagnosticEvidenceItems(evidence))
}

func diagnosticEvidenceItems(evidence []protocol.EvidenceItem) []protocol.EvidenceItem {
	items := make([]protocol.EvidenceItem, 0, len(evidence))
	seenRecordedSignals := make(map[string]bool)
	for _, item := range evidence {
		if item.SourceType == "" || item.SourceType == "graph" {
			continue
		}
		if expanded, recorded := expandRecordedEvidenceItem(item); recorded {
			for _, signal := range expanded {
				key := strings.ToLower(signal.SourceType + "\x00" + signal.Snippet)
				if seenRecordedSignals[key] {
					continue
				}
				seenRecordedSignals[key] = true
				items = append(items, signal)
			}
			continue
		}
		items = append(items, item)
	}
	return items
}

func expandRecordedEvidenceItem(item protocol.EvidenceItem) ([]protocol.EvidenceItem, bool) {
	document := item.Snippet
	recorded := strings.Contains(document, "# Telemetry Evidence Case")
	if !recorded && strings.HasPrefix(strings.TrimSpace(document), "{") {
		var wrapper struct {
			ProvenanceProfile string `json:"provenance_profile"`
			Data              string `json:"data"`
		}
		if json.Unmarshal([]byte(document), &wrapper) == nil && wrapper.ProvenanceProfile == "recorded_blind" {
			recorded = true
			document = wrapper.Data
		}
	}
	if !recorded {
		return nil, false
	}

	section := ""
	items := make([]protocol.EvidenceItem, 0)
	for _, line := range strings.Split(document, "\n") {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "## Metric Signals":
			section = "recorded_metric"
			continue
		case "## Log Signals":
			section = "recorded_log"
			continue
		case "## Trace Signals":
			section = "recorded_trace"
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			section = ""
			continue
		}
		if section == "" || !strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "- no ") {
			continue
		}
		snippet := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		if snippet == "" {
			continue
		}
		items = append(items, protocol.EvidenceItem{
			SourceType: section,
			SourceID:   item.SourceID,
			Title:      strings.TrimPrefix(section, "recorded_") + " signal",
			Snippet:    snippet,
		})
	}
	return items, true
}

func checkTraceComplete(taskResult *protocol.TaskResult) bool {
	if taskResult.Metadata == nil {
		return false
	}

	hasBeliefGraph := false
	hasFSMHistory := false

	if _, ok := taskResult.Metadata["belief_graph"]; ok {
		hasBeliefGraph = true
	}
	if _, ok := taskResult.Metadata["fsm_history"]; ok {
		hasFSMHistory = true
	}

	hasEvidence := countDiagnosticEvidence(taskResult.Evidence) > 0
	hasArtifacts := len(taskResult.ArtifactRefs) > 0
	if taskResult.Status == protocol.ResultStatusDegraded || taskResult.Status == protocol.ResultStatusFailed {
		_, hasErrorPhase := taskResult.Metadata["error_phase"]
		return hasBeliefGraph && hasFSMHistory && hasErrorPhase
	}

	return hasBeliefGraph && hasFSMHistory && (hasEvidence || hasArtifacts)
}

func LoadCases(filePath string) ([]EvalCase, error) {
	dataset, err := LoadDataset(filePath)
	if err != nil {
		return nil, err
	}
	return dataset.Cases, nil
}

func metadataInt(metadata map[string]any, key string) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func evaluateEvidence(evidence []protocol.EvidenceItem, expectedKeywords []string) (int, int, int) {
	if len(expectedKeywords) == 0 {
		return len(evidence), 0, 0
	}
	relevant := 0
	covered := make(map[string]bool)
	for _, item := range evidence {
		text := strings.ToLower(strings.Join([]string{item.SourceType, item.Title, item.Snippet}, " "))
		itemRelevant := false
		for _, keyword := range expectedKeywords {
			normalized := strings.ToLower(strings.TrimSpace(keyword))
			if normalized != "" && strings.Contains(text, normalized) {
				covered[normalized] = true
				itemRelevant = true
			}
		}
		if itemRelevant {
			relevant++
		}
	}
	return relevant, len(expectedKeywords), len(covered)
}

func EvaluateEvidence(evidence []protocol.EvidenceItem, expectedKeywords []string) (int, int, int) {
	_, relevant, expected, covered := EvaluateEvidenceMetrics(evidence, expectedKeywords)
	return relevant, expected, covered
}

func EvaluateEvidenceMetrics(evidence []protocol.EvidenceItem, expectedKeywords []string) (int, int, int, int) {
	diagnostic := diagnosticEvidenceItems(evidence)
	relevant, expected, covered := evaluateEvidence(diagnostic, expectedKeywords)
	return len(diagnostic), relevant, expected, covered
}

func transitionFlags(raw any) (bool, bool) {
	if raw == nil {
		return false, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false, false
	}
	var history []belief.FSMTransition
	if err := json.Unmarshal(data, &history); err != nil {
		return false, false
	}
	refined := false
	backtracked := false
	for _, transition := range history {
		if transition.ToLevel > transition.FromLevel {
			refined = true
		}
		if transition.ToLevel < transition.FromLevel {
			backtracked = true
		}
	}
	return refined, backtracked
}

func failurePhase(taskResult *protocol.TaskResult, outcomeMatched bool, evidenceCount int, graphValid bool, evalCase EvalCase, refined, backtracked bool) string {
	if taskResult.Metadata != nil {
		if phase, ok := taskResult.Metadata["error_phase"].(string); ok {
			phase = normalizeFailurePhase(phase)
			if validFailurePhase(phase) {
				return phase
			}
		}
	}
	if taskResult.Status == protocol.ResultStatusSucceeded && outcomeMatched && (!evalCase.RequireRefine || refined) && (!evalCase.RequireBacktrack || backtracked) {
		return ""
	}
	if !graphValid || evidenceCount == 0 {
		return "update"
	}
	if evalCase.RequireRefine && !refined || evalCase.RequireBacktrack && !backtracked {
		return "state"
	}
	return "report"
}

func normalizeFailurePhase(phase string) string {
	phase = strings.TrimSpace(strings.TrimSuffix(phase, "_failed"))
	switch phase {
	case "graph_invalid", "confidence_update":
		return "update"
	case "state_conversion", "budget_exhausted", "level_step_limit", "score_tie", "ambiguous_frontier":
		return "state"
	case "no_frontier":
		return "report"
	default:
		return phase
	}
}

func CheckGate(metrics *EvalMetrics, baseline *EvalMetrics) *GateReport {
	report := &GateReport{
		AllPassed: true,
		Gates:     make([]GateResult, 0),
	}

	report.Gates = append(report.Gates, GateResult{
		Name:     "root_cause_accuracy",
		Passed:   rootCauseAccuracy(metrics) >= rootCauseAccuracy(baseline),
		Expected: fmt.Sprintf(">= %.2f%%", rootCauseAccuracy(baseline)*100),
		Actual:   fmt.Sprintf("%.2f%%", rootCauseAccuracy(metrics)*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "contract_compliance",
		Passed:   metrics.ContractCompliance >= baseline.ContractCompliance,
		Expected: fmt.Sprintf(">= %.2f%%", baseline.ContractCompliance*100),
		Actual:   fmt.Sprintf("%.2f%%", metrics.ContractCompliance*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "evidence_precision",
		Passed:   metrics.EvidencePrecision >= baseline.EvidencePrecision,
		Expected: fmt.Sprintf(">= %.2f%%", baseline.EvidencePrecision*100),
		Actual:   fmt.Sprintf("%.2f%%", metrics.EvidencePrecision*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "evidence_coverage",
		Passed:   metrics.EvidenceCoverage >= baseline.EvidenceCoverage,
		Expected: fmt.Sprintf(">= %.2f%%", baseline.EvidenceCoverage*100),
		Actual:   fmt.Sprintf("%.2f%%", metrics.EvidenceCoverage*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "backtrack_success",
		Passed:   metrics.BacktrackSuccess >= baseline.BacktrackSuccess,
		Expected: fmt.Sprintf(">= %.2f%%", baseline.BacktrackSuccess*100),
		Actual:   fmt.Sprintf("%.2f%%", metrics.BacktrackSuccess*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "premature_stop_rate",
		Passed:   metrics.PrematureStopRate <= baseline.PrematureStopRate,
		Expected: fmt.Sprintf("<= %.2f%%", baseline.PrematureStopRate*100),
		Actual:   fmt.Sprintf("%.2f%%", metrics.PrematureStopRate*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "graph_validity",
		Passed:   metrics.GraphValidity == 1,
		Expected: "= 100%",
		Actual:   fmt.Sprintf("%.2f%%", metrics.GraphValidity*100),
	})

	baselineP95 := baseline.P95Latency
	if baselineP95 == 0 {
		baselineP95 = baseline.AvgLatency
	}
	if baselineP95 < 100*time.Millisecond {
		baselineP95 = 100 * time.Millisecond
	}
	actualP95 := metrics.P95Latency
	if actualP95 == 0 {
		actualP95 = metrics.AvgLatency
	}
	report.Gates = append(report.Gates, GateResult{
		Name:     "p95_latency",
		Passed:   actualP95 <= baselineP95*3/2,
		Expected: fmt.Sprintf("<= %v", baselineP95*3/2),
		Actual:   fmt.Sprintf("%v", actualP95),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "llm_calls",
		Passed:   metrics.AvgLLMCalls <= baseline.AvgLLMCalls*2,
		Expected: fmt.Sprintf("<= %.1f", baseline.AvgLLMCalls*2),
		Actual:   fmt.Sprintf("%.1f", metrics.AvgLLMCalls),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "degradation_rate",
		Passed:   metrics.DegradationRate <= baseline.DegradationRate,
		Expected: fmt.Sprintf("<= %.2f%%", baseline.DegradationRate*100),
		Actual:   fmt.Sprintf("%.2f%%", metrics.DegradationRate*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "traceability",
		Passed:   metrics.Traceability == 1.0,
		Expected: "= 100%",
		Actual:   fmt.Sprintf("%.2f%%", metrics.Traceability*100),
	})

	for _, g := range report.Gates {
		if !g.Passed {
			report.AllPassed = false
			break
		}
	}

	return report
}

func rootCauseAccuracy(metrics *EvalMetrics) float64 {
	if metrics.RootCauseAccuracy != 0 || metrics.Accuracy == 0 {
		return metrics.RootCauseAccuracy
	}
	return metrics.Accuracy
}
