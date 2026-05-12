package eval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/protocol"
)

type testLogger struct{}

func (l *testLogger) Info(msg string, keysAndValues ...interface{})  {}
func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {}

func TestRunner_RunFromCases(t *testing.T) {
	cfg := gos_engine.DefaultConfig()
	cfg.SessionMaxSteps = 2
	cfg.FSM.GapDelta = 0.1
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 2

	logger := &testLogger{}
	engine := gos_engine.NewGoSEngine(cfg, logger)

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
		EvidenceCount: 3,
		TraceComplete: true,
	})

	metrics.AddResult(&EvalResult{
		Status:        string(protocol.ResultStatusDegraded),
		EvidenceCount: 1,
		TraceComplete: false,
	})

	assert.Equal(t, 2, metrics.TotalCases)
	assert.Equal(t, 1, metrics.Succeeded)
	assert.Equal(t, 1, metrics.Degraded)
	assert.Equal(t, float64(2), metrics.EvidenceCoverage)
	assert.Equal(t, float64(1), metrics.Traceability)
}

func TestEvalMetrics_Finalize(t *testing.T) {
	metrics := NewEvalMetrics()

	metrics.AddResult(&EvalResult{
		Status:        string(protocol.ResultStatusSucceeded),
		EvidenceCount: 3,
		TraceComplete: true,
	})

	metrics.AddResult(&EvalResult{
		Status:        string(protocol.ResultStatusDegraded),
		EvidenceCount: 1,
		TraceComplete: false,
	})

	metrics.Finalize()

	assert.Equal(t, 0.5, metrics.Accuracy)
	assert.Equal(t, 1.0, metrics.EvidenceCoverage)
	assert.Equal(t, 0.5, metrics.DegradationRate)
	assert.Equal(t, 0.5, metrics.Traceability)
}
