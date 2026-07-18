package gos_engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/belief"
)

func phase2TestConfig() *Config {
	cfg := DefaultConfig()
	cfg.StateConversion.Enabled = true
	cfg.StateConversion.MaxDepth = 2
	cfg.StateConversion.TieEpsilon = 0.01
	cfg.FSM.GapDelta = 0.2
	cfg.FSM.MinConfidence = 0.7
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 3
	cfg.SessionMaxSteps = 5
	return cfg
}

func addTestSupport(graph *belief.BeliefGraph, targetID string, strength float64) string {
	sourceID := fmt.Sprintf("support-%d", len(graph.Nodes)+1)
	source := &belief.EvidenceSource{
		SourceType:         "metric",
		SourceID:           sourceID,
		Timestamp:          time.Now().UTC(),
		SummarySnippet:     "supporting observation",
		Relation:           "support",
		TargetHypothesisID: targetID,
		Strength:           strength,
	}
	evidenceID := graph.AddNode(belief.NodeEvidence, "support", strength, 0, map[string]interface{}{
		"score":                strength,
		"relation":             "support",
		"target_hypothesis_id": targetID,
		"strength":             strength,
		"dedup_key":            strings.Join([]string{"metric", sourceID, targetID}, "\x1f"),
	}, source)
	graph.AddEdge(evidenceID, targetID, belief.EdgeSupport, strength, "test")
	return evidenceID
}

func TestStateConverterDecisionTable(t *testing.T) {
	t.Run("continue when evidence is insufficient", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
		graph.AddHypothesis("网络问题", 0.2, 1, "alternative")
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})

		assert.Equal(t, DecisionContinue, decision.Action)
		assert.Equal(t, "insufficient_evidence", decision.ReasonCode)
	})

	t.Run("refine supported but coarse hypothesis", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		parentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
		graph.AddHypothesis("网络问题", 0.2, 1, "alternative")
		addTestSupport(graph, parentID, 0.9)
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})

		assert.Equal(t, DecisionRefine, decision.Action)
		assert.Equal(t, 2, decision.ToLevel)
		assert.NotEmpty(t, decision.Candidates)
	})

	t.Run("reuse validated graph proposal children", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		parentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
		graph.AddHypothesis("网络问题", 0.2, 1, "alternative")
		addTestSupport(graph, parentID, 0.9)
		childID := graph.AddNode(belief.NodeHypothesis, "CPU 饱和", 0.85, 2, map[string]interface{}{
			"why":        "CPU 使用率持续高位",
			"actionable": true,
		}, nil)
		graph.AddEdge(parentID, childID, belief.EdgeRefines, 0.85, "expert_graph_proposal_v1")
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
		converter := NewStateConverter(cfg)
		beforeNodes := len(graph.Nodes)

		decision := converter.Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})

		require.Equal(t, DecisionRefine, decision.Action)
		assert.Equal(t, "validated_graph_proposal", decision.ReasonCode)
		require.Len(t, decision.Candidates, 1)
		assert.Equal(t, "CPU 饱和", decision.Candidates[0].Label)
		require.NoError(t, converter.Apply(graph, fsm, decision))
		assert.Equal(t, 2, fsm.GetCurrentLevel())
		assert.Len(t, graph.Nodes, beforeNodes)
		assert.Equal(t, childID, graph.ExtractFrontier(2).NodeID)
	})

	t.Run("report actionable root cause", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		hypothesisID := graph.AddNode(belief.NodeHypothesis, "CPU 饱和", 0.85, 1, map[string]interface{}{
			"why":        "cpu saturated",
			"actionable": true,
		}, nil)
		addTestSupport(graph, hypothesisID, 0.9)
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 5, MaxSteps: 5})

		assert.Equal(t, DecisionReport, decision.Action)
		assert.Equal(t, "actionable_root_cause", decision.ReasonCode)
	})

	t.Run("report evidence-dominant root cause despite a small score gap", func(t *testing.T) {
		cfg := phase2TestConfig()
		cfg.FSM.MinSupport = 2
		graph := belief.NewBeliefGraph()
		hypothesisID := graph.AddNode(belief.NodeHypothesis, "重试风暴", 0.85, 1, map[string]interface{}{
			"why":        "日志和指标均显示请求放大",
			"actionable": true,
		}, nil)
		graph.AddHypothesis("上游容量不足", 0.75, 1, "alternative")
		addTestSupport(graph, hypothesisID, 0.9)
		addTestSupport(graph, hypothesisID, 0.8)
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 2, MaxSteps: 5})

		assert.Equal(t, DecisionReport, decision.Action)
		assert.Equal(t, "actionable_root_cause", decision.ReasonCode)
	})

	t.Run("keep exploring when a close competitor also has support", func(t *testing.T) {
		cfg := phase2TestConfig()
		cfg.FSM.MinSupport = 2
		graph := belief.NewBeliefGraph()
		hypothesisID := graph.AddNode(belief.NodeHypothesis, "重试风暴", 0.85, 1, map[string]interface{}{
			"why":        "日志和指标均显示请求放大",
			"actionable": true,
		}, nil)
		competitorID := graph.AddHypothesis("上游容量不足", 0.75, 1, "alternative")
		addTestSupport(graph, hypothesisID, 0.9)
		addTestSupport(graph, hypothesisID, 0.8)
		addTestSupport(graph, competitorID, 0.7)
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 2, MaxSteps: 5})

		assert.Equal(t, DecisionContinue, decision.Action)
		assert.Equal(t, "ambiguous_frontier", decision.ReasonCode)
	})

	t.Run("degrade when budget is exhausted", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 5, MaxSteps: 5})

		assert.Equal(t, DecisionDegraded, decision.Action)
		assert.Equal(t, "budget_exhausted", decision.ReasonCode)
	})

	t.Run("degrade when no candidate exists", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{MaxSteps: 5})

		assert.Equal(t, DecisionDegraded, decision.Action)
		assert.Equal(t, "no_candidates", decision.ReasonCode)
	})

	t.Run("single supported candidate may refine", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		parentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
		addTestSupport(graph, parentID, 0.9)
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})

		assert.Equal(t, DecisionRefine, decision.Action)
	})

	t.Run("continue on deterministic score tie", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		firstID := graph.AddHypothesis("资源耗尽", 0.8, 1, "first")
		graph.AddHypothesis("网络问题", 0.8, 1, "second")
		addTestSupport(graph, firstID, 0.9)
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})

		assert.Equal(t, DecisionContinue, decision.Action)
		assert.Equal(t, "score_tie", decision.ReasonCode)
		assert.Equal(t, firstID, decision.FrontierID)
	})

	t.Run("degrade when tie survives level limit", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		graph.AddHypothesis("资源耗尽", 0.8, 1, "first")
		graph.AddHypothesis("网络问题", 0.8, 1, "second")
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
		fsm.TickStep(cfg.FSM.MaxSteps)

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: cfg.FSM.MaxSteps, MaxSteps: 5})

		assert.Equal(t, DecisionDegraded, decision.Action)
		assert.Equal(t, "score_tie", decision.ReasonCode)
	})

	t.Run("degrade at max depth without actionable granularity", func(t *testing.T) {
		cfg := phase2TestConfig()
		graph := belief.NewBeliefGraph()
		parentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
		childID := graph.AddHypothesis("仍然粗粒度", 0.8, 2, "not actionable")
		graph.AddEdge(parentID, childID, belief.EdgeRefines, 0.8, "test")
		addTestSupport(graph, childID, 0.9)
		fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
		fsm.DrillDown("test setup")

		decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})

		assert.Equal(t, DecisionDegraded, decision.Action)
		assert.Equal(t, "max_depth_without_actionable_root_cause", decision.ReasonCode)
	})
}

func TestStateConverterRefineIsAtomic(t *testing.T) {
	cfg := phase2TestConfig()
	graph := belief.NewBeliefGraph()
	parentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
	graph.AddHypothesis("网络问题", 0.2, 1, "alternative")
	addTestSupport(graph, parentID, 0.9)
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
	converter := NewStateConverter(cfg)
	decision := converter.Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})

	require.Equal(t, DecisionRefine, decision.Action)
	require.NoError(t, converter.Apply(graph, fsm, decision))
	assert.Equal(t, 2, fsm.GetCurrentLevel())
	require.NotNil(t, graph.ExtractFrontier(2))

	children := graph.GetActiveHypothesisCopies(2)
	assert.Len(t, children, len(decision.Candidates))
	for _, child := range children {
		actionable, _ := child.Attrs["actionable"].(bool)
		assert.True(t, actionable)
	}
}

func TestStateConverterRefineRollbackKeepsFSMAndGraphUnchanged(t *testing.T) {
	cfg := phase2TestConfig()
	graph := belief.NewBeliefGraph()
	parentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
	nodesBefore := len(graph.Nodes)
	converter := NewStateConverter(cfg)

	err := converter.Apply(graph, fsm, StateDecision{
		Action:     DecisionRefine,
		ReasonCode: "test",
		Reason:     "invalid child must roll back",
		FromLevel:  1,
		ToLevel:    2,
		FrontierID: parentID,
		Candidates: []RefinementCandidate{{Label: "invalid", Score: 1.5, Actionable: true}},
	})

	assert.Error(t, err)
	assert.Equal(t, 1, fsm.GetCurrentLevel())
	assert.Len(t, graph.Nodes, nodesBefore)
	assert.Nil(t, graph.ExtractFrontier(2))
}

func TestStateConverterBacktracksInvalidAncestorBranch(t *testing.T) {
	cfg := phase2TestConfig()
	graph := belief.NewBeliefGraph()
	oldParentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "initial best")
	newParentID := graph.AddHypothesis("网络问题", 0.7, 1, "alternative")
	childID := graph.AddNode(belief.NodeHypothesis, "CPU 饱和", 0.8, 2, map[string]interface{}{
		"why":          "cpu saturated",
		"actionable":   true,
		"refined_from": oldParentID,
	}, nil)
	graph.AddEdge(oldParentID, childID, belief.EdgeRefines, 0.8, "test")
	evidenceID := addTestSupport(graph, childID, 0.9)
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
	fsm.DrillDown("test setup")
	graph.UpdateNode(oldParentID, 0.2, "refuted")
	graph.UpdateNode(newParentID, 0.9, "now best")
	converter := NewStateConverter(cfg)

	decision := converter.Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})
	require.Equal(t, DecisionBacktrack, decision.Action)
	require.Equal(t, oldParentID, decision.BranchRootID)
	require.NoError(t, converter.Apply(graph, fsm, decision))

	assert.Equal(t, 1, fsm.GetCurrentLevel())
	assert.Equal(t, newParentID, graph.ExtractFrontier(1).NodeID)
	assert.Equal(t, belief.StatusRetracted, graph.Nodes[childID].Status)
	assert.Equal(t, belief.StatusRetracted, graph.Nodes[evidenceID].Status)
	assert.Equal(t, belief.StatusActive, graph.Nodes[oldParentID].Status)
	require.Len(t, fsm.History, 2)
	assert.Equal(t, 2, fsm.History[1].FromLevel)
	assert.Equal(t, 1, fsm.History[1].ToLevel)
	require.NotEmpty(t, graph.Snapshots)
	require.NotEmpty(t, graph.Deltas)
	assert.Equal(t, belief.StatusRetracted, graph.Deltas[len(graph.Deltas)-1].UpsertNodes[childID].Status)
}

func TestStateConverterRejectsMissingRefinesAncestry(t *testing.T) {
	cfg := phase2TestConfig()
	graph := belief.NewBeliefGraph()
	graph.AddNode(belief.NodeHypothesis, "orphan", 0.8, 2, map[string]interface{}{"actionable": true}, nil)
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
	fsm.DrillDown("test setup")

	decision := NewStateConverter(cfg).Decide(graph, fsm, ConversionBudget{UsedSteps: 1, MaxSteps: 5})

	assert.Equal(t, DecisionDegraded, decision.Action)
	assert.Equal(t, "invalid_refines_ancestry", decision.ReasonCode)
}

func TestStateConverterRejectsIllegalTransitions(t *testing.T) {
	cfg := phase2TestConfig()
	graph := belief.NewBeliefGraph()
	parentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "coarse")
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
	converter := NewStateConverter(cfg)

	tests := []StateDecision{
		{Action: DecisionContinue, FromLevel: 1, ToLevel: 2},
		{Action: DecisionRefine, FromLevel: 1, ToLevel: 3, FrontierID: parentID, Candidates: []RefinementCandidate{{Label: "child", Score: 0.5}}},
		{Action: DecisionBacktrack, FromLevel: 1, ToLevel: 1, BranchRootID: parentID},
		{Action: "unknown", FromLevel: 1, ToLevel: 1},
	}
	for _, decision := range tests {
		assert.Error(t, converter.Apply(graph, fsm, decision), decision.Action)
		assert.Equal(t, 1, fsm.GetCurrentLevel())
	}
}

func TestStateConverterRejectsBacktrackBeforeGraphMutationWhenFSMIsNotDrilling(t *testing.T) {
	cfg := phase2TestConfig()
	graph := belief.NewBeliefGraph()
	parentID := graph.AddHypothesis("资源耗尽", 0.8, 1, "parent")
	childID := graph.AddNode(belief.NodeHypothesis, "CPU 饱和", 0.8, 2, map[string]interface{}{"actionable": true}, nil)
	graph.AddEdge(parentID, childID, belief.EdgeRefines, 0.8, "test")
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())
	fsm.DrillDown("setup")
	fsm.TransitionTo(belief.StateReporting, "setup")

	err := NewStateConverter(cfg).Apply(graph, fsm, StateDecision{
		Action:       DecisionBacktrack,
		FromLevel:    2,
		ToLevel:      1,
		BranchRootID: parentID,
		Reason:       "must not mutate",
	})

	assert.Error(t, err)
	assert.Equal(t, belief.StatusActive, graph.Nodes[childID].Status)
	assert.Equal(t, 2, fsm.GetCurrentLevel())
}
