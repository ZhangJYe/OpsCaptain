package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/protocol"
)

func TestRecordedEvidenceMetricsUseDeduplicatedSourceSignals(t *testing.T) {
	document := "# Telemetry Evidence Case\n\n## Metric Signals\n\n- pod_cpu_usage [service-a]: score=9\n\n## Log Signals\n\n- service-a: timeout while dialing\n\n## Trace Signals\n\n- no trace signal extracted\n"
	wrapper, err := json.Marshal(map[string]any{
		"success": true, "provenance_profile": "recorded_blind", "data": document,
	})
	require.NoError(t, err)
	repeated, err := json.Marshal(map[string]any{
		"success": true, "provenance_profile": "recorded_blind",
		"data": "recorded snapshot already delivered for this case",
	})
	require.NoError(t, err)

	evidence := []protocol.EvidenceItem{
		{SourceType: "tool", SourceID: "tool-a", Snippet: string(wrapper)},
		{SourceType: "rag", SourceID: "rag-a", Snippet: document},
		{SourceType: "tool", SourceID: "tool-repeat", Snippet: string(repeated)},
	}
	count, relevant, expected, covered := EvaluateEvidenceMetrics(evidence, []string{"pod_cpu_usage", "timeout"})

	require.Equal(t, 2, count)
	require.Equal(t, 2, relevant)
	require.Equal(t, 2, expected)
	require.Equal(t, 2, covered)
}

type testLogger struct{}

func (l *testLogger) Info(msg string, keysAndValues ...interface{})  {}
func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {}

type llmCountExpert struct{}

func (e *llmCountExpert) Name() string {
	return "llm_count_expert"
}

func (e *llmCountExpert) Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *experts.ExpertAnalysis {
	return &experts.ExpertAnalysis{
		ExpertName: "llm_count_expert",
		Analysis:   "服务响应超时",
		Confidence: 0.9,
		Status:     "succeeded",
		LLMCalls:   2,
		Evidence: []experts.EvidenceItem{
			{
				SourceType:         "test",
				SourceID:           "ev-1",
				Title:              "test evidence",
				Snippet:            "服务响应超时",
				Score:              1,
				Relation:           experts.EvidenceRelationSupport,
				TargetHypothesisID: frontier.NodeID,
				Strength:           1,
			},
		},
	}
}

func TestRunner_RunFromCases(t *testing.T) {
	cfg := gos_engine.DefaultConfig()
	cfg.SessionMaxSteps = 2
	cfg.FSM.GapDelta = 0.1
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 2

	logger := &testLogger{}
	engine := gos_engine.NewGoSEngine(cfg, logger)
	engine.RegisterExpert("llm_count_expert", &llmCountExpert{})

	runner := NewRunner(engine)

	cases := []EvalCase{
		{
			ID:          "case-1",
			Symptom:     "服务响应超时",
			GroundTruth: "服务响应超时",
		},
		{
			ID:          "case-2",
			Symptom:     "数据库连接失败",
			GroundTruth: "数据库连接失败",
		},
	}

	metrics, results, err := runner.RunFromCases(context.Background(), cases)
	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Len(t, results, 2)
	assert.Equal(t, 2, metrics.TotalCases)
}

func TestRunner_RunFromCases_NonZeroLLMCalls(t *testing.T) {
	cfg := gos_engine.DefaultConfig()
	cfg.SessionMaxSteps = 2
	cfg.FSM.GapDelta = 0.1
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 2

	logger := &testLogger{}
	engine := gos_engine.NewGoSEngine(cfg, logger)
	engine.RegisterExpert("llm_count_expert", &llmCountExpert{})

	runner := NewRunner(engine)

	cases := []EvalCase{
		{
			ID:          "case-1",
			Symptom:     "服务响应超时",
			GroundTruth: "服务响应超时",
		},
	}

	metrics, results, err := runner.RunFromCases(context.Background(), cases)
	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Len(t, results, 1)

	assert.Greater(t, results[0].LLMCalls, 0, "LLMCalls should be non-zero")
	assert.Greater(t, metrics.AvgLLMCalls, float64(0), "AvgLLMCalls should be non-zero")
}

func TestMatchPrediction(t *testing.T) {
	tests := []struct {
		name             string
		prediction       string
		groundTruth      string
		expectedKeywords []string
		expected         bool
	}{
		{
			name:        "exact match",
			prediction:  "CPU 资源耗尽导致服务超时",
			groundTruth: "CPU 资源耗尽导致服务超时",
			expected:    true,
		},
		{
			name:        "partial match",
			prediction:  "CPU 使用率过高导致服务响应超时",
			groundTruth: "CPU 资源耗尽导致服务超时",
			expected:    true,
		},
		{
			name:        "keyword match",
			prediction:  "服务器 CPU 负载过高，导致服务超时",
			groundTruth: "CPU 资源耗尽",
			expected:    true,
		},
		{
			name:        "no match",
			prediction:  "网络延迟升高",
			groundTruth: "CPU 资源耗尽",
			expected:    false,
		},
		{
			name:        "degraded prediction",
			prediction:  "[DEGRADED] 信息不足",
			groundTruth: "CPU 资源耗尽",
			expected:    false,
		},
		{
			name:             "with expected keywords match",
			prediction:       "数据库连接池耗尽导致服务超时",
			groundTruth:      "数据库连接问题",
			expectedKeywords: []string{"数据库", "连接池"},
			expected:         true,
		},
		{
			name:             "with expected keywords no match",
			prediction:       "网络延迟升高",
			groundTruth:      "数据库连接问题",
			expectedKeywords: []string{"数据库", "连接池"},
			expected:         false,
		},
		{
			name:             "with expected keywords partial",
			prediction:       "数据库负载过高",
			groundTruth:      "数据库连接问题",
			expectedKeywords: []string{"数据库", "连接池"},
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchPrediction(tt.prediction, tt.groundTruth, tt.expectedKeywords)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckGate(t *testing.T) {
	baseline := &EvalMetrics{
		Accuracy:          0.8,
		RootCauseAccuracy: 0.8,
		EvidencePrecision: 0.8,
		EvidenceCoverage:  0.9,
		BacktrackSuccess:  1,
		GraphValidity:     1,
		AvgLatency:        5 * time.Second,
		P95Latency:        5 * time.Second,
		AvgLLMCalls:       3.0,
		DegradationRate:   0.1,
		Traceability:      1.0,
	}

	gosMetrics := &EvalMetrics{
		Accuracy:          0.85,
		RootCauseAccuracy: 0.85,
		EvidencePrecision: 0.85,
		EvidenceCoverage:  0.95,
		BacktrackSuccess:  1,
		GraphValidity:     1,
		AvgLatency:        4 * time.Second,
		P95Latency:        4 * time.Second,
		AvgLLMCalls:       2.5,
		DegradationRate:   0.05,
		Traceability:      1.0,
	}

	report := CheckGate(gosMetrics, baseline)
	assert.True(t, report.AllPassed)
	assert.Len(t, report.Gates, 11)

	for _, g := range report.Gates {
		assert.True(t, g.Passed, "Gate %s should pass", g.Name)
	}
}

func TestCheckGate_Failure(t *testing.T) {
	baseline := &EvalMetrics{
		Accuracy:          0.8,
		RootCauseAccuracy: 0.8,
		BacktrackSuccess:  1,
		GraphValidity:     1,
		EvidenceCoverage:  0.9,
		AvgLatency:        5 * time.Second,
		AvgLLMCalls:       3.0,
		DegradationRate:   0.1,
		Traceability:      1.0,
	}

	gosMetrics := &EvalMetrics{
		Accuracy:          0.7,
		RootCauseAccuracy: 0.7,
		BacktrackSuccess:  1,
		GraphValidity:     1,
		EvidenceCoverage:  0.95,
		AvgLatency:        4 * time.Second,
		AvgLLMCalls:       2.5,
		DegradationRate:   0.05,
		Traceability:      1.0,
	}

	report := CheckGate(gosMetrics, baseline)
	assert.False(t, report.AllPassed)

	accuracyGate := report.Gates[0]
	assert.Equal(t, "root_cause_accuracy", accuracyGate.Name)
	assert.False(t, accuracyGate.Passed)
}

func TestEvalMetrics_AddResult(t *testing.T) {
	metrics := NewEvalMetrics()

	metrics.AddResult(&EvalResult{
		Status:        string(protocol.ResultStatusSucceeded),
		Matched:       true,
		Latency:       2 * time.Second,
		LLMCalls:      3,
		EvidenceCount: 3,
		TraceComplete: true,
	})

	metrics.AddResult(&EvalResult{
		Status:        string(protocol.ResultStatusDegraded),
		Matched:       false,
		Latency:       1 * time.Second,
		LLMCalls:      2,
		EvidenceCount: 1,
		TraceComplete: false,
	})

	assert.Equal(t, 2, metrics.TotalCases)
	assert.Equal(t, 1, metrics.Succeeded)
	assert.Equal(t, 1, metrics.Degraded)
	assert.Equal(t, 1, metrics.Matched)
	assert.Equal(t, 2, metrics.evidencePresent)
	assert.Equal(t, 1, metrics.traceComplete)
	assert.Equal(t, 3*time.Second, metrics.totalLatency)
	assert.Equal(t, 5, metrics.totalLLMCalls)
}

func TestEvalMetrics_Finalize(t *testing.T) {
	metrics := NewEvalMetrics()

	metrics.AddResult(&EvalResult{
		Status:        string(protocol.ResultStatusSucceeded),
		Matched:       true,
		Latency:       2 * time.Second,
		LLMCalls:      3,
		EvidenceCount: 3,
		TraceComplete: true,
	})

	metrics.AddResult(&EvalResult{
		Status:        string(protocol.ResultStatusDegraded),
		Matched:       false,
		Latency:       1 * time.Second,
		LLMCalls:      2,
		EvidenceCount: 1,
		TraceComplete: false,
	})

	metrics.Finalize()

	assert.Equal(t, 0.5, metrics.Accuracy)
	assert.Equal(t, 1.0, metrics.EvidenceCoverage)
	assert.Equal(t, 0.5, metrics.DegradationRate)
	assert.Equal(t, 0.5, metrics.Traceability)
	assert.Equal(t, 1500*time.Millisecond, metrics.AvgLatency)
	assert.Equal(t, 2.5, metrics.AvgLLMCalls)
}

func TestEvalMetrics_FinalizePhaseZeroMetrics(t *testing.T) {
	metrics := NewEvalMetrics()
	metrics.AddResult(&EvalResult{
		Status:            string(protocol.ResultStatusSucceeded),
		Matched:           true,
		Latency:           time.Second,
		LLMCalls:          2,
		ToolCalls:         3,
		RAGCalls:          1,
		EvidenceCount:     4,
		RelevantEvidence:  3,
		ExpectedEvidence:  3,
		CoveredEvidence:   2,
		TraceComplete:     true,
		GraphValid:        true,
		ContractMatched:   true,
		BacktrackRequired: true,
		Backtracked:       true,
	})
	metrics.AddResult(&EvalResult{
		Status:            string(protocol.ResultStatusDegraded),
		Latency:           3 * time.Second,
		LLMCalls:          4,
		ToolCalls:         1,
		RAGCalls:          3,
		EvidenceCount:     2,
		RelevantEvidence:  1,
		ExpectedEvidence:  1,
		CoveredEvidence:   1,
		BacktrackRequired: true,
		PrematureStop:     true,
		FailurePhase:      "act",
	})

	metrics.Finalize()

	assert.Equal(t, 0.5, metrics.RootCauseAccuracy)
	assert.InDelta(t, 4.0/6.0, metrics.EvidencePrecision, 0.0001)
	assert.Equal(t, 0.75, metrics.EvidenceCoverage)
	assert.Equal(t, 0.5, metrics.BacktrackSuccess)
	assert.Equal(t, 0.5, metrics.PrematureStopRate)
	assert.Equal(t, 0.5, metrics.GraphValidity)
	assert.Equal(t, 0.5, metrics.ContractCompliance)
	assert.Equal(t, 2*time.Second, metrics.AvgLatency)
	assert.Equal(t, time.Second, metrics.P50Latency)
	assert.Equal(t, 3*time.Second, metrics.P95Latency)
	assert.Equal(t, 3.0, metrics.AvgLLMCalls)
	assert.Equal(t, 2.0, metrics.AvgToolCalls)
	assert.Equal(t, 2.0, metrics.AvgRAGCalls)
	assert.Equal(t, map[string]int{"act": 1}, metrics.FailuresByPhase)
}

func TestIsPrematureStopDoesNotDuplicateRootCauseAccuracy(t *testing.T) {
	assert.False(t, IsPrematureStop(
		string(protocol.ResultStatusSucceeded),
		true,
		false,
		false,
		false,
		false,
	))
	assert.True(t, IsPrematureStop(
		string(protocol.ResultStatusSucceeded),
		true,
		true,
		false,
		false,
		false,
	))
	assert.True(t, IsPrematureStop(
		string(protocol.ResultStatusSucceeded),
		false,
		false,
		false,
		false,
		false,
	))
	assert.False(t, IsPrematureStop(
		string(protocol.ResultStatusDegraded),
		false,
		true,
		false,
		true,
		false,
	))
}

type scriptedEngine struct {
	result *protocol.TaskResult
}

func (e scriptedEngine) Run(context.Context, string) *protocol.TaskResult {
	return e.result
}

func TestRunnerCapturesTransitionsEvidenceAndCalls(t *testing.T) {
	runner := NewRunner(scriptedEngine{result: &protocol.TaskResult{
		Status:  protocol.ResultStatusSucceeded,
		Summary: "数据库连接池耗尽",
		Evidence: []protocol.EvidenceItem{
			{SourceType: "graph", Title: "input"},
			{SourceType: "tool", Title: "pool metrics", Snippet: "connection pool waiters increased"},
			{SourceType: "rag", Title: "runbook", Snippet: "max_connections exhausted"},
		},
		Metadata: map[string]any{
			"belief_graph": map[string]any{},
			"fsm_history": []belief.FSMTransition{
				{FromLevel: 0, ToLevel: 1},
				{FromLevel: 1, ToLevel: 0},
			},
			"graph_valid": true,
			"llm_calls":   4,
			"tool_calls":  2,
			"rag_calls":   1,
		},
	}})

	metrics, results, err := runner.RunFromCases(context.Background(), []EvalCase{{
		ID:                       "case",
		Scenario:                 "backtracking_required",
		Symptom:                  "连接池等待升高",
		GroundTruth:              "数据库连接池耗尽",
		ExpectedKeywords:         []string{"数据库", "连接池"},
		ExpectedEvidenceKeywords: []string{"pool", "max_connections"},
		RequireRefine:            true,
		RequireBacktrack:         true,
	}})
	require.NoError(t, err)
	require.Len(t, results, 1)

	result := results[0]
	assert.True(t, result.Matched)
	assert.Equal(t, 2, result.EvidenceCount)
	assert.Equal(t, 2, result.RelevantEvidence)
	assert.Equal(t, 2, result.CoveredEvidence)
	assert.Equal(t, 4, result.LLMCalls)
	assert.Equal(t, 2, result.ToolCalls)
	assert.Equal(t, 1, result.RAGCalls)
	assert.True(t, result.Refined)
	assert.True(t, result.Backtracked)
	assert.True(t, result.StatusMatched)
	assert.True(t, result.FailurePhaseMatched)
	assert.True(t, result.ContractMatched)
	assert.False(t, result.PrematureStop)
	assert.Empty(t, result.FailurePhase)
	assert.Equal(t, 1.0, metrics.BacktrackSuccess)
}

func TestRunnerWrongDiagnosisIsReportFailureNotPrematureStop(t *testing.T) {
	runner := NewRunner(scriptedEngine{result: &protocol.TaskResult{
		Status:  protocol.ResultStatusSucceeded,
		Summary: "网络链路抖动",
		Evidence: []protocol.EvidenceItem{
			{SourceType: "tool", SourceID: "metric-1", Snippet: "latency increased"},
		},
		Metadata: map[string]any{
			"belief_graph": map[string]any{},
			"fsm_history":  []belief.FSMTransition{},
			"graph_valid":  true,
		},
	}})

	_, results, err := runner.RunFromCases(context.Background(), []EvalCase{{
		ID:               "wrong-report",
		Symptom:          "数据库连接等待升高",
		GroundTruth:      "数据库连接池耗尽",
		ExpectedKeywords: []string{"数据库", "连接池"},
		ExpectedStatus:   string(protocol.ResultStatusSucceeded),
	}})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].Matched)
	assert.False(t, results[0].PrematureStop)
	assert.Equal(t, "report", results[0].FailurePhase)
}

func TestFailurePhaseAttribution(t *testing.T) {
	tests := []struct {
		name       string
		result     *protocol.TaskResult
		evalCase   EvalCase
		matched    bool
		evidence   int
		graphValid bool
		refined    bool
		backtrack  bool
		expected   string
	}{
		{name: "explicit act", result: &protocol.TaskResult{Metadata: map[string]any{"error_phase": "act_failed"}}, expected: "act"},
		{name: "invalid graph", result: &protocol.TaskResult{}, matched: true, expected: "update"},
		{name: "missing state transition", result: &protocol.TaskResult{}, evalCase: EvalCase{RequireBacktrack: true}, matched: true, evidence: 1, graphValid: true, expected: "state"},
		{name: "wrong report", result: &protocol.TaskResult{}, evidence: 1, graphValid: true, expected: "report"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, failurePhase(tt.result, tt.matched, tt.evidence, tt.graphValid, tt.evalCase, tt.refined, tt.backtrack))
		})
	}
}

func TestRunnerChecksExpectedStatusAndFailurePhase(t *testing.T) {
	runner := NewRunner(scriptedEngine{result: &protocol.TaskResult{
		Status:  protocol.ResultStatusDegraded,
		Summary: "tool unavailable",
		Metadata: map[string]any{
			"belief_graph": map[string]any{},
			"fsm_history":  []belief.FSMTransition{},
			"graph_valid":  true,
			"error_phase":  "act_failed",
		},
	}})

	_, results, err := runner.RunFromCases(context.Background(), []EvalCase{{
		ID:                   "tool-timeout",
		Symptom:              "tool timeout",
		GroundTruth:          "tool unavailable",
		ExpectedStatus:       "degraded",
		ExpectedFailurePhase: "act",
	}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].StatusMatched)
	assert.True(t, results[0].FailurePhaseMatched)
	assert.True(t, results[0].ContractMatched)

	_, results, err = runner.RunFromCases(context.Background(), []EvalCase{{
		ID:                   "wrong-phase",
		Symptom:              "tool timeout",
		GroundTruth:          "tool unavailable",
		ExpectedStatus:       "degraded",
		ExpectedFailurePhase: "update",
	}})
	require.NoError(t, err)
	assert.False(t, results[0].FailurePhaseMatched)
	assert.False(t, results[0].ContractMatched)
}

func TestCheckTraceComplete(t *testing.T) {
	taskResult := &protocol.TaskResult{
		Status: protocol.ResultStatusSucceeded,
		Metadata: map[string]any{
			"belief_graph": map[string]interface{}{},
			"fsm_history":  []interface{}{},
		},
		Evidence: []protocol.EvidenceItem{
			{SourceType: "test", Title: "test"},
		},
	}
	assert.True(t, checkTraceComplete(taskResult))

	taskResult2 := &protocol.TaskResult{
		Status:   protocol.ResultStatusSucceeded,
		Metadata: map[string]any{},
	}
	assert.False(t, checkTraceComplete(taskResult2))

	taskResult3 := &protocol.TaskResult{
		Status: protocol.ResultStatusSucceeded,
		Metadata: map[string]any{
			"belief_graph": map[string]interface{}{},
		},
	}
	assert.False(t, checkTraceComplete(taskResult3))
}

func TestCountDiagnosticEvidenceSkipsInputGraphEvidence(t *testing.T) {
	evidence := []protocol.EvidenceItem{
		{SourceType: "graph", Title: "user symptom"},
		{SourceType: "tool", Title: "query_logs"},
		{SourceType: "rag", Title: "runbook"},
		{Title: "unknown"},
	}

	assert.Equal(t, 2, countDiagnosticEvidence(evidence))
}

func TestDiagnosticEvidenceItemsKeepsPrometheusAlertToolOutput(t *testing.T) {
	evidence := []protocol.EvidenceItem{{
		SourceType: "tool",
		SourceID:   "network_sre:query_prometheus_alerts:abc",
		Title:      "query_prometheus_alerts output",
		Snippet:    `{"success":true,"alerts":[{"alert_name":"IsolatedEvalServiceDegraded","labels":{"service":"eval-retry-inventory"}}]}`,
	}}

	got := diagnosticEvidenceItems(evidence)
	require.Len(t, got, 1)
	assert.Equal(t, evidence[0].SourceID, got[0].SourceID)
}

func TestCaseRunnerBuildsAnIsolatedEnginePerCase(t *testing.T) {
	built := make([]string, 0, 2)
	runner := NewCaseRunner(func(caseID string) (EngineRunner, error) {
		built = append(built, caseID)
		return scriptedEngine{result: &protocol.TaskResult{
			Status:  protocol.ResultStatusSucceeded,
			Summary: caseID,
			Metadata: map[string]any{
				"belief_graph": map[string]any{},
				"fsm_history":  []any{},
				"graph_valid":  true,
			},
			Evidence: []protocol.EvidenceItem{{SourceType: "recorded", Snippet: caseID}},
		}}, nil
	})

	metrics, results, err := runner.RunFromCases(context.Background(), []EvalCase{
		{ID: "case-a", Domain: "service", Scenario: "recorded", Symptom: "a", GroundTruth: "case-a"},
		{ID: "case-b", Domain: "service", Scenario: "recorded", Symptom: "b", GroundTruth: "case-b"},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"case-a", "case-b"}, built)
	require.Equal(t, 2, metrics.TotalCases)
	require.Equal(t, "case-a", results[0].Prediction)
	require.Equal(t, "case-b", results[1].Prediction)
}

func TestCaseRunnerStopsWhenCaseEngineCannotBeBuilt(t *testing.T) {
	runner := NewCaseRunner(func(caseID string) (EngineRunner, error) {
		return nil, fmt.Errorf("missing evidence for %s", caseID)
	})

	metrics, results, err := runner.RunFromCases(context.Background(), []EvalCase{
		{ID: "case-a", Domain: "service", Scenario: "recorded", Symptom: "a", GroundTruth: "root"},
	})

	require.ErrorContains(t, err, "case-a")
	require.Nil(t, metrics)
	require.Nil(t, results)
}
