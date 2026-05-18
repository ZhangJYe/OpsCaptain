package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/protocol"
)

type Runner struct {
	engine *gos_engine.GoSEngine
}

func NewRunner(engine *gos_engine.GoSEngine) *Runner {
	return &Runner{
		engine: engine,
	}
}

func (r *Runner) RunFromFile(ctx context.Context, filePath string) (*EvalMetrics, []EvalResult, error) {
	cases, err := loadCases(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load cases: %w", err)
	}

	return r.RunFromCases(ctx, cases)
}

func (r *Runner) RunFromCases(ctx context.Context, cases []EvalCase) (*EvalMetrics, []EvalResult, error) {
	metrics := NewEvalMetrics()
	results := make([]EvalResult, 0, len(cases))

	for _, c := range cases {
		result := r.runCase(ctx, c)
		results = append(results, result)
		metrics.AddResult(&result)
	}

	metrics.Finalize()
	return metrics, results, nil
}

func (r *Runner) runCase(ctx context.Context, c EvalCase) EvalResult {
	start := time.Now()

	taskResult := r.engine.Run(ctx, c.Symptom)

	latency := time.Since(start)

	llmCalls := 0
	if meta, ok := taskResult.Metadata["llm_calls"].(int); ok {
		llmCalls = meta
	}

	evidenceCount := countDiagnosticEvidence(taskResult.Evidence)

	matched := MatchPrediction(taskResult.Summary, c.GroundTruth, c.ExpectedKeywords)

	traceComplete := checkTraceComplete(taskResult)

	prediction := taskResult.Summary
	if taskResult.Status == protocol.ResultStatusDegraded {
		prediction = fmt.Sprintf("[DEGRADED] %s", taskResult.Summary)
	}

	return EvalResult{
		CaseID:            c.ID,
		Symptom:           c.Symptom,
		Prediction:        prediction,
		GroundTruth:       c.GroundTruth,
		Matched:           matched,
		Latency:           latency,
		LLMCalls:          llmCalls,
		Status:            string(taskResult.Status),
		EvidenceCount:     evidenceCount,
		TraceComplete:     traceComplete,
		DegradationReason: taskResult.DegradationReason,
	}
}

func countDiagnosticEvidence(evidence []protocol.EvidenceItem) int {
	count := 0
	for _, item := range evidence {
		if item.SourceType == "" || item.SourceType == "graph" {
			continue
		}
		count++
	}
	return count
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

	hasEvidence := len(taskResult.Evidence) > 0
	hasArtifacts := len(taskResult.ArtifactRefs) > 0

	return hasBeliefGraph && hasFSMHistory && (hasEvidence || hasArtifacts)
}

// LoadCases loads eval cases from a JSON file. Exported for use by external baselines.
func LoadCases(filePath string) ([]EvalCase, error) {
	return loadCases(filePath)
}

func loadCases(filePath string) ([]EvalCase, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}

	return cases, nil
}

func CheckGate(metrics *EvalMetrics, baseline *EvalMetrics) *GateReport {
	report := &GateReport{
		AllPassed: true,
		Gates:     make([]GateResult, 0),
	}

	report.Gates = append(report.Gates, GateResult{
		Name:     "accuracy",
		Passed:   metrics.Accuracy >= baseline.Accuracy,
		Expected: fmt.Sprintf(">= %.2f%%", baseline.Accuracy*100),
		Actual:   fmt.Sprintf("%.2f%%", metrics.Accuracy*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "evidence_coverage",
		Passed:   metrics.EvidenceCoverage >= baseline.EvidenceCoverage,
		Expected: fmt.Sprintf(">= %.2f%%", baseline.EvidenceCoverage*100),
		Actual:   fmt.Sprintf("%.2f%%", metrics.EvidenceCoverage*100),
	})

	report.Gates = append(report.Gates, GateResult{
		Name:     "latency",
		Passed:   metrics.AvgLatency <= baseline.AvgLatency*3/2,
		Expected: fmt.Sprintf("<= %v", baseline.AvgLatency*3/2),
		Actual:   fmt.Sprintf("%v", metrics.AvgLatency),
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
