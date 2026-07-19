package gos_engine

import (
	"context"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/belief"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredIngestorAppliesValidatedProposalAtomically(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	graph := belief.NewBeliefGraph()
	ingestor := NewStructuredIngestor(graph, cfg, &testLogger{}, func(_ context.Context, prompt string) (string, error) {
		assert.Contains(t, prompt, "checkout 延迟升高")
		return `{
  "signal":"checkout 延迟升高",
  "observations":[
    {"label":"P99 延迟超过 2 秒","source_type":"telemetry","source_id":"metric://checkout/p99"},
    {"label":"刚完成配置发布","source_type":"change","source_id":"change://deploy-1"}
  ],
  "hypotheses":[
    {"label":"依赖网络延迟","score":0.7,"why":"调用链延迟与现象一致","actionable":true},
    {"label":"配置变更错误","score":0.3,"why":"故障紧随发布出现","actionable":false}
  ]
}`, nil
	})

	outcome, err := ingestor.IngestWithOutcome(context.Background(), "checkout 延迟升高")
	require.NoError(t, err)
	assert.Equal(t, "structured", outcome.Mode)
	assert.Equal(t, 1, outcome.LLMCalls)
	assert.Equal(t, 2, outcome.ObservationCount)
	assert.Equal(t, 2, outcome.HypothesisCount)
	require.NoError(t, ValidateBeliefGraph(graph))

	signalCount, observationCount, hypothesisCount := 0, 0, 0
	actionability := map[string]bool{}
	for _, node := range graph.Nodes {
		switch node.Type {
		case belief.NodeSignal:
			signalCount++
			if node.Attrs["semantic_type"] == "observation" {
				observationCount++
			}
		case belief.NodeHypothesis:
			hypothesisCount++
			actionability[node.Label], _ = node.Attrs["actionable"].(bool)
		}
	}
	assert.Equal(t, 3, signalCount)
	assert.Equal(t, 2, observationCount)
	assert.Equal(t, 2, hypothesisCount)
	assert.True(t, actionability["依赖网络延迟"])
	assert.False(t, actionability["配置变更错误"])
	assert.Equal(t, "structured", graph.Nodes[graph.StartSignalID].Attrs["ingest_mode"])

	var actionableID string
	for id, node := range graph.Nodes {
		if node.Type == belief.NodeHypothesis && node.Label == "依赖网络延迟" {
			actionableID = id
		}
	}
	require.NotEmpty(t, actionableID)
	addTestSupport(graph, actionableID, 0.9)
	addTestSupport(graph, actionableID, 0.8)
	decision := NewStateConverter(cfg).Decide(graph, belief.NewBeliefFSM(cfg.ToFSMThresholds()), ConversionBudget{UsedSteps: 1, MaxSteps: cfg.SessionMaxSteps})
	assert.Equal(t, DecisionReport, decision.Action)
	assert.Equal(t, "actionable_root_cause", decision.ReasonCode)
}

func TestStructuredIngestorUsesTrustedBootstrapEvidenceProvenance(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	graph := belief.NewBeliefGraph()
	ingestor := NewStructuredIngestor(graph, cfg, &testLogger{}, func(_ context.Context, prompt string) (string, error) {
		assert.Contains(t, prompt, "metric://mysql/connections")
		assert.Contains(t, prompt, "active connections 100/100")
		return `{
  "signal":"checkout 在故障时间窗内超时",
  "observations":[{"label":"模型伪造观测","source_type":"telemetry","source_id":"invented://source"}],
  "hypotheses":[{"label":"MySQL 连接池耗尽","score":0.9,"why":"活跃连接达到上限","actionable":true}]
}`, nil
	})

	outcome, err := ingestor.IngestWithBootstrap(context.Background(), "checkout 在故障时间窗内超时", []BootstrapEvidence{{
		SourceType: "metric",
		SourceID:   "metric://mysql/connections",
		Title:      "MySQL connections",
		Snippet:    "active connections 100/100",
	}})
	require.NoError(t, err)
	assert.Equal(t, "structured", outcome.Mode)
	assert.Equal(t, 1, outcome.ObservationCount)

	foundTrustedObservation := false
	for _, node := range graph.Nodes {
		if node.Attrs["semantic_type"] != "observation" {
			continue
		}
		assert.Equal(t, "metric://mysql/connections", node.Attrs["source_id"])
		assert.NotEqual(t, "invented://source", node.Attrs["source_id"])
		foundTrustedObservation = true
	}
	assert.True(t, foundTrustedObservation)
}

func TestStructuredIngestorDoesNotCreateGenericHypothesesWhenBootstrapIngestFails(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	graph := belief.NewBeliefGraph()
	ingestor := NewStructuredIngestor(graph, cfg, &testLogger{}, func(context.Context, string) (string, error) {
		return "", context.DeadlineExceeded
	})

	outcome, err := ingestor.IngestWithBootstrap(context.Background(), "仅包含故障时间窗", []BootstrapEvidence{{
		SourceType: "rag",
		SourceID:   "rag://snapshot",
		Snippet:    "pod payment-0 restart_count increased to 12",
	}})
	require.Error(t, err)
	assert.Equal(t, "rule_fallback", outcome.Mode)
	assert.Equal(t, "structured_generation_failed", outcome.FallbackReason)
	assert.Empty(t, graph.Nodes)
	assert.Empty(t, graph.Edges)
}

func TestStructuredIngestorRequiresExplicitActionability(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	graph := belief.NewBeliefGraph()
	ingestor := NewStructuredIngestor(graph, cfg, &testLogger{}, func(context.Context, string) (string, error) {
		return `{"signal":"延迟升高","observations":[],"hypotheses":[{"label":"网络问题","score":0.8,"why":"需要验证"}]}`, nil
	})

	outcome, err := ingestor.IngestWithOutcome(context.Background(), "延迟升高")
	require.NoError(t, err)
	assert.Equal(t, "rule_fallback", outcome.Mode)
	assert.Equal(t, "structured_contract_invalid", outcome.FallbackReason)
}

func TestStructuredIngestorInvalidSchemaFallsBackWithoutPartialMutation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	graph := belief.NewBeliefGraph()
	ingestor := NewStructuredIngestor(graph, cfg, &testLogger{}, func(context.Context, string) (string, error) {
		return `{"signal":"污染信号","observations":[],"hypotheses":[{"label":"伪造根因","score":2,"why":"invalid"}],"extra":true}`, nil
	})

	outcome, err := ingestor.IngestWithOutcome(context.Background(), "原始症状")
	require.NoError(t, err)
	assert.Equal(t, "rule_fallback", outcome.Mode)
	assert.Equal(t, "structured_schema_invalid", outcome.FallbackReason)
	require.NoError(t, ValidateBeliefGraph(graph))
	assert.Equal(t, "原始症状", graph.Nodes[graph.StartSignalID].Label)
	assert.Len(t, graph.Nodes, 4)
	for _, node := range graph.Nodes {
		assert.NotEqual(t, "伪造根因", node.Label)
		assert.NotEqual(t, "污染信号", node.Label)
	}
}

func TestStructuredIngestorCancellationLeavesGraphUntouched(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	graph := belief.NewBeliefGraph()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewStructuredIngestor(graph, cfg, &testLogger{}, func(context.Context, string) (string, error) {
		t.Fatal("generator must not run after cancellation")
		return "", nil
	}).IngestWithOutcome(ctx, "症状")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, graph.Nodes)
	assert.Empty(t, graph.Edges)
}

func TestStructuredPlannerProducesEvidenceScopedPlan(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	expertsMap := map[string]experts.ExpertAgent{
		"linux_sre":   &mockExpert{name: "linux_sre"},
		"network_sre": &mockExpert{name: "network_sre"},
	}
	planner := NewStructuredPlanner(expertsMap, cfg, &testLogger{}, func(_ context.Context, prompt string) (string, error) {
		assert.Contains(t, prompt, "called_goal_keys")
		assert.Contains(t, prompt, "network_sre")
		assert.Contains(t, prompt, `"budget":{"llm_calls":3`)
		return `{"items":[{
  "expert_name":"network_sre",
  "reason":"frontier 指向网络延迟，需要验证传播路径",
  "expected_evidence":["调用链延迟和丢包证据"],
  "allowed_tools":["query_logs"],
  "stop_conditions":["获得 source-backed 支持或反驳证据"],
  "budget":{"llm_calls":1,"tool_calls":1,"rag_calls":1,"timeout_ms":10000,"max_retrieval_steps":2,"max_output_tokens":1024}
}]}`, nil
	})

	outcome, err := planner.PlanWithContext(context.Background(), PlanningContext{
		Frontier:        &belief.Frontier{NodeID: "hyp-1", Label: "网络问题", Why: "跨区延迟升高"},
		CalledGoalKeys:  map[string]struct{}{},
		FailedTools:     map[string]struct{}{},
		RemainingBudget: scalePlanBudget(cfg.StructuredCognition.PlanBudget, 2),
	})
	require.NoError(t, err)
	assert.Equal(t, "structured", outcome.Mode)
	assert.Equal(t, 1, outcome.LLMCalls)
	require.Len(t, outcome.Items, 1)
	assert.Equal(t, "network_sre", outcome.Items[0].ExpertName)
	assert.NotEmpty(t, outcome.Items[0].ExpectedEvidence)
	assert.NotEmpty(t, outcome.Items[0].AllowedTools)
	assert.NotEmpty(t, outcome.Items[0].StopConditions)
}

func TestStructuredPlannerSupportsComplementaryMultiExpertPlan(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	expertsMap := map[string]experts.ExpertAgent{
		"network_sre": &mockExpert{name: "network_sre"},
		"linux_sre":   &mockExpert{name: "linux_sre"},
	}
	planner := NewStructuredPlanner(expertsMap, cfg, &testLogger{}, func(context.Context, string) (string, error) {
		return `{"items":[{"expert_name":"network_sre","reason":"inspect request path","expected_evidence":["latency and packet loss"],"allowed_tools":["query_logs"],"stop_conditions":["network evidence found"],"budget":{"llm_calls":1,"tool_calls":1,"rag_calls":1,"timeout_ms":1000,"max_retrieval_steps":1,"max_output_tokens":64}},{"expert_name":"linux_sre","reason":"inspect host saturation","expected_evidence":["cpu and memory"],"allowed_tools":["query_logs"],"stop_conditions":["host evidence found"],"budget":{"llm_calls":1,"tool_calls":1,"rag_calls":1,"timeout_ms":1000,"max_retrieval_steps":1,"max_output_tokens":64}}]}`, nil
	})

	outcome, err := planner.PlanWithContext(context.Background(), PlanningContext{
		Frontier:        &belief.Frontier{NodeID: "hyp-1", Label: "请求超时"},
		CalledGoalKeys:  map[string]struct{}{},
		FailedTools:     map[string]struct{}{},
		RemainingBudget: scalePlanBudget(cfg.StructuredCognition.PlanBudget, 2),
	})

	require.NoError(t, err)
	assert.Equal(t, "structured", outcome.Mode)
	require.Len(t, outcome.Items, 2)
	assert.Equal(t, "network_sre", outcome.Items[0].ExpertName)
	assert.Equal(t, "linux_sre", outcome.Items[1].ExpertName)
}

func TestStructuredPlannerInvalidProposalUsesDeterministicFallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	expertsMap := map[string]experts.ExpertAgent{
		"linux_sre":   &mockExpert{name: "linux_sre"},
		"network_sre": &mockExpert{name: "network_sre"},
	}
	planner := NewStructuredPlanner(expertsMap, cfg, &testLogger{}, func(context.Context, string) (string, error) {
		return `{"items":[{"expert_name":"unknown","reason":"x","expected_evidence":["x"],"allowed_tools":["query_logs"],"stop_conditions":["x"],"budget":{"llm_calls":1,"tool_calls":1,"rag_calls":1,"timeout_ms":1,"max_retrieval_steps":1,"max_output_tokens":1}}]}`, nil
	})

	outcome, err := planner.PlanWithContext(context.Background(), PlanningContext{
		Frontier:        &belief.Frontier{NodeID: "hyp-1", Label: "网络问题"},
		CalledGoalKeys:  map[string]struct{}{},
		FailedTools:     map[string]struct{}{},
		RemainingBudget: scalePlanBudget(cfg.StructuredCognition.PlanBudget, 2),
	})
	require.NoError(t, err)
	assert.Equal(t, "rule_fallback", outcome.Mode)
	assert.Equal(t, "structured_contract_invalid", outcome.FallbackReason)
	assert.Contains(t, outcome.FallbackDetail, "unknown expert")
	require.Len(t, outcome.Items, 1)
	assert.Equal(t, "network_sre", outcome.Items[0].ExpertName)
}

func TestStructuredPlannerClampsBudgetToExpertLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	expertsMap := map[string]experts.ExpertAgent{
		"network_sre": &mockExpert{name: "network_sre"},
	}
	planner := NewStructuredPlanner(expertsMap, cfg, &testLogger{}, func(context.Context, string) (string, error) {
		return `{"items":[{"expert_name":"network_sre","reason":"inspect latency","expected_evidence":["latency"],"allowed_tools":["query_logs"],"stop_conditions":["evidence found"],"budget":{"llm_calls":4,"tool_calls":1,"rag_calls":1,"timeout_ms":1000,"max_retrieval_steps":1,"max_output_tokens":64}}]}`, nil
	})

	outcome, err := planner.PlanWithContext(context.Background(), PlanningContext{
		Frontier:        &belief.Frontier{NodeID: "hyp-1", Label: "网络问题"},
		CalledGoalKeys:  map[string]struct{}{},
		FailedTools:     map[string]struct{}{},
		RemainingBudget: scalePlanBudget(cfg.StructuredCognition.PlanBudget, 2),
	})

	require.NoError(t, err)
	assert.Equal(t, "structured", outcome.Mode)
	assert.Empty(t, outcome.FallbackReason)
	require.Len(t, outcome.Items, 1)
	assert.Equal(t, planner.expertBudget("network_sre").LLMCalls, outcome.Items[0].Budget.LLMCalls)
	assert.Equal(t, 1000, outcome.Items[0].Budget.TimeoutMs)
}

func TestStructuredPlannerFillsMissingBudgetFromConfiguredLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	expertsMap := map[string]experts.ExpertAgent{
		"network_sre": &mockExpert{name: "network_sre"},
	}
	planner := NewStructuredPlanner(expertsMap, cfg, &testLogger{}, func(context.Context, string) (string, error) {
		return `{"items":[{"expert_name":"network_sre","reason":"inspect latency","expected_evidence":["latency"],"allowed_tools":["query_logs"],"stop_conditions":["evidence found"],"budget":{}}]}`, nil
	})

	outcome, err := planner.PlanWithContext(context.Background(), PlanningContext{
		Frontier:        &belief.Frontier{NodeID: "hyp-1", Label: "网络问题"},
		CalledGoalKeys:  map[string]struct{}{},
		FailedTools:     map[string]struct{}{},
		RemainingBudget: scalePlanBudget(cfg.StructuredCognition.PlanBudget, 2),
	})

	require.NoError(t, err)
	assert.Equal(t, "structured", outcome.Mode)
	require.Len(t, outcome.Items, 1)
	assert.Equal(t, planner.expertBudget("network_sre"), outcome.Items[0].Budget)
}

func TestPlannerDoesNotRepeatExpertWithoutNewEvidenceGoal(t *testing.T) {
	cfg := DefaultConfig()
	expertsMap := map[string]experts.ExpertAgent{
		"database_sre": &mockExpert{name: "database_sre"},
		"linux_sre":    &mockExpert{name: "linux_sre"},
		"network_sre":  &mockExpert{name: "network_sre"},
	}
	planner := NewPlanner(expertsMap, cfg, &testLogger{})
	planning := PlanningContext{
		Frontier:        &belief.Frontier{NodeID: "hyp-1", Label: "网络问题"},
		CalledGoalKeys:  map[string]struct{}{},
		FailedTools:     map[string]struct{}{},
		RemainingBudget: scalePlanBudget(cfg.StructuredCognition.PlanBudget, 2),
	}
	first, err := planner.PlanWithContext(context.Background(), planning)
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	assert.Equal(t, "network_sre", first.Items[0].ExpertName)
	planning.CalledGoalKeys[planGoalKey(first.Items[0])] = struct{}{}

	second, err := planner.PlanWithContext(context.Background(), planning)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.NotEqual(t, first.Items[0].ExpertName, second.Items[0].ExpertName)
	assert.False(t, strings.EqualFold(planGoalKey(first.Items[0]), planGoalKey(second.Items[0])))
}

func TestEnginePlannerDisablesStructuredGenerationAfterFirstFailure(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StructuredCognition.Enabled = true
	calls := 0
	cfg.StructuredGenerate = func(context.Context, string) (string, error) {
		calls++
		return "", context.DeadlineExceeded
	}
	engine := NewGoSEngine(cfg, &testLogger{})
	engine.RegisterExpert("linux_sre", &mockExpert{name: "linux_sre"})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("资源耗尽", 0.5, 1, "检查资源指标")
	frontier := &belief.Frontier{NodeID: hypothesisID, Label: "资源耗尽"}
	history := NewPlanningHistory()
	history.RemainingBudget = scalePlanBudget(cfg.StructuredCognition.PlanBudget, 2)

	first, err := engine.plan(context.Background(), graph, frontier, history)
	require.NoError(t, err)
	assert.Equal(t, "structured_generation_failed", first.FallbackReason)
	require.True(t, history.StructuredGenerationDisabled)

	second, err := engine.plan(context.Background(), graph, frontier, history)
	require.NoError(t, err)
	assert.Equal(t, "structured_generation_disabled", second.FallbackReason)
	assert.Equal(t, 1, calls)
}
