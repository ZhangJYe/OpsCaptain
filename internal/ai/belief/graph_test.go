package belief

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeliefGraph_AddNode(t *testing.T) {
	g := NewBeliefGraph()

	signalID := g.AddSignal("Service timeout alert")
	assert.NotEmpty(t, signalID)
	assert.Equal(t, NodeSignal, g.Nodes[signalID].Type)
	assert.Equal(t, 0, g.Nodes[signalID].Level)

	evidenceID := g.AddEvidence("CPU usage 95%", nil)
	assert.NotEmpty(t, evidenceID)
	assert.Equal(t, NodeEvidence, g.Nodes[evidenceID].Type)

	hypoID := g.AddHypothesis("Resource exhaustion", 0.8, 1, "High CPU supports this")
	assert.NotEmpty(t, hypoID)
	assert.Equal(t, NodeHypothesis, g.Nodes[hypoID].Type)
	assert.Equal(t, 1, g.Nodes[hypoID].Level)
	assert.Equal(t, 0.8, g.Nodes[hypoID].Score)
}

func TestBeliefGraph_AddEdge(t *testing.T) {
	g := NewBeliefGraph()

	evidenceID := g.AddEvidence("CPU usage 95%", nil)
	hypoID := g.AddHypothesis("Resource exhaustion", 0.8, 1, "High CPU")

	g.AddEdge(evidenceID, hypoID, EdgeSupport, 0.9, "expert_analysis")

	dict := g.ToDict()
	edges := dict["edges"].([]map[string]interface{})
	require.Len(t, edges, 1)
	assert.Equal(t, EdgeType("support"), edges[0]["type"])
	assert.Equal(t, evidenceID, edges[0]["src"])
	assert.Equal(t, hypoID, edges[0]["dst"])
}

func TestBeliefGraph_ExtractFrontier(t *testing.T) {
	g := NewBeliefGraph()

	g.AddHypothesis("Hypothesis A", 0.9, 1, "Strong evidence")
	g.AddHypothesis("Hypothesis B", 0.6, 1, "Some evidence")
	g.AddHypothesis("Hypothesis C", 0.3, 1, "Weak evidence")

	frontier := g.ExtractFrontier(1)
	require.NotNil(t, frontier)
	assert.Equal(t, "Hypothesis A", frontier.Label)
	assert.Equal(t, 0.9, frontier.Score)

	frontier2 := g.ExtractFrontier(2)
	assert.Nil(t, frontier2)
}

func TestBeliefGraph_ExtractFrontier_WithEdges(t *testing.T) {
	g := NewBeliefGraph()

	evidenceID := g.AddEvidence("Evidence 1", nil)
	hypoID := g.AddHypothesis("Hypothesis A", 0.8, 1, "Why")

	g.AddEdge(evidenceID, hypoID, EdgeSupport, 0.9, "test")

	frontier := g.ExtractFrontier(1)
	require.NotNil(t, frontier)
	assert.Equal(t, 1, frontier.Supports)
	assert.Equal(t, 0, frontier.Refutes)
}

func TestBeliefGraph_RetractNode(t *testing.T) {
	g := NewBeliefGraph()

	hypoID := g.AddHypothesis("Old hypothesis", 0.7, 1, "Initial")
	assert.Equal(t, StatusActive, g.Nodes[hypoID].Status)

	g.RetractNode(hypoID, "new-evidence", "Contradicted")
	assert.Equal(t, StatusRetracted, g.Nodes[hypoID].Status)
	assert.Equal(t, "new-evidence", g.Nodes[hypoID].RetractedBy)
}

func TestBeliefGraph_SupersedeNode(t *testing.T) {
	g := NewBeliefGraph()

	oldID := g.AddHypothesis("Old hypothesis", 0.5, 1, "Old")
	newID := g.AddHypothesis("New hypothesis", 0.9, 1, "Better")

	g.SupersedeNode(oldID, newID, "Better evidence")
	assert.Equal(t, StatusSuperseded, g.Nodes[oldID].Status)
	assert.Equal(t, newID, g.Nodes[oldID].SupersededBy)
}

func TestBeliefGraph_UpdateCopyOnWrite(t *testing.T) {
	g := NewBeliefGraph()

	g.AddHypothesis("Initial", 0.5, 1, "Start")

	result := g.UpdateCopyOnWrite(func(cp *BeliefGraph) error {
		cp.AddHypothesisCopy("New hypothesis", 0.8, 1, "Added in transaction")
		return nil
	})

	assert.True(t, result.Committed)
	assert.Len(t, g.Nodes, 2)
}

func TestBeliefGraph_UpdateCopyOnWrite_Rollback(t *testing.T) {
	g := NewBeliefGraph()

	g.AddHypothesis("Initial", 0.5, 1, "Start")
	nodesBefore := len(g.Nodes)

	result := g.UpdateCopyOnWrite(func(cp *BeliefGraph) error {
		cp.AddHypothesisCopy("New hypothesis", 0.8, 1, "Will fail")
		return assert.AnError
	})

	assert.False(t, result.Committed)
	assert.Len(t, g.Nodes, nodesBefore)
}

func TestBeliefGraph_Snapshots(t *testing.T) {
	g := NewBeliefGraph()

	g.AddSignal("Alert")
	g.AddEvidence("Evidence", nil)
	g.AddHypothesis("Hypothesis", 0.8, 1, "Why")

	assert.Len(t, g.Snapshots, 1)
	assert.Len(t, g.Deltas, 2)
	assert.Equal(t, "add_node", g.Snapshots[0].Action)
	assert.Equal(t, "add_node", g.Deltas[0].Action)
	assert.Equal(t, "add_node", g.Deltas[1].Action)
	assert.Len(t, g.Deltas[0].UpsertNodes, 1)
	assert.Len(t, g.Deltas[1].UpsertNodes, 1)
}

func TestBeliefGraph_CheckpointAndDeltaHistoryIsBounded(t *testing.T) {
	g := NewBeliefGraphWithPolicy(GraphPolicy{
		CheckpointInterval: 3,
		MaxNodes:           20,
		MaxEdges:           20,
		MaxDepth:           3,
		MaxSnapshots:       3,
		MaxDeltas:          2,
	})

	g.AddSignal("alert")
	g.AddHypothesis("h1", 0.8, 1, "first")
	g.AddHypothesis("h2", 0.5, 1, "second")
	g.UpdateNode("node-2", 0.9, "updated")
	g.AddEvidence("metric", nil)

	stats := g.ResourceStats()
	assert.Equal(t, 2, stats.Snapshots)
	assert.Equal(t, 1, stats.Deltas)
	assert.Positive(t, stats.HistoryBytes)
	assert.NoError(t, g.ValidateResources())
}

func TestBeliefGraph_CopyOnWriteRejectsResourceLimitWithoutMutation(t *testing.T) {
	g := NewBeliefGraphWithPolicy(GraphPolicy{
		CheckpointInterval: 10,
		MaxNodes:           2,
		MaxEdges:           2,
		MaxDepth:           1,
		MaxSnapshots:       2,
		MaxDeltas:          10,
	})
	g.AddSignal("alert")

	result := g.UpdateCopyOnWrite(func(cp *BeliefGraph) error {
		cp.AddHypothesisCopy("h1", 0.8, 1, "first")
		cp.AddHypothesisCopy("h2", 0.7, 1, "second")
		return nil
	})

	require.False(t, result.Committed)
	var limitErr *GraphResourceLimitError
	require.ErrorAs(t, result.Error, &limitErr)
	assert.Equal(t, "nodes", limitErr.Resource)
	assert.Len(t, g.Nodes, 1)
}

func TestBeliefGraph_CopyOnWriteRejectsDepthLimit(t *testing.T) {
	g := NewBeliefGraphWithPolicy(GraphPolicy{
		CheckpointInterval: 10,
		MaxNodes:           4,
		MaxEdges:           4,
		MaxDepth:           1,
		MaxSnapshots:       2,
		MaxDeltas:          10,
	})
	g.AddSignal("alert")

	result := g.UpdateCopyOnWrite(func(cp *BeliefGraph) error {
		cp.AddHypothesisCopy("too deep", 0.8, 2, "invalid depth")
		return nil
	})

	require.False(t, result.Committed)
	var limitErr *GraphResourceLimitError
	require.ErrorAs(t, result.Error, &limitErr)
	assert.Equal(t, "depth", limitErr.Resource)
	assert.Len(t, g.Nodes, 1)
}

func TestBeliefGraph_CopyOnWriteRejectsSnapshotLimit(t *testing.T) {
	g := NewBeliefGraphWithPolicy(GraphPolicy{
		CheckpointInterval: 1,
		MaxNodes:           4,
		MaxEdges:           4,
		MaxDepth:           2,
		MaxSnapshots:       1,
		MaxDeltas:          1,
	})
	g.AddSignal("alert")

	result := g.UpdateCopyOnWrite(func(cp *BeliefGraph) error {
		cp.AddHypothesisCopy("h1", 0.8, 1, "first")
		return nil
	})

	require.False(t, result.Committed)
	var limitErr *GraphResourceLimitError
	require.ErrorAs(t, result.Error, &limitErr)
	assert.Equal(t, "snapshots", limitErr.Resource)
	assert.Len(t, g.Nodes, 1)
	assert.Len(t, g.Snapshots, 1)
}

func TestBeliefGraph_DirectMutationDoesNotPartiallyApplyAtHistoryLimit(t *testing.T) {
	g := NewBeliefGraphWithPolicy(GraphPolicy{
		CheckpointInterval: 1,
		MaxNodes:           4,
		MaxEdges:           4,
		MaxDepth:           2,
		MaxSnapshots:       1,
		MaxDeltas:          1,
	})
	require.NotEmpty(t, g.AddSignal("alert"))

	assert.Empty(t, g.AddHypothesis("not committed", 0.8, 1, "snapshot cap"))
	assert.Len(t, g.Nodes, 1)
	var limitErr *GraphResourceLimitError
	require.ErrorAs(t, g.ValidateResources(), &limitErr)
	assert.Equal(t, "snapshots", limitErr.Resource)
}

func TestBeliefGraph_TargetScaleHistoryRemainsBounded(t *testing.T) {
	g := NewBeliefGraphWithPolicy(GraphPolicy{
		CheckpointInterval: 10,
		MaxNodes:           128,
		MaxEdges:           256,
		MaxDepth:           3,
		MaxSnapshots:       32,
		MaxDeltas:          10,
	})
	ids := make([]string, 0, 100)
	legacyFullSnapshotBytes := 0
	for index := 0; index < 100; index++ {
		ids = append(ids, g.AddHypothesis("hypothesis", 0.5, 1, "target scale"))
		legacyFullSnapshotBytes += serializedGraphStateBytes(g)
	}
	for index := 1; index < len(ids); index++ {
		g.AddEdge(ids[index-1], ids[index], EdgeCausal, 0.5, "target_scale_test")
		legacyFullSnapshotBytes += serializedGraphStateBytes(g)
	}

	stats := g.ResourceStats()
	assert.Equal(t, 100, stats.Nodes)
	assert.Equal(t, 99, stats.Edges)
	assert.LessOrEqual(t, stats.Snapshots, g.Policy.MaxSnapshots)
	assert.LessOrEqual(t, stats.Deltas, g.Policy.MaxDeltas)
	assert.Less(t, stats.HistoryBytes, 4*1024*1024)
	assert.Less(t, stats.HistoryBytes, legacyFullSnapshotBytes/2)
	t.Logf("checkpoint+delta history=%d bytes, per-mutation full snapshots=%d bytes", stats.HistoryBytes, legacyFullSnapshotBytes)
	assert.NoError(t, g.ValidateResources())
}

func serializedGraphStateBytes(g *BeliefGraph) int {
	encoded, _ := json.Marshal(struct {
		Nodes map[string]*Node `json:"nodes"`
		Edges map[string]*Edge `json:"edges"`
	}{Nodes: g.Nodes, Edges: g.Edges})
	return len(encoded)
}

func TestBeliefGraph_DeepCopy_Independence(t *testing.T) {
	g := NewBeliefGraph()
	g.AddHypothesis("Original", 0.5, 1, "Start")

	cp := g.deepCopy()
	cp.AddHypothesisCopy("Copy only", 0.9, 1, "In copy")

	assert.Len(t, g.Nodes, 1)
	assert.Len(t, cp.Nodes, 2)
}

func TestBeliefFSM_Decide(t *testing.T) {
	g := NewBeliefGraph()
	evidenceID := g.AddEvidence("Evidence", nil)
	hypoID := g.AddHypothesis("H1", 0.9, 1, "Strong")
	g.AddHypothesis("H2", 0.3, 1, "Weak")
	g.AddEdge(evidenceID, hypoID, EdgeSupport, 0.9, "test")

	fsm := NewBeliefFSM(FSMThresholds{
		GapDelta:   0.3,
		MinSupport: 1,
		MaxSteps:   5,
	})

	decision := fsm.Decide(g)
	assert.Equal(t, "report", decision.Action)
}

func TestBeliefFSM_Decide_Continue(t *testing.T) {
	g := NewBeliefGraph()
	g.AddHypothesis("H1", 0.6, 1, "Medium")
	g.AddHypothesis("H2", 0.5, 1, "Medium")

	fsm := NewBeliefFSM(FSMThresholds{
		GapDelta:   0.3,
		MinSupport: 1,
		MaxSteps:   5,
	})

	decision := fsm.Decide(g)
	assert.Equal(t, "continue", decision.Action)
}

func TestBeliefFSM_DrillDown(t *testing.T) {
	fsm := NewBeliefFSM(FSMThresholds{})

	assert.Equal(t, 1, fsm.CurrentLevel)
	assert.Equal(t, StateDrilling, fsm.State)

	fsm.DrillDown("go deeper")
	assert.Equal(t, 2, fsm.CurrentLevel)
	assert.Equal(t, StateDrilling, fsm.State)
	require.Len(t, fsm.History, 1)
	assert.Equal(t, 1, fsm.History[0].FromLevel)
	assert.Equal(t, 2, fsm.History[0].ToLevel)
}

func TestBeliefFSM_BacktrackTo(t *testing.T) {
	fsm := NewBeliefFSM(FSMThresholds{})
	fsm.DrillDown("go deeper")

	require.NoError(t, fsm.BacktrackTo(1, "ancestor changed"))
	assert.Equal(t, 1, fsm.CurrentLevel)
	require.Len(t, fsm.History, 2)
	assert.Equal(t, 2, fsm.History[1].FromLevel)
	assert.Equal(t, 1, fsm.History[1].ToLevel)
	assert.Error(t, fsm.BacktrackTo(1, "invalid"))
}

func TestBeliefFSM_MarkDone(t *testing.T) {
	fsm := NewBeliefFSM(FSMThresholds{})

	fsm.MarkDone("finished")
	assert.True(t, fsm.IsFinalState())
	assert.Equal(t, StateDone, fsm.State)
	assert.Len(t, fsm.History, 1)
}

func TestBeliefFSM_TickStep(t *testing.T) {
	fsm := NewBeliefFSM(FSMThresholds{})

	fsm.TickStep(1)
	fsm.TickStep(2)

	assert.Equal(t, 3, fsm.TotalSteps)
	assert.Equal(t, 3, fsm.LevelSteps[1])
}

func TestBeliefGraph_IsHighestConfInLevel(t *testing.T) {
	g := NewBeliefGraph()

	id1 := g.AddHypothesis("H1", 0.9, 1, "Best")
	g.AddHypothesis("H2", 0.5, 1, "Lower")

	assert.True(t, g.IsHighestConfInLevel(id1))

	id2 := g.AddHypothesis("H3", 0.3, 1, "Lowest")
	assert.False(t, g.IsHighestConfInLevel(id2))
}

func TestSafeWhy(t *testing.T) {
	assert.Equal(t, "", safeWhy(nil))
	assert.Equal(t, "", safeWhy(map[string]interface{}{}))
	assert.Equal(t, "reason", safeWhy(map[string]interface{}{"why": "reason"}))
	assert.Equal(t, "42", safeWhy(map[string]interface{}{"why": 42}))
}

func TestEdgeKey(t *testing.T) {
	assert.Equal(t, "a->b", edgeKey("a", "b"))
	assert.Equal(t, "node-1->node-2", edgeKey("node-1", "node-2"))
}
