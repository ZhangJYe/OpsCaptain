package gos_engine

import (
	"context"
	"testing"
	"time"

	einoschema "github.com/cloudwego/eino/schema"
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

type plannedMockExpert struct {
	name     string
	delay    time.Duration
	task     experts.ExpertTask
	response *experts.ExpertAnalysis
}

func (m *plannedMockExpert) Name() string {
	return m.name
}

func (m *plannedMockExpert) Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *experts.ExpertAnalysis {
	return m.RunPlanned(ctx, experts.ExpertTask{Frontier: frontier, Graph: graph})
}

func (m *plannedMockExpert) RunPlanned(ctx context.Context, task experts.ExpertTask) *experts.ExpertAnalysis {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return &experts.ExpertAnalysis{ExpertName: m.name, Status: "degraded", DegradationReason: "context_cancelled"}
		}
	}
	m.task = task
	if m.response == nil {
		return nil
	}
	response := *m.response
	return &response
}

func (m *mockExpert) Name() string {
	return m.name
}

func (m *mockExpert) Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *experts.ExpertAnalysis {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return &experts.ExpertAnalysis{
				ExpertName:        m.name,
				Status:            "degraded",
				DegradationReason: "context_cancelled",
			}
		}
	}
	if m.response == nil {
		return nil
	}
	response := *m.response
	response.Evidence = append([]experts.EvidenceItem(nil), m.response.Evidence...)
	for i := range response.Evidence {
		if response.Evidence[i].TargetHypothesisID == "" && frontier != nil {
			response.Evidence[i].TargetHypothesisID = frontier.NodeID
		}
	}
	return &response
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
					Relation:   experts.EvidenceRelationSupport,
					Strength:   1.0,
				},
			},
		},
	})

	result := engine.Run(context.Background(), "服务响应超时")
	require.NotNil(t, result)
	assert.Equal(t, "gos_engine", result.Agent)
}

func TestGoSEngineRunRealBaseExpertCanReachMinSupport(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 2
	cfg.FSM.MinConfidence = 0.65
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 2
	cfg.StateConversion.Enabled = true
	cfg.StateConversion.MaxDepth = 2

	expert := experts.NewBaseExpert(experts.ExpertRuntimeConfig{
		Name:              "linux_sre",
		MaxRetrievalSteps: 2,
		CallTimeout:       time.Second,
		RAGQueryFunc: func(context.Context, string) ([]*einoschema.Document, error) {
			return []*einoschema.Document{{Content: "cpu_usage=0.95"}}, nil
		},
		GenerateContentFunc: func(_ context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, _ []experts.RetrievalRecord, decision map[string]string) (string, error) {
			if decision["action"] == "retrieve" {
				return "查询 CPU 使用率", nil
			}
			return `{"analysis":"CPU 饱和导致服务延迟","confidence":0.85,"evidence":[{"index":0,"relation":"support","strength":1.0}]}`, nil
		},
	}, experts.NewToolRegistry())
	engine := NewGoSEngine(cfg, &testLogger{})
	engine.RegisterExpert("linux_sre", expert)

	result := engine.Run(context.Background(), "服务响应超时且 CPU 使用率持续 95%")

	require.NotNil(t, result)
	require.Equal(t, protocol.ResultStatusSucceeded, result.Status)
	frontier, ok := result.Metadata["frontier"].(*belief.Frontier)
	require.True(t, ok)
	require.NotNil(t, frontier)
	assert.GreaterOrEqual(t, frontier.Supports, cfg.FSM.MinSupport)
	assert.GreaterOrEqual(t, frontier.Score, cfg.FSM.MinConfidence)
	require.NotEmpty(t, result.Evidence)
	history, ok := result.Metadata["fsm_history"].([]belief.FSMTransition)
	require.True(t, ok)
	require.Len(t, history, 2)
	assert.Equal(t, 1, history[0].FromLevel)
	assert.Equal(t, 2, history[0].ToLevel)
	assert.Contains(t, history[0].Reason, "more actionable")
}

func TestGoSEngine_Run_StateConversionRefinesBeforeReporting(t *testing.T) {
	cfg := phase2TestConfig()
	cfg.SessionMaxSteps = 2
	cfg.FSM.GapDelta = 0.3
	cfg.FSM.MinConfidence = 0.7
	cfg.FSM.MinSupport = 1
	engine := NewGoSEngine(cfg, &testLogger{})
	engine.RegisterExpert("linux_sre", &mockExpert{
		name: "linux_sre",
		response: &experts.ExpertAnalysis{
			ExpertName: "linux_sre",
			Analysis:   "CPU 指标支持当前假设",
			Confidence: 0.9,
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					SourceType: "metric",
					SourceID:   "cpu-phase2",
					Title:      "CPU usage 95%",
					Snippet:    "cpu_usage=0.95",
					Score:      1,
					Relation:   experts.EvidenceRelationSupport,
					Strength:   1,
				},
			},
		},
	})

	result := engine.Run(context.Background(), "服务响应超时")

	require.NotNil(t, result)
	assert.Equal(t, protocol.ResultStatusSucceeded, result.Status)
	history, ok := result.Metadata["fsm_history"].([]belief.FSMTransition)
	require.True(t, ok)
	require.Len(t, history, 2)
	assert.Contains(t, history[0].Reason, "more actionable")
	assert.Equal(t, 1, history[0].FromLevel)
	assert.Equal(t, 2, history[0].ToLevel)
	frontier, ok := result.Metadata["frontier"].(*belief.Frontier)
	require.True(t, ok)
	assert.Equal(t, 2, frontier.Level)
	assert.Equal(t, "CPU 饱和", frontier.Label)
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

func TestGoSEngine_ActUsesStablePlanOrderAndCompressedGraph(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Execution.MaxConcurrentExperts = 2
	engine := NewGoSEngine(cfg, &testLogger{})
	network := &plannedMockExpert{
		name:  "network_sre",
		delay: 40 * time.Millisecond,
		response: &experts.ExpertAnalysis{
			ExpertName: "network_sre", Status: "succeeded", LLMCalls: 1,
		},
	}
	linux := &plannedMockExpert{
		name:  "linux_sre",
		delay: time.Millisecond,
		response: &experts.ExpertAnalysis{
			ExpertName: "linux_sre", Status: "succeeded", LLMCalls: 1,
		},
	}
	engine.RegisterExpert(network.name, network)
	engine.RegisterExpert(linux.name, linux)
	graph := belief.NewBeliefGraph()
	graph.StartSignalID = graph.AddSignal("checkout latency increased")
	frontierID := graph.AddHypothesis("网络问题", 0.7, 1, "检查网络遥测")
	unrelatedID := graph.AddHypothesis("数据库问题", 0.3, 1, "检查数据库")
	frontier := &belief.Frontier{NodeID: frontierID, Label: "网络问题", Why: "检查网络遥测", Level: 1}
	plan := []PlanItem{
		{
			ExpertName: "network_sre", TargetHypothesisID: frontierID, Reason: "network evidence",
			ExpectedEvidence: []string{"latency"}, AllowedTools: []string{"query_logs"}, StopConditions: []string{"evidence found"},
			Budget: PlanBudgetConfig{LLMCalls: 1, ToolCalls: 1, RAGCalls: 1, TimeoutMs: 1000, MaxRetrievalSteps: 1, MaxOutputTokens: 64},
		},
		{
			ExpertName: "linux_sre", TargetHypothesisID: frontierID, Reason: "host evidence",
			ExpectedEvidence: []string{"cpu"}, AllowedTools: []string{"query_logs"}, StopConditions: []string{"evidence found"},
			Budget: PlanBudgetConfig{LLMCalls: 1, ToolCalls: 1, RAGCalls: 1, TimeoutMs: 1000, MaxRetrievalSteps: 1, MaxOutputTokens: 64},
		},
	}

	result, err := engine.act(context.Background(), plan, frontier, graph, &RunStats{})

	require.NoError(t, err)
	require.Len(t, result.Analyses, 2)
	assert.Equal(t, "network_sre", result.Analyses[0].ExpertName)
	assert.Equal(t, "linux_sre", result.Analyses[1].ExpertName)
	assert.NotContains(t, network.task.Graph.Nodes, unrelatedID)
	assert.NotContains(t, linux.task.Graph.Nodes, unrelatedID)
	assert.Contains(t, network.task.Graph.Nodes, frontierID)
	assert.Equal(t, []string{"latency"}, network.task.ExpectedEvidence)
	assert.Equal(t, []string{"query_logs"}, network.task.AllowedTools)
}

func TestGoSEngine_ActKeepsUsableEvidenceWhenOtherExpertFails(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	frontierID := graph.AddHypothesis("网络问题", 0.5, 1, "检查网络遥测")
	frontier := &belief.Frontier{NodeID: frontierID, Label: "网络问题"}
	engine.RegisterExpert("network_sre", &mockExpert{name: "network_sre", response: &experts.ExpertAnalysis{
		ExpertName: "network_sre", Status: "failed", DegradationReason: "tool_failed",
	}})
	engine.RegisterExpert("linux_sre", &mockExpert{name: "linux_sre", response: &experts.ExpertAnalysis{
		ExpertName: "linux_sre", Status: "degraded", DegradationReason: "partial_tool_failure",
		Evidence: []experts.EvidenceItem{{
			SourceType: "metric", SourceID: "cpu-1", TargetHypothesisID: frontierID,
			Relation: experts.EvidenceRelationNeutral,
		}},
	}})
	plan := []PlanItem{{ExpertName: "network_sre"}, {ExpertName: "linux_sre"}}

	result, err := engine.act(context.Background(), plan, frontier, graph, &RunStats{})

	require.NoError(t, err)
	assert.Equal(t, 1, result.FailedCount)
	assert.Equal(t, 1, result.DegradedCount)
	require.Len(t, result.Analyses[1].Evidence, 1)
}

func TestGoSEngine_PartialExpertFailureCommitsValidEvidenceAndDegradesReport(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	frontierID := graph.AddHypothesis("网络问题", 0.5, 1, "检查网络遥测")
	frontier := &belief.Frontier{NodeID: frontierID, Label: "网络问题", Level: 1}
	engine.RegisterExpert("network_sre", &mockExpert{name: "network_sre", response: &experts.ExpertAnalysis{
		ExpertName: "network_sre", Status: "failed", DegradationReason: "tool_failed",
	}})
	engine.RegisterExpert("linux_sre", &mockExpert{name: "linux_sre", response: &experts.ExpertAnalysis{
		ExpertName: "linux_sre", Status: "succeeded", Analysis: "CPU 指标支持资源侧异常", Confidence: 0.8,
		Evidence: []experts.EvidenceItem{{
			SourceType: "metric", SourceID: "cpu-1", Title: "CPU 使用率 95%", Snippet: "cpu=0.95",
			TargetHypothesisID: frontierID, Relation: experts.EvidenceRelationSupport, Strength: 0.8,
		}},
	}})
	plan := []PlanItem{{ExpertName: "network_sre"}, {ExpertName: "linux_sre"}}
	stats := &RunStats{}

	actResult, err := engine.act(context.Background(), plan, frontier, graph, stats)
	require.NoError(t, err)
	stats.ExpertFailed += actResult.FailedCount
	stats.ExpertDegraded += actResult.DegradedCount
	updateResult := engine.updateGraph(context.Background(), graph, actResult.Analyses, frontier)
	require.True(t, updateResult.Committed, updateResult.Error)
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
	report := engine.generateReport(context.Background(), graph, fsm, time.Now(), stats)

	assert.Equal(t, protocol.ResultStatusDegraded, report.Status)
	assert.Contains(t, report.DegradationReason, "partial_expert_failure")
	require.Len(t, report.Evidence, 1)
	assert.Equal(t, "cpu-1", report.Evidence[0].SourceID)
}

func TestGoSEngine_RunStopsAfterConfiguredNoProgressRounds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 5
	cfg.Execution.NoProgressRoundLimit = 2
	engine := NewGoSEngine(cfg, &testLogger{})
	engine.RegisterExpert("database_sre", &mockExpert{name: "database_sre", response: &experts.ExpertAnalysis{
		ExpertName: "database_sre", Status: "succeeded",
	}})
	engine.RegisterExpert("linux_sre", &mockExpert{name: "linux_sre", response: &experts.ExpertAnalysis{
		ExpertName: "linux_sre", Status: "succeeded",
	}})

	result := engine.Run(context.Background(), "服务响应超时")

	require.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Contains(t, result.DegradationReason, "no_progress_loop")
	assert.Equal(t, 2, result.Metadata["no_progress_rounds"])
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
					SourceType:         "test",
					SourceID:           "ev-1",
					Title:              "Test evidence",
					Snippet:            "Test snippet",
					Score:              1.0,
					Relation:           experts.EvidenceRelationSupport,
					TargetHypothesisID: hypoID,
					Strength:           1.0,
				},
			},
		},
	}

	result := engine.updateGraph(context.Background(), graph, analyses, frontier)
	assert.True(t, result.Committed)
	assert.Len(t, graph.Nodes, 2)
}

func TestGoSEngine_UpdateGraph_UsesExplicitSupportRelation(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypoID := graph.AddHypothesis("Network issue", 0.5, 1, "Initial")
	frontier := &belief.Frontier{NodeID: hypoID}

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{
		{
			ExpertName: "network_sre",
			Analysis:   "Packet loss supports a network issue",
			Confidence: 0.2,
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					SourceType:         "metric",
					SourceID:           "packet-loss-1",
					Title:              "Packet loss 8%",
					Snippet:            "packet_loss=0.08",
					Score:              0.9,
					Relation:           experts.EvidenceRelationSupport,
					TargetHypothesisID: hypoID,
					Strength:           0.9,
				},
			},
		},
	}, frontier)

	require.True(t, result.Committed)
	updated := graph.ExtractFrontier(1)
	require.NotNil(t, updated)
	assert.Greater(t, updated.Score, 0.5)
	assert.Equal(t, 1, updated.Supports)
	assert.Equal(t, 0, updated.Refutes)
	contributions, ok := graph.Nodes[hypoID].Attrs["confidence_contributions"].([]confidenceContributionDetail)
	require.True(t, ok)
	require.Len(t, contributions, 1)
	assert.Equal(t, string(experts.EvidenceRelationSupport), contributions[0].Relation)
	assert.Equal(t, 0.9, contributions[0].Strength)
	assert.NotEmpty(t, contributions[0].EvidenceKey)
}

func TestGoSEngine_UpdateGraph_RefuteLowersConfidence(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypoID := graph.AddHypothesis("Database issue", 0.8, 1, "Initial")
	frontier := &belief.Frontier{NodeID: hypoID}

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{
		{
			ExpertName: "database_sre",
			Analysis:   "Connection pool is healthy",
			Confidence: 0.95,
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					SourceType:         "metric",
					SourceID:           "pool-healthy-1",
					Title:              "Connection pool healthy",
					Snippet:            "active=10 max=100",
					Score:              1,
					Relation:           experts.EvidenceRelationRefute,
					TargetHypothesisID: hypoID,
					Strength:           1,
				},
			},
		},
	}, frontier)

	require.True(t, result.Committed)
	updated := graph.ExtractFrontier(1)
	require.NotNil(t, updated)
	assert.Less(t, updated.Score, 0.8)
	assert.Equal(t, 0, updated.Supports)
	assert.Equal(t, 1, updated.Refutes)
}

func TestGoSEngine_UpdateGraph_DeduplicatesEvidence(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypoID := graph.AddHypothesis("Resource exhaustion", 0.5, 1, "Initial")
	frontier := &belief.Frontier{NodeID: hypoID}
	analysis := &experts.ExpertAnalysis{
		ExpertName: "linux_sre",
		Analysis:   "CPU is saturated",
		Confidence: 0.9,
		Status:     "succeeded",
		Evidence: []experts.EvidenceItem{
			{
				SourceType:         "metric",
				SourceID:           "cpu-observation-1",
				Title:              "CPU 95%",
				Snippet:            "cpu_usage=0.95",
				Score:              1,
				Relation:           experts.EvidenceRelationSupport,
				TargetHypothesisID: hypoID,
				Strength:           1,
			},
		},
	}

	first := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{analysis}, frontier)
	require.True(t, first.Committed)
	firstScore := graph.Nodes[hypoID].Score
	second := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{analysis}, frontier)
	require.True(t, second.Committed)

	assert.Len(t, graph.Nodes, 2)
	assert.Equal(t, firstScore, graph.Nodes[hypoID].Score)
}

func TestGoSEngine_UpdateGraph_PromotesSupportedHypothesisActionability(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("客户端重试机制导致请求放大", 0.7, 1, "verify retry evidence")
	frontier := &belief.Frontier{NodeID: hypothesisID}
	actionable := true
	analysis := &experts.ExpertAnalysis{
		ExpertName:                  "linux_sre",
		Analysis:                    "日志确认无退避重试导致请求放大",
		Confidence:                  0.9,
		Status:                      "succeeded",
		CurrentHypothesisActionable: &actionable,
		Evidence: []experts.EvidenceItem{{
			SourceType: "log", SourceID: "retry-log", Title: "HTTP 503 without backoff", Snippet: "requests amplified",
			Score: 1, Relation: experts.EvidenceRelationSupport, TargetHypothesisID: hypothesisID, Strength: 0.9,
		}},
	}

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{analysis}, frontier)

	require.True(t, result.Committed, result.Error)
	assert.Equal(t, true, graph.Nodes[hypothesisID].Attrs["actionable"])
	assert.Equal(t, "expert_support_evidence_v1", graph.Nodes[hypothesisID].Attrs["actionable_source"])
}

func TestGoSEngine_UpdateGraph_InvalidTargetRollsBack(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypoID := graph.AddHypothesis("Resource exhaustion", 0.5, 1, "Initial")
	frontier := &belief.Frontier{NodeID: hypoID}

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{
		{
			ExpertName: "linux_sre",
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					SourceType:         "metric",
					SourceID:           "cpu-observation-1",
					Title:              "CPU 95%",
					Snippet:            "cpu_usage=0.95",
					Score:              1,
					Relation:           experts.EvidenceRelationSupport,
					TargetHypothesisID: "missing-hypothesis",
					Strength:           1,
				},
			},
		},
	}, frontier)

	assert.False(t, result.Committed)
	assert.Error(t, result.Error)
	assert.Len(t, graph.Nodes, 1)
	assert.Equal(t, 0.5, graph.Nodes[hypoID].Score)
}

func TestGoSEngine_UpdateGraph_MissingSourceRollsBack(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypoID := graph.AddHypothesis("Resource exhaustion", 0.5, 1, "Initial")
	frontier := &belief.Frontier{NodeID: hypoID}

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{
		{
			ExpertName: "linux_sre",
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					Title:              "CPU 95%",
					Snippet:            "cpu_usage=0.95",
					Score:              1,
					Relation:           experts.EvidenceRelationSupport,
					TargetHypothesisID: hypoID,
					Strength:           1,
				},
			},
		},
	}, frontier)

	assert.False(t, result.Committed)
	assert.Error(t, result.Error)
	assert.Len(t, graph.Nodes, 1)
}

func TestGoSEngine_UpdateGraph_AppliesSourceBackedRefinementProposal(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("资源耗尽", 0.5, 1, "检查资源指标")
	frontier := &belief.Frontier{NodeID: hypothesisID, Label: "资源耗尽", Level: 1}

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "linux_sre",
		Analysis:   "CPU 指标表明需要细化到 CPU 饱和",
		Confidence: 0.9,
		Status:     "succeeded",
		Evidence: []experts.EvidenceItem{{
			SourceType:         "metric",
			SourceID:           "cpu-usage-1",
			Title:              "CPU 使用率持续 96%",
			Snippet:            "cpu_usage=0.96",
			Score:              0.96,
			Relation:           experts.EvidenceRelationSupport,
			TargetHypothesisID: hypothesisID,
			Strength:           0.9,
		}},
		Refinements: []experts.HypothesisRefinement{{
			Label:      "CPU 饱和",
			Score:      0.85,
			Why:        "CPU 使用率持续高位，需要核验负载和节流",
			Actionable: true,
		}},
	}}, frontier)

	require.True(t, result.Committed, result.Error)
	require.Len(t, graph.GetActiveHypothesisCopies(2), 1)
	child := graph.GetActiveHypothesisCopies(2)[0]
	assert.Equal(t, "CPU 饱和", child.Label)
	assert.Equal(t, true, child.Attrs["actionable"])
	assert.Equal(t, "linux_sre", child.Attrs["proposal_expert"])
	edge, ok := graph.Edges[hypothesisID+"->"+child.ID]
	require.True(t, ok)
	assert.Equal(t, belief.EdgeRefines, edge.Type)
	assert.Equal(t, "expert_graph_proposal_v1", edge.DerivationType)
	assert.NoError(t, validateBeliefGraph(graph))
}

func TestGoSEngine_UpdateGraph_InvalidRefinementProposalDoesNotPartiallyMutateGraph(t *testing.T) {
	tests := []struct {
		name       string
		refinement experts.HypothesisRefinement
		wantError  string
	}{
		{
			name:       "score out of range",
			refinement: experts.HypothesisRefinement{Label: "CPU 饱和", Score: 1.1, Why: "CPU 使用率高"},
			wantError:  "score must be within",
		},
		{
			name:       "semantic refines cycle",
			refinement: experts.HypothesisRefinement{Label: "资源耗尽", Score: 0.8, Why: "重复父假设"},
			wantError:  "semantic refines cycle",
		},
		{
			name:       "missing reason",
			refinement: experts.HypothesisRefinement{Label: "CPU 饱和", Score: 0.8},
			wantError:  "label and why are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.StructuredCognition.Enabled = true
			engine := NewGoSEngine(cfg, &testLogger{})
			graph := belief.NewBeliefGraph()
			hypothesisID := graph.AddHypothesis("资源耗尽", 0.5, 1, "检查资源指标")
			before := graph.ToDict()
			beforeStep := graph.CurrentStep
			beforeSnapshots := len(graph.Snapshots)

			result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
				ExpertName: "linux_sre",
				Confidence: 0.8,
				Evidence: []experts.EvidenceItem{{
					SourceType:         "metric",
					SourceID:           "cpu-usage-1",
					Title:              "CPU 使用率",
					Relation:           experts.EvidenceRelationSupport,
					TargetHypothesisID: hypothesisID,
					Strength:           0.8,
				}},
				Refinements: []experts.HypothesisRefinement{tt.refinement},
			}}, &belief.Frontier{NodeID: hypothesisID})

			require.False(t, result.Committed)
			require.ErrorContains(t, result.Error, tt.wantError)
			assert.Equal(t, before, graph.ToDict())
			assert.Equal(t, beforeStep, graph.CurrentStep)
			assert.Len(t, graph.Snapshots, beforeSnapshots)
		})
	}
}

func TestGoSEngine_UpdateGraph_RejectsWholeBatchBeforeMutation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("资源耗尽", 0.5, 1, "检查资源指标")
	before := graph.ToDict()

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{
		{
			ExpertName: "a_linux_sre",
			Confidence: 0.8,
			Evidence: []experts.EvidenceItem{{
				SourceType:         "metric",
				SourceID:           "cpu-usage-1",
				Title:              "CPU 使用率",
				Relation:           experts.EvidenceRelationSupport,
				TargetHypothesisID: hypothesisID,
				Strength:           0.8,
			}},
			Refinements: []experts.HypothesisRefinement{{
				Label: "CPU 饱和", Score: 0.8, Why: "CPU 使用率持续高位",
			}},
		},
		{
			ExpertName: "z_network_sre",
			Confidence: 0.7,
			Evidence: []experts.EvidenceItem{{
				SourceType:         "metric",
				Title:              "无来源标识的网络指标",
				Relation:           experts.EvidenceRelationNeutral,
				TargetHypothesisID: hypothesisID,
			}},
		},
	}, &belief.Frontier{NodeID: hypothesisID})

	require.False(t, result.Committed)
	require.ErrorContains(t, result.Error, "source_type and source_id are required")
	assert.Equal(t, before, graph.ToDict())
	assert.Len(t, graph.Nodes, 1)
}

func TestGoSEngine_UpdateGraph_IgnoresRefinementWhenStructuredCognitionDisabled(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("资源耗尽", 0.5, 1, "检查资源指标")

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "linux_sre",
		Confidence: 0.8,
		Evidence: []experts.EvidenceItem{{
			SourceType:         "metric",
			SourceID:           "cpu-usage-1",
			Title:              "CPU 使用率",
			Relation:           experts.EvidenceRelationSupport,
			TargetHypothesisID: hypothesisID,
			Strength:           0.8,
		}},
		Refinements: []experts.HypothesisRefinement{{
			Label: "CPU 饱和", Score: 0.8, Why: "CPU 使用率持续高位",
		}},
	}}, &belief.Frontier{NodeID: hypothesisID})

	require.True(t, result.Committed, result.Error)
	assert.Empty(t, graph.GetActiveHypothesisCopies(2))
}

func TestGoSEngine_UpdateGraph_NeutralEvidenceDoesNotChangeConfidence(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypoID := graph.AddHypothesis("Network issue", 0.5, 1, "Initial")
	frontier := &belief.Frontier{NodeID: hypoID}

	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{
		{
			ExpertName: "network_sre",
			Status:     "succeeded",
			Evidence: []experts.EvidenceItem{
				{
					SourceType:         "log",
					SourceID:           "unrelated-log-1",
					Title:              "Unrelated log",
					Snippet:            "background cleanup completed",
					Score:              0.4,
					Relation:           experts.EvidenceRelationNeutral,
					TargetHypothesisID: hypoID,
					Strength:           0,
				},
			},
		},
	}, frontier)

	require.True(t, result.Committed)
	assert.Len(t, graph.Nodes, 2)
	updated := graph.ExtractFrontier(1)
	require.NotNil(t, updated)
	assert.Equal(t, 0.5, updated.Score)
	assert.Zero(t, updated.Supports)
	assert.Zero(t, updated.Refutes)
}

func TestGoSEngine_UpdateGraph_InvalidConfidenceConfigRollsBack(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Confidence.SupportWeight = 1.1
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	hypoID := graph.AddHypothesis("Resource exhaustion", 0.5, 1, "Initial")

	result := engine.updateGraph(context.Background(), graph, nil, &belief.Frontier{NodeID: hypoID})

	assert.False(t, result.Committed)
	assert.Error(t, result.Error)
	assert.Len(t, graph.Nodes, 1)
}

func TestValidateBeliefGraphRejectsEvidenceWithoutProvenance(t *testing.T) {
	graph := belief.NewBeliefGraph()
	graph.AddEvidence("unverifiable evidence", nil)

	err := validateBeliefGraph(graph)

	assert.Error(t, err)
}

func TestValidateBeliefGraphRejectsDanglingEdge(t *testing.T) {
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("Resource exhaustion", 0.5, 1, "Initial")
	graph.Edges["missing->"+hypothesisID] = &belief.Edge{
		Src:        "missing",
		Dst:        hypothesisID,
		Type:       belief.EdgeSupport,
		Status:     belief.StatusActive,
		Confidence: 1,
	}

	err := validateBeliefGraph(graph)

	assert.Error(t, err)
}

func TestValidateBeliefGraphRejectsEvidenceRelationMismatch(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("Resource exhaustion", 0.5, 1, "Initial")
	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "linux_sre",
		Evidence: []experts.EvidenceItem{{
			SourceType:         "metric",
			SourceID:           "cpu-1",
			Title:              "CPU",
			Relation:           experts.EvidenceRelationSupport,
			TargetHypothesisID: hypothesisID,
			Strength:           0.8,
		}},
	}}, &belief.Frontier{NodeID: hypothesisID})
	require.True(t, result.Committed)

	for _, node := range graph.Nodes {
		if node.Type == belief.NodeEvidence {
			node.Source.Relation = string(experts.EvidenceRelationRefute)
			break
		}
	}
	assert.Error(t, validateBeliefGraph(graph))
}

func TestValidateBeliefGraphRejectsSupportEvidenceWithoutEdge(t *testing.T) {
	engine := NewGoSEngine(DefaultConfig(), &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("Resource exhaustion", 0.5, 1, "Initial")
	result := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "linux_sre",
		Evidence: []experts.EvidenceItem{{
			SourceType:         "metric",
			SourceID:           "cpu-1",
			Title:              "CPU",
			Relation:           experts.EvidenceRelationSupport,
			TargetHypothesisID: hypothesisID,
			Strength:           0.8,
		}},
	}}, &belief.Frontier{NodeID: hypothesisID})
	require.True(t, result.Committed)

	graph.Edges = map[string]*belief.Edge{}
	assert.Error(t, validateBeliefGraph(graph))
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
					Relation:   experts.EvidenceRelationSupport,
					Strength:   1.0,
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

	evidenceCount := 0
	refinesCount := 0
	for _, node := range graph.Nodes {
		if node.Type == belief.NodeEvidence {
			evidenceCount++
		}
	}
	for _, edge := range graph.Edges {
		if edge.Src == graph.StartSignalID && edge.Type == belief.EdgeRefines {
			refinesCount++
		}
	}
	assert.Zero(t, evidenceCount)
	assert.Equal(t, 3, refinesCount)
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
	assert.Contains(t, result.Summary, "## 结论")
	assert.Contains(t, result.Summary, "证据不足，不能确认为根因")
	assert.Contains(t, result.Summary, "部分数据提示 CPU 高负载，需要继续确认")
	assert.Contains(t, result.Metadata, "evidence_report")
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
