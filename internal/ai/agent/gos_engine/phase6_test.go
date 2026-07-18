package gos_engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/protocol"
)

type panickingExpert struct{}

func (p *panickingExpert) Name() string { return "linux_sre" }

func (p *panickingExpert) Run(context.Context, *belief.Frontier, *belief.BeliefGraph) *experts.ExpertAnalysis {
	panic("fault injection")
}

type cancellationTrackingExpert struct {
	started  chan struct{}
	finished chan struct{}
}

func (e *cancellationTrackingExpert) Name() string { return "linux_sre" }

func (e *cancellationTrackingExpert) Run(ctx context.Context, _ *belief.Frontier, _ *belief.BeliefGraph) *experts.ExpertAnalysis {
	close(e.started)
	defer close(e.finished)
	<-ctx.Done()
	return &experts.ExpertAnalysis{
		ExpertName:        e.Name(),
		Status:            "degraded",
		DegradationReason: "context_cancelled",
	}
}

func phase6EngineConfig() *Config {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 1
	cfg.FSM.MaxSteps = 1
	cfg.FSM.GapDelta = 0.1
	cfg.FSM.MinConfidence = 0.1
	cfg.FSM.MinSupport = 1
	return cfg
}

func TestGoSEngineResourceLimitReturnsDegraded(t *testing.T) {
	cfg := phase6EngineConfig()
	cfg.Graph.MaxNodes = 2
	engine := NewGoSEngine(cfg, &testLogger{})

	result := engine.Run(context.Background(), "服务响应超时")

	require.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Equal(t, "graph_resource_limit", result.Metadata["error_phase"])
	assert.Contains(t, result.DegradationReason, "nodes limit exceeded")
	stats := result.Metadata["graph_resource_stats"].(belief.GraphResourceStats)
	assert.Zero(t, stats.Nodes)
}

func TestGoSEngineInvalidGraphConfigReturnsDegraded(t *testing.T) {
	cfg := phase6EngineConfig()
	cfg.Graph.MaxEdges = 0
	engine := NewGoSEngine(cfg, &testLogger{})

	result := engine.Run(context.Background(), "服务响应超时")

	require.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Equal(t, "graph_config_invalid", result.Metadata["error_phase"])
	assert.Contains(t, result.DegradationReason, "max_edges must be positive")
}

func TestGoSEngineResultIncludesTraceMetricsAndVersions(t *testing.T) {
	cfg := phase6EngineConfig()
	engine := NewGoSEngine(cfg, &testLogger{})
	var tracePayload map[string]any
	engine.SetEmitter(func(_ context.Context, _ string, payload map[string]any) {
		if payload["stage"] == "observability" {
			tracePayload = payload
		}
	})
	engine.RegisterExpert("linux_sre", &mockExpert{name: "linux_sre", response: &experts.ExpertAnalysis{
		ExpertName: "linux_sre",
		Status:     "succeeded",
		Evidence: []experts.EvidenceItem{{
			SourceType: "metric", SourceID: "cpu-usage", Title: "CPU high",
			Relation: experts.EvidenceRelationSupport, Strength: 0.9,
		}},
	}})

	result := engine.Run(context.Background(), "CPU 使用率持续升高")

	require.NotNil(t, result.Metadata["observability"])
	observability := result.Metadata["observability"].(map[string]any)
	phaseLatency := observability["phase_latency_ms"].(map[string]int64)
	assert.Contains(t, phaseLatency, "ingest")
	assert.Contains(t, phaseLatency, "plan")
	assert.Contains(t, phaseLatency, "act")
	assert.Contains(t, phaseLatency, "update")
	assert.Contains(t, phaseLatency, "report")
	assert.Equal(t, 1, observability["new_evidence_count"])
	assert.NotNil(t, observability["graph"])
	versions := result.Metadata["versions"].(RuntimeVersions)
	assert.Equal(t, cfg.ModelPath, versions.ModelPath)
	assert.NotEmpty(t, versions.Model)
	assert.Contains(t, versions.Prompt, "sha256:")
	assert.Contains(t, versions.Config, "sha256:")
	storage := result.Metadata["storage_boundary"].(map[string]string)
	assert.Equal(t, "redis_ledger_required", storage["multi_instance"])
	require.NotNil(t, tracePayload)
	assert.NotNil(t, tracePayload["phase_latency_ms"])
	assert.NotNil(t, tracePayload["versions"])
}

func TestGoSEngineRuntimeVersionsAreStableAndConfigSensitive(t *testing.T) {
	cfg := phase6EngineConfig()
	engine := NewGoSEngine(cfg, &testLogger{})

	first := engine.runtimeVersions()
	second := engine.runtimeVersions()
	require.Equal(t, first, second)

	cfg.Graph.MaxNodes++
	changed := engine.runtimeVersions()
	assert.Equal(t, first.Prompt, changed.Prompt)
	assert.Equal(t, first.ModelPath, changed.ModelPath)
	assert.Equal(t, first.Model, changed.Model)
	assert.NotEqual(t, first.Config, changed.Config)
}

func TestGoSEngineRecoversExpertPanicAsDegraded(t *testing.T) {
	engine := NewGoSEngine(phase6EngineConfig(), &testLogger{})
	engine.RegisterExpert("linux_sre", &panickingExpert{})

	result := engine.Run(context.Background(), "服务响应超时")

	require.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Contains(t, result.DegradationReason, "act_failed")
	assert.Equal(t, 1, result.Metadata["expert_failed_count"])
}

func TestGoSEngineRecoversStructuredGeneratorPanic(t *testing.T) {
	cfg := phase6EngineConfig()
	cfg.StructuredCognition.Enabled = true
	cfg.StructuredGenerate = func(context.Context, string) (string, error) {
		panic("structured fault injection")
	}
	engine := NewGoSEngine(cfg, &testLogger{})

	result := engine.Run(context.Background(), "服务响应超时")

	require.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Equal(t, "panic_recovered", result.Metadata["error_phase"])
	assert.Contains(t, result.DegradationReason, "panic_recovered")
}

func TestGoSEngineCancellationWaitsForExpertExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	expert := &cancellationTrackingExpert{started: make(chan struct{}), finished: make(chan struct{})}
	engine := NewGoSEngine(phase6EngineConfig(), &testLogger{})
	engine.RegisterExpert("linux_sre", expert)
	resultCh := make(chan *protocol.TaskResult, 1)
	go func() {
		resultCh <- engine.Run(ctx, "服务响应超时")
	}()

	select {
	case <-expert.started:
	case <-time.After(time.Second):
		t.Fatal("expert did not start")
	}
	cancel()

	select {
	case result := <-resultCh:
		require.Equal(t, protocol.ResultStatusDegraded, result.Status)
	case <-time.After(time.Second):
		t.Fatal("engine did not stop after cancellation")
	}
	select {
	case <-expert.finished:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expert goroutine remained after engine returned")
	}
}
