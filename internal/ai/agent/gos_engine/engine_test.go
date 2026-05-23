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

type testLogger struct{}

func (l *testLogger) Info(msg string, keysAndValues ...interface{})  {}
func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {}

type mockExpert struct {
	name     string
	response *experts.ExpertAnalysis
	delay    time.Duration
}

func (m *mockExpert) Name() string {
	return m.name
}

func (m *mockExpert) Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *experts.ExpertAnalysis {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return m.response
}

func TestGoSEngine_Run_ExpertSuccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 2
	cfg.FSM.GapDelta = 0.1
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 2

	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	engine.RegisterExpert("linux_sre", &mockExpert{
		name: "linux_sre",
		response: &experts.ExpertAnalysis{
			ExpertName: "linux_sre",
			Analysis:   "CPU 高负载",
			Confidence: 0.9,
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					SourceType: "metric",
					SourceID:   "cpu-1",
					Title:      "CPU 使用率 95%",
					Snippet:    "CPU usage 95%",
					Score:      1.0,
				},
			},
		},
	})

	result := engine.Run(context.Background(), "服务响应超时")
	require.NotNil(t, result)
	assert.Equal(t, "gos_engine", result.Agent)
}

func TestGoSEngine_Run_AllExpertsFail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 2

	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	engine.RegisterExpert("linux_sre", &mockExpert{
		name: "linux_sre",
		response: &experts.ExpertAnalysis{
			ExpertName:        "linux_sre",
			Status:            "failed",
			DegradationReason: "tool_error",
		},
	})

	result := engine.Run(context.Background(), "服务响应超时")
	require.NotNil(t, result)
	assert.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.NotEmpty(t, result.DegradationReason)
}

func TestGoSEngine_Run_ContextCancelStopsHangingExpert(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 1

	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	engine.RegisterExpert("linux_sre", &mockExpert{
		name:  "linux_sre",
		delay: 200 * time.Millisecond,
		response: &experts.ExpertAnalysis{
			ExpertName: "linux_sre",
			Status:     "succeeded",
			Analysis:   "late result",
			Confidence: 0.9,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := engine.Run(ctx, "cpu high")

	assert.Less(t, time.Since(start), 150*time.Millisecond)
	require.NotNil(t, result)
	assert.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.NotEmpty(t, result.DegradationReason)
}

func TestGoSEngine_Run_NoExperts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 2

	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	result := engine.Run(context.Background(), "服务响应超时")
	require.NotNil(t, result)
	assert.Equal(t, protocol.ResultStatusDegraded, result.Status)
}

func TestGoSEngine_ShouldReport(t *testing.T) {
	cfg := DefaultConfig()
	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	frontier := &belief.Frontier{
		Score:    0.9,
		Supports: 3,
	}
	assert.True(t, engine.shouldReport(frontier))

	frontier2 := &belief.Frontier{
		Score:    0.5,
		Supports: 1,
	}
	assert.False(t, engine.shouldReport(frontier2))
}

func TestGoSEngine_UpdateGraph(t *testing.T) {
	cfg := DefaultConfig()
	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	graph := belief.NewBeliefGraph()
	hypoID := graph.AddHypothesis("Test", 0.5, 1, "Initial")
	frontier := &belief.Frontier{
		NodeID: hypoID,
	}

	analyses := []*experts.ExpertAnalysis{
		{
			ExpertName: "test",
			Analysis:   "Updated analysis",
			Confidence: 0.9,
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					SourceType: "test",
					SourceID:   "ev-1",
					Title:      "Test evidence",
					Snippet:    "Test snippet",
					Score:      1.0,
				},
			},
		},
	}

	result := engine.updateGraph(context.Background(), graph, analyses, frontier)
	assert.True(t, result.Committed)
	assert.Len(t, graph.Nodes, 2)
}

func TestGoSEngine_Run_TwiceDoesNotReuseState(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 2
	cfg.FSM.GapDelta = 0.1
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 2

	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	engine.RegisterExpert("linux_sre", &mockExpert{
		name: "linux_sre",
		response: &experts.ExpertAnalysis{
			ExpertName: "linux_sre",
			Analysis:   "CPU 高负载",
			Confidence: 0.9,
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					SourceType: "metric",
					SourceID:   "cpu-1",
					Title:      "CPU 使用率 95%",
					Snippet:    "CPU usage 95%",
					Score:      1.0,
				},
			},
		},
	})

	result1 := engine.Run(context.Background(), "第一次请求-服务A超时")
	require.NotNil(t, result1)

	result2 := engine.Run(context.Background(), "第二次请求-数据库连接失败")
	require.NotNil(t, result2)

	assert.NotEqual(t, result1.TaskID, result2.TaskID)

	if result1.Metadata != nil && result2.Metadata != nil {
		graph2, ok2 := result2.Metadata["belief_graph"].(map[string]interface{})
		if ok2 {
			belief2, _ := graph2["belief"].(string)
			assert.NotContains(t, belief2, "服务A超时")
			assert.NotContains(t, belief2, "第一次请求")
		}
	}
}

func TestIngestor_Ingest(t *testing.T) {
	graph := belief.NewBeliefGraph()
	logger := &testLogger{}
	ingestor := NewIngestor(graph, logger)

	err := ingestor.Ingest(context.Background(), "CPU 使用率过高")
	require.NoError(t, err)

	assert.NotEmpty(t, graph.StartSignalID)
	assert.True(t, len(graph.Nodes) > 0)
}

func TestPlanner_Plan(t *testing.T) {
	expertsMap := map[string]experts.ExpertAgent{
		"linux_sre": &mockExpert{name: "linux_sre"},
	}
	cfg := DefaultConfig()
	logger := &testLogger{}
	planner := NewPlanner(expertsMap, cfg, logger)

	frontier := &belief.Frontier{
		NodeID: "test",
		Label:  "Test hypothesis",
	}

	plan, err := planner.Plan(context.Background(), frontier)
	require.NoError(t, err)
	assert.Len(t, plan, 1)
	assert.Equal(t, "linux_sre", plan[0].ExpertName)
}

func TestPlanner_Plan_SelectsSingleMatchingExpert(t *testing.T) {
	expertsMap := map[string]experts.ExpertAgent{
		"linux_sre":    &mockExpert{name: "linux_sre"},
		"network_sre":  &mockExpert{name: "network_sre"},
		"database_sre": &mockExpert{name: "database_sre"},
	}
	cfg := DefaultConfig()
	logger := &testLogger{}
	planner := NewPlanner(expertsMap, cfg, logger)

	plan, err := planner.Plan(context.Background(), &belief.Frontier{
		NodeID: "test",
		Label:  "网络问题",
		Why:    "跨区域延迟升高",
	})
	require.NoError(t, err)
	assert.Len(t, plan, 1)
	assert.Equal(t, "network_sre", plan[0].ExpertName)
}

func TestPlanner_Plan_FallsBackDeterministically(t *testing.T) {
	expertsMap := map[string]experts.ExpertAgent{
		"z_sre": &mockExpert{name: "z_sre"},
		"a_sre": &mockExpert{name: "a_sre"},
	}
	cfg := DefaultConfig()
	logger := &testLogger{}
	planner := NewPlanner(expertsMap, cfg, logger)

	plan, err := planner.Plan(context.Background(), &belief.Frontier{
		NodeID: "test",
		Label:  "资源耗尽",
	})
	require.NoError(t, err)
	assert.Len(t, plan, 1)
	assert.Equal(t, "a_sre", plan[0].ExpertName)
}

func TestGoSEngine_Run_NilExpertResult(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 2

	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	engine.RegisterExpert("linux_sre", &mockExpert{
		name:     "linux_sre",
		response: nil,
	})

	result := engine.Run(context.Background(), "服务响应超时")
	require.NotNil(t, result)
	assert.Equal(t, protocol.ResultStatusDegraded, result.Status)
}

func TestGoSEngine_Run_AllExpertsDegraded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 2

	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	engine.RegisterExpert("linux_sre", &mockExpert{
		name: "linux_sre",
		response: &experts.ExpertAnalysis{
			ExpertName:        "linux_sre",
			Status:            "degraded",
			DegradationReason: "partial_data",
			Analysis:          "部分数据提示 CPU 高负载，需要继续确认",
			Confidence:        0.3,
		},
	})

	result := engine.Run(context.Background(), "服务响应超时")
	require.NotNil(t, result)
	assert.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Equal(t, "部分数据提示 CPU 高负载，需要继续确认", result.Summary)
}

func TestGoSEngine_DegradedSummaryRejectsWeakText(t *testing.T) {
	cfg := DefaultConfig()
	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)

	summary, _ := engine.degradedSummary("act_failed", assert.AnError, &ActResult{
		Analyses: []*experts.ExpertAnalysis{
			{
				ExpertName: "linux_sre",
				Status:     "degraded",
				Analysis:   "诊断降级",
				ToolErrors: []experts.ToolError{
					{ToolName: "query_internal_docs", Action: "execute", Error: "no documents found"},
				},
			},
		},
	})
	assert.Contains(t, summary, "缺少或不可用的证据")
	assert.Contains(t, summary, "query_internal_docs execute: no documents found")
	assert.NotEqual(t, "诊断降级", summary)
}

func TestGoSEngine_DegradedResultSkipsInputEvidence(t *testing.T) {
	cfg := DefaultConfig()
	logger := &testLogger{}
	engine := NewGoSEngine(cfg, logger)
	graph := belief.NewBeliefGraph()
	require.NoError(t, NewIngestor(graph, logger).Ingest(context.Background(), "服务响应超时"))
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

	result := engine.degradedResult(graph, fsm, time.Now(), &RunStats{}, "act_failed", assert.AnError, nil, false)
	require.NotNil(t, result)
	assert.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Empty(t, result.Evidence)
	assert.Contains(t, result.Summary, "GoS 未获得足够可用证据")
}
