package eval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/protocol"
)

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
			{SourceType: "test", SourceID: "ev-1", Title: "test evidence", Snippet: "服务响应超时", Score: 1},
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
		Accuracy:         0.8,
		EvidenceCoverage: 0.9,
		AvgLatency:       5 * time.Second,
		AvgLLMCalls:      3.0,
		DegradationRate:  0.1,
		Traceability:     1.0,
	}

	gosMetrics := &EvalMetrics{
		Accuracy:         0.85,
		EvidenceCoverage: 0.95,
		AvgLatency:       4 * time.Second,
		AvgLLMCalls:      2.5,
		DegradationRate:  0.05,
		Traceability:     1.0,
	}

	report := CheckGate(gosMetrics, baseline)
	assert.True(t, report.AllPassed)
	assert.Len(t, report.Gates, 6)

	for _, g := range report.Gates {
		assert.True(t, g.Passed, "Gate %s should pass", g.Name)
	}
}

func TestCheckGate_Failure(t *testing.T) {
	baseline := &EvalMetrics{
		Accuracy:         0.8,
		EvidenceCoverage: 0.9,
		AvgLatency:       5 * time.Second,
		AvgLLMCalls:      3.0,
		DegradationRate:  0.1,
		Traceability:     1.0,
	}

	gosMetrics := &EvalMetrics{
		Accuracy:         0.7,
		EvidenceCoverage: 0.95,
		AvgLatency:       4 * time.Second,
		AvgLLMCalls:      2.5,
		DegradationRate:  0.05,
		Traceability:     1.0,
	}

	report := CheckGate(gosMetrics, baseline)
	assert.False(t, report.AllPassed)

	accuracyGate := report.Gates[0]
	assert.Equal(t, "accuracy", accuracyGate.Name)
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
