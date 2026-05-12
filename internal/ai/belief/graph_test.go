package belief

import (
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

	assert.Len(t, g.Snapshots, 3)
	assert.Equal(t, "add_node", g.Snapshots[0].Action)
	assert.Equal(t, "add_node", g.Snapshots[1].Action)
	assert.Equal(t, "add_node", g.Snapshots[2].Action)
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
