package belief

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type BeliefGraph struct {
	mu            sync.RWMutex
	Nodes         map[string]*Node
	Edges         map[string]*Edge
	StartSignalID string
	Belief        string
	LevelNodes    map[int][]LevelNode
	Snapshots     []GraphSnapshot
	CurrentStep   int
}

func NewBeliefGraph() *BeliefGraph {
	return &BeliefGraph{
		Nodes:      make(map[string]*Node),
		Edges:      make(map[string]*Edge),
		LevelNodes: make(map[int][]LevelNode),
	}
}

func edgeKey(src, dst string) string {
	return src + "->" + dst
}

func safeWhy(attrs map[string]interface{}) string {
	if attrs == nil {
		return ""
	}
	v, ok := attrs["why"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

func (g *BeliefGraph) AddNode(nodeType NodeType, label string, score float64, level int, attrs map[string]interface{}, source *EvidenceSource) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.addNodeInternal(nodeType, label, score, level, attrs, source)
}

func (g *BeliefGraph) AddSignal(label string) string {
	return g.AddNode(NodeSignal, label, 1.0, 0, nil, nil)
}

func (g *BeliefGraph) AddEvidence(label string, source *EvidenceSource) string {
	return g.AddNode(NodeEvidence, label, 1.0, 0, nil, source)
}

func (g *BeliefGraph) AddHypothesis(label string, score float64, level int, why string) string {
	return g.AddNode(NodeHypothesis, label, score, level, map[string]interface{}{"why": why}, nil)
}

func (g *BeliefGraph) UpdateNode(nodeID string, newScore float64, newWhy string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.updateNodeInternal(nodeID, newScore, newWhy)
}

func (g *BeliefGraph) AddEdge(src, dst string, edgeType EdgeType, confidence float64, derivationType string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.addEdgeInternal(src, dst, edgeType, confidence, derivationType)
}

func (g *BeliefGraph) ExtractFrontier(level int) *Frontier {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.extractFrontierInternal(level)
}

func (g *BeliefGraph) RetractNode(nodeID string, retractedBy string, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}

	g.CurrentStep++
	now := time.Now()
	node.Status = StatusRetracted
	node.RetractedBy = retractedBy
	node.RetractedAt = &now
	if node.Attrs == nil {
		node.Attrs = make(map[string]interface{})
	}
	node.Attrs["retract_reason"] = reason

	for _, edge := range g.Edges {
		if (edge.Src == nodeID || edge.Dst == nodeID) && edge.Status == StatusActive {
			edge.Status = StatusRetracted
			edge.RetractedBy = retractedBy
			edge.RetractedAt = &now
		}
	}

	g.updateLevelNodesInternal()
	g.takeSnapshotInternal("retract_node")
}

func (g *BeliefGraph) SupersedeNode(oldNodeID string, newNodeID string, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	oldNode, exists := g.Nodes[oldNodeID]
	if !exists {
		return
	}

	g.CurrentStep++
	oldNode.Status = StatusSuperseded
	oldNode.SupersededBy = newNodeID
	if oldNode.Attrs == nil {
		oldNode.Attrs = make(map[string]interface{})
	}
	oldNode.Attrs["supersede_reason"] = reason

	g.updateLevelNodesInternal()
	g.takeSnapshotInternal("supersede_node")
}

func (g *BeliefGraph) GetActiveNodeCopies() []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.getActiveNodeCopiesInternal()
}

func (g *BeliefGraph) getActiveNodeCopiesInternal() []Node {
	var active []Node
	for _, node := range g.Nodes {
		if node.Status == StatusActive {
			active = append(active, *node)
		}
	}
	return active
}

func (g *BeliefGraph) getActiveEdgeCopies() []Edge {
	var active []Edge
	for _, edge := range g.Edges {
		if edge.Status == StatusActive {
			active = append(active, *edge)
		}
	}
	return active
}

func (g *BeliefGraph) ToDict() map[string]interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.toDictInternal()
}

func (g *BeliefGraph) toDictInternal() map[string]interface{} {
	edgeList := make([]map[string]interface{}, 0, len(g.Edges))
	for _, e := range g.Edges {
		edgeList = append(edgeList, map[string]interface{}{
			"src":  e.Src,
			"dst":  e.Dst,
			"type": e.Type,
		})
	}

	nodeCopies := make(map[string]Node, len(g.Nodes))
	for k, v := range g.Nodes {
		nodeCopies[k] = *v
	}

	return map[string]interface{}{
		"nodes":           nodeCopies,
		"edges":           edgeList,
		"start_signal_id": g.StartSignalID,
		"belief":          g.Belief,
	}
}

func (g *BeliefGraph) GenerateBeliefText() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.generateBeliefTextInternal()
}

func (g *BeliefGraph) generateBeliefTextInternal() {
	var parts []string
	for _, n := range g.Nodes {
		if n.Status != StatusActive {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s (score=%.2f)", n.Type, n.Label, n.Score))
	}

	if len(parts) > 0 {
		g.Belief = fmt.Sprintf("Current belief state: %d active nodes. %v", len(parts), parts)
	} else {
		g.Belief = "No active nodes in belief graph."
	}
}

func (g *BeliefGraph) addNodeInternal(nodeType NodeType, label string, score float64, level int, attrs map[string]interface{}, source *EvidenceSource) string {
	g.CurrentStep++
	nodeID := fmt.Sprintf("node-%d", g.CurrentStep)
	if attrs == nil {
		attrs = make(map[string]interface{})
	}
	g.Nodes[nodeID] = &Node{
		ID:     nodeID,
		Type:   nodeType,
		Label:  label,
		Score:  score,
		Status: StatusActive,
		Level:  level,
		Attrs:  attrs,
		Source: source,
		StepID: fmt.Sprintf("step-%d", g.CurrentStep),
	}
	g.updateLevelNodesInternal()
	g.takeSnapshotInternal("add_node")
	return nodeID
}

func (g *BeliefGraph) updateNodeInternal(nodeID string, newScore float64, newWhy string) {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}
	g.CurrentStep++
	node.Score = newScore
	if node.Attrs == nil {
		node.Attrs = make(map[string]interface{})
	}
	node.Attrs["why"] = newWhy
	node.Attrs["updated_at"] = time.Now()
	g.updateLevelNodesInternal()
	g.takeSnapshotInternal("update_node")
}

func (g *BeliefGraph) addEdgeInternal(src, dst string, edgeType EdgeType, confidence float64, derivationType string) {
	if _, ok := g.Nodes[src]; !ok {
		return
	}
	if _, ok := g.Nodes[dst]; !ok {
		return
	}
	g.CurrentStep++
	g.Edges[edgeKey(src, dst)] = &Edge{
		Src:              src,
		Dst:              dst,
		Type:             edgeType,
		Status:           StatusActive,
		Confidence:       confidence,
		DerivationType:   derivationType,
		ExtractorVersion: "v1.0",
		StepID:           fmt.Sprintf("step-%d", g.CurrentStep),
	}
	g.takeSnapshotInternal("add_edge")
}

func (g *BeliefGraph) extractFrontierInternal(level int) *Frontier {
	var candidates []*Node
	for _, n := range g.Nodes {
		if n.Type == NodeHypothesis && n.Status == StatusActive && n.Level == level {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	top := candidates[0]
	sup, ref := 0, 0
	for _, e := range g.Edges {
		if e.Dst == top.ID && e.Status == StatusActive {
			if e.Type == EdgeSupport {
				sup++
			}
			if e.Type == EdgeRefute {
				ref++
			}
		}
	}
	return &Frontier{
		NodeID:   top.ID,
		Label:    top.Label,
		Why:      safeWhy(top.Attrs),
		Score:    top.Score,
		Level:    top.Level,
		Supports: sup,
		Refutes:  ref,
	}
}

func (g *BeliefGraph) updateLevelNodesInternal() {
	g.LevelNodes = make(map[int][]LevelNode)
	for _, n := range g.Nodes {
		if n.Type == NodeHypothesis && n.Status == StatusActive {
			g.LevelNodes[n.Level] = append(g.LevelNodes[n.Level], LevelNode{NodeID: n.ID, Confidence: n.Score})
		}
	}
	for lv := range g.LevelNodes {
		sort.Slice(g.LevelNodes[lv], func(i, j int) bool {
			return g.LevelNodes[lv][i].Confidence > g.LevelNodes[lv][j].Confidence
		})
	}
}

func (g *BeliefGraph) takeSnapshotInternal(action string) {
	snap := GraphSnapshot{
		StepID:    fmt.Sprintf("step-%d", g.CurrentStep),
		Timestamp: time.Now(),
		Action:    action,
		Nodes:     make(map[string]*Node, len(g.Nodes)),
		Edges:     make(map[string]*Edge, len(g.Edges)),
	}
	for k, v := range g.Nodes {
		c := *v
		if v.Attrs != nil {
			c.Attrs = make(map[string]interface{}, len(v.Attrs))
			for ak, av := range v.Attrs {
				c.Attrs[ak] = av
			}
		}
		if v.Source != nil {
			sc := *v.Source
			c.Source = &sc
		}
		snap.Nodes[k] = &c
	}
	for k, v := range g.Edges {
		c := *v
		snap.Edges[k] = &c
	}
	g.Snapshots = append(g.Snapshots, snap)
}

func (g *BeliefGraph) UpdateCopyOnWrite(fn func(copy *BeliefGraph) error) *GraphUpdateResult {
	g.mu.RLock()
	cp := g.deepCopy()
	g.mu.RUnlock()

	if err := fn(cp); err != nil {
		return &GraphUpdateResult{Committed: false, Error: err}
	}

	g.mu.Lock()
	g.Nodes = cp.Nodes
	g.Edges = cp.Edges
	g.CurrentStep = cp.CurrentStep
	g.updateLevelNodesInternal()
	g.takeSnapshotInternal("copy_on_write_update")
	g.mu.Unlock()

	return &GraphUpdateResult{Committed: true}
}

func (g *BeliefGraph) deepCopy() *BeliefGraph {
	cp := &BeliefGraph{
		Nodes:         make(map[string]*Node, len(g.Nodes)),
		Edges:         make(map[string]*Edge, len(g.Edges)),
		StartSignalID: g.StartSignalID,
		Belief:        g.Belief,
		CurrentStep:   g.CurrentStep,
	}
	for k, v := range g.Nodes {
		n := *v
		if v.Attrs != nil {
			n.Attrs = make(map[string]interface{}, len(v.Attrs))
			for ak, av := range v.Attrs {
				n.Attrs[ak] = av
			}
		}
		if v.Source != nil {
			sc := *v.Source
			n.Source = &sc
		}
		cp.Nodes[k] = &n
	}
	for k, v := range g.Edges {
		e := *v
		cp.Edges[k] = &e
	}
	return cp
}

func (g *BeliefGraph) AddNodeCopy(nodeType NodeType, label string, score float64, level int, attrs map[string]interface{}, source *EvidenceSource) string {
	return g.addNodeInternal(nodeType, label, score, level, attrs, source)
}

func (g *BeliefGraph) AddEvidenceCopy(label string, source *EvidenceSource) string {
	return g.addNodeInternal(NodeEvidence, label, 1.0, 0, nil, source)
}

func (g *BeliefGraph) AddHypothesisCopy(label string, score float64, level int, why string) string {
	return g.addNodeInternal(NodeHypothesis, label, score, level, map[string]interface{}{"why": why}, nil)
}

func (g *BeliefGraph) UpdateNodeCopy(nodeID string, newScore float64, newWhy string) {
	g.updateNodeInternal(nodeID, newScore, newWhy)
}

func (g *BeliefGraph) AddEdgeCopy(src, dst string, edgeType EdgeType, confidence float64, derivationType string) {
	g.addEdgeInternal(src, dst, edgeType, confidence, derivationType)
}

func (g *BeliefGraph) HasNode(nodeID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.Nodes[nodeID]
	return exists
}

func (g *BeliefGraph) IsHighestConfInLevel(nodeID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.Nodes[nodeID]
	if !exists || node.Type != NodeHypothesis || node.Status != StatusActive {
		return false
	}

	level := node.Level
	levelNodes, ok := g.LevelNodes[level]
	if !ok || len(levelNodes) == 0 {
		return false
	}

	return levelNodes[0].NodeID == nodeID
}
