package belief

import (
	"crypto/sha256"
	"encoding/json"
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
	Deltas        []GraphDelta
	CurrentStep   int
	Policy        GraphPolicy

	lastCheckpointStep int
	recordedNodeHashes map[string][sha256.Size]byte
	recordedEdgeHashes map[string][sha256.Size]byte
	recordingDisabled  bool
	limitErr           error
}

func NewBeliefGraph() *BeliefGraph {
	return NewBeliefGraphWithPolicy(GraphPolicy{})
}

func NewBeliefGraphWithPolicy(policy GraphPolicy) *BeliefGraph {
	policy = normalizeGraphPolicy(policy)
	return &BeliefGraph{
		Nodes:              make(map[string]*Node),
		Edges:              make(map[string]*Edge),
		LevelNodes:         make(map[int][]LevelNode),
		Policy:             policy,
		recordedNodeHashes: make(map[string][sha256.Size]byte),
		recordedEdgeHashes: make(map[string][sha256.Size]byte),
	}
}

func normalizeGraphPolicy(policy GraphPolicy) GraphPolicy {
	if policy.CheckpointInterval <= 0 {
		policy.CheckpointInterval = 10
	}
	if policy.MaxNodes <= 0 {
		policy.MaxNodes = 256
	}
	if policy.MaxEdges <= 0 {
		policy.MaxEdges = 512
	}
	if policy.MaxDepth <= 0 {
		policy.MaxDepth = 4
	}
	if policy.MaxSnapshots <= 0 {
		policy.MaxSnapshots = 32
	}
	if policy.MaxDeltas <= 0 {
		policy.MaxDeltas = policy.CheckpointInterval
	}
	return policy
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
	g.retractNodeInternal(nodeID, retractedBy, reason)
}

func (g *BeliefGraph) retractNodeInternal(nodeID string, retractedBy string, reason string) {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}
	if !g.prepareMutationInternal(g.CurrentStep + 1) {
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
	g.recordMutationInternal("retract_node")
}

func (g *BeliefGraph) SupersedeNode(oldNodeID string, newNodeID string, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	oldNode, exists := g.Nodes[oldNodeID]
	if !exists {
		return
	}
	if !g.prepareMutationInternal(g.CurrentStep + 1) {
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
	g.recordMutationInternal("supersede_node")
}

func (g *BeliefGraph) GetActiveNodeCopies() []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.getActiveNodeCopiesInternal()
}

func (g *BeliefGraph) GetActiveHypothesisCopies(level int) []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	active := make([]Node, 0)
	for _, node := range g.Nodes {
		if node.Type == NodeHypothesis && node.Status == StatusActive && node.Level == level {
			active = append(active, g.copyNode(node))
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Score == active[j].Score {
			return active[i].ID < active[j].ID
		}
		return active[i].Score > active[j].Score
	})
	return active
}

func (g *BeliefGraph) GetActiveRefinesParentCopies(nodeID string) []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	parents := make([]Node, 0, 1)
	for _, edge := range g.Edges {
		if edge.Dst != nodeID || edge.Type != EdgeRefines || edge.Status != StatusActive {
			continue
		}
		parent := g.Nodes[edge.Src]
		if parent == nil || parent.Type != NodeHypothesis || parent.Status != StatusActive {
			continue
		}
		parents = append(parents, g.copyNode(parent))
	}
	sort.Slice(parents, func(i, j int) bool { return parents[i].ID < parents[j].ID })
	return parents
}

func (g *BeliefGraph) getActiveNodeCopiesInternal() []Node {
	var active []Node
	for _, node := range g.Nodes {
		if node.Status == StatusActive {
			active = append(active, g.copyNode(node))
		}
	}
	return active
}

func (g *BeliefGraph) copyNode(node *Node) Node {
	copied := *node
	if node.Attrs != nil {
		copied.Attrs = make(map[string]interface{}, len(node.Attrs))
		for k, v := range node.Attrs {
			copied.Attrs[k] = v
		}
	}
	if node.Source != nil {
		sourceCopy := *node.Source
		copied.Source = &sourceCopy
	}
	return copied
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

func (g *BeliefGraph) GetActiveEdgeCopies() []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.getActiveEdgeCopies()
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
		nodeCopies[k] = g.copyNode(v)
	}

	return map[string]interface{}{
		"nodes":           nodeCopies,
		"edges":           edgeList,
		"start_signal_id": g.StartSignalID,
		"belief":          g.Belief,
		"resource_stats":  g.resourceStatsInternal(),
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
	if g.limitErr != nil {
		return ""
	}
	if len(g.Nodes)+1 > g.Policy.MaxNodes {
		g.limitErr = &GraphResourceLimitError{Resource: "nodes", Limit: g.Policy.MaxNodes, Actual: len(g.Nodes) + 1}
		return ""
	}
	if level > g.Policy.MaxDepth {
		g.limitErr = &GraphResourceLimitError{Resource: "depth", Limit: g.Policy.MaxDepth, Actual: level}
		return ""
	}
	if !g.prepareMutationInternal(g.CurrentStep + 1) {
		return ""
	}
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
	g.recordMutationInternal("add_node")
	return nodeID
}

func (g *BeliefGraph) updateNodeInternal(nodeID string, newScore float64, newWhy string) {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}
	if !g.prepareMutationInternal(g.CurrentStep + 1) {
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
	g.recordMutationInternal("update_node")
}

func (g *BeliefGraph) addEdgeInternal(src, dst string, edgeType EdgeType, confidence float64, derivationType string) {
	if g.limitErr != nil {
		return
	}
	if _, ok := g.Nodes[src]; !ok {
		return
	}
	if _, ok := g.Nodes[dst]; !ok {
		return
	}
	key := edgeKey(src, dst)
	if _, exists := g.Edges[key]; !exists && len(g.Edges)+1 > g.Policy.MaxEdges {
		g.limitErr = &GraphResourceLimitError{Resource: "edges", Limit: g.Policy.MaxEdges, Actual: len(g.Edges) + 1}
		return
	}
	if !g.prepareMutationInternal(g.CurrentStep + 1) {
		return
	}
	g.CurrentStep++
	g.Edges[key] = &Edge{
		Src:              src,
		Dst:              dst,
		Type:             edgeType,
		Status:           StatusActive,
		Confidence:       confidence,
		DerivationType:   derivationType,
		ExtractorVersion: "v1.0",
		StepID:           fmt.Sprintf("step-%d", g.CurrentStep),
	}
	g.recordMutationInternal("add_edge")
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
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].Score > candidates[j].Score
	})
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
			if g.LevelNodes[lv][i].Confidence == g.LevelNodes[lv][j].Confidence {
				return g.LevelNodes[lv][i].NodeID < g.LevelNodes[lv][j].NodeID
			}
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
	g.Deltas = nil
	g.lastCheckpointStep = g.CurrentStep
	g.refreshRecordedHashesInternal()
}

func (g *BeliefGraph) recordMutationInternal(action string) {
	if g.recordingDisabled || g.limitErr != nil {
		return
	}
	checkpoint := g.lastCheckpointStep == 0 || g.CurrentStep-g.lastCheckpointStep >= g.Policy.CheckpointInterval
	if checkpoint {
		if len(g.Snapshots) >= g.Policy.MaxSnapshots {
			g.limitErr = &GraphResourceLimitError{Resource: "snapshots", Limit: g.Policy.MaxSnapshots, Actual: len(g.Snapshots) + 1}
			return
		}
		g.takeSnapshotInternal(action)
		return
	}
	if len(g.Deltas) >= g.Policy.MaxDeltas {
		g.limitErr = &GraphResourceLimitError{Resource: "deltas", Limit: g.Policy.MaxDeltas, Actual: len(g.Deltas) + 1}
		return
	}
	delta := GraphDelta{
		StepID:      fmt.Sprintf("step-%d", g.CurrentStep),
		Timestamp:   time.Now(),
		Action:      action,
		UpsertNodes: make(map[string]*Node),
		UpsertEdges: make(map[string]*Edge),
	}
	for id, node := range g.Nodes {
		hash := hashGraphValue(node)
		if previous, ok := g.recordedNodeHashes[id]; !ok || previous != hash {
			copy := g.copyNode(node)
			delta.UpsertNodes[id] = &copy
		}
	}
	for id, edge := range g.Edges {
		hash := hashGraphValue(edge)
		if previous, ok := g.recordedEdgeHashes[id]; !ok || previous != hash {
			copy := *edge
			delta.UpsertEdges[id] = &copy
		}
	}
	g.Deltas = append(g.Deltas, delta)
	g.refreshRecordedHashesInternal()
}

func hashGraphValue(value any) [sha256.Size]byte {
	data, _ := json.Marshal(value)
	return sha256.Sum256(data)
}

func (g *BeliefGraph) refreshRecordedHashesInternal() {
	g.recordedNodeHashes = make(map[string][sha256.Size]byte, len(g.Nodes))
	for id, node := range g.Nodes {
		g.recordedNodeHashes[id] = hashGraphValue(node)
	}
	g.recordedEdgeHashes = make(map[string][sha256.Size]byte, len(g.Edges))
	for id, edge := range g.Edges {
		g.recordedEdgeHashes[id] = hashGraphValue(edge)
	}
}

func (g *BeliefGraph) preflightRecordInternal(nextStep int) error {
	if nextStep == g.CurrentStep {
		return nil
	}
	checkpoint := g.lastCheckpointStep == 0 || nextStep-g.lastCheckpointStep >= g.Policy.CheckpointInterval
	if checkpoint && len(g.Snapshots) >= g.Policy.MaxSnapshots {
		return &GraphResourceLimitError{Resource: "snapshots", Limit: g.Policy.MaxSnapshots, Actual: len(g.Snapshots) + 1}
	}
	if !checkpoint && len(g.Deltas) >= g.Policy.MaxDeltas {
		return &GraphResourceLimitError{Resource: "deltas", Limit: g.Policy.MaxDeltas, Actual: len(g.Deltas) + 1}
	}
	return nil
}

func (g *BeliefGraph) prepareMutationInternal(nextStep int) bool {
	if g.limitErr != nil {
		return false
	}
	if g.recordingDisabled {
		return true
	}
	if err := g.preflightRecordInternal(nextStep); err != nil {
		g.limitErr = err
		return false
	}
	return true
}

func (g *BeliefGraph) UpdateCopyOnWrite(fn func(copy *BeliefGraph) error) *GraphUpdateResult {
	g.mu.RLock()
	originalStep := g.CurrentStep
	cp := g.deepCopy()
	g.mu.RUnlock()

	if err := fn(cp); err != nil {
		return &GraphUpdateResult{Committed: false, Error: err}
	}
	if cp.limitErr != nil {
		return &GraphUpdateResult{Committed: false, Error: cp.limitErr}
	}
	if err := cp.validateResourceLimitsInternal(); err != nil {
		return &GraphUpdateResult{Committed: false, Error: err}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.CurrentStep != originalStep {
		return &GraphUpdateResult{Committed: false, Error: fmt.Errorf("belief graph changed during copy-on-write update")}
	}
	if err := g.preflightRecordInternal(cp.CurrentStep); err != nil {
		return &GraphUpdateResult{Committed: false, Error: err}
	}
	g.Nodes = cp.Nodes
	g.Edges = cp.Edges
	g.StartSignalID = cp.StartSignalID
	g.Belief = cp.Belief
	g.CurrentStep = cp.CurrentStep
	g.updateLevelNodesInternal()
	g.recordMutationInternal("copy_on_write_update")
	if g.limitErr != nil {
		return &GraphUpdateResult{Committed: false, Error: g.limitErr}
	}

	return &GraphUpdateResult{Committed: true}
}

func (g *BeliefGraph) deepCopy() *BeliefGraph {
	cp := &BeliefGraph{
		Nodes:             make(map[string]*Node, len(g.Nodes)),
		Edges:             make(map[string]*Edge, len(g.Edges)),
		StartSignalID:     g.StartSignalID,
		Belief:            g.Belief,
		CurrentStep:       g.CurrentStep,
		Policy:            g.Policy,
		recordingDisabled: true,
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

func (g *BeliefGraph) ValidateResources() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.limitErr != nil {
		return g.limitErr
	}
	return g.validateResourceLimitsInternal()
}

func (g *BeliefGraph) validateResourceLimitsInternal() error {
	stats := g.resourceStatsInternal()
	checks := []struct {
		name   string
		actual int
		limit  int
	}{
		{name: "nodes", actual: stats.Nodes, limit: g.Policy.MaxNodes},
		{name: "edges", actual: stats.Edges, limit: g.Policy.MaxEdges},
		{name: "depth", actual: stats.Depth, limit: g.Policy.MaxDepth},
		{name: "snapshots", actual: stats.Snapshots, limit: g.Policy.MaxSnapshots},
		{name: "deltas", actual: stats.Deltas, limit: g.Policy.MaxDeltas},
	}
	for _, check := range checks {
		if check.actual > check.limit {
			return &GraphResourceLimitError{Resource: check.name, Limit: check.limit, Actual: check.actual}
		}
	}
	return nil
}

func (g *BeliefGraph) ResourceStats() GraphResourceStats {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.resourceStatsInternal()
}

func (g *BeliefGraph) resourceStatsInternal() GraphResourceStats {
	depth := 0
	for _, node := range g.Nodes {
		if node.Level > depth {
			depth = node.Level
		}
	}
	history, _ := json.Marshal(struct {
		Snapshots []GraphSnapshot `json:"snapshots"`
		Deltas    []GraphDelta    `json:"deltas"`
	}{Snapshots: g.Snapshots, Deltas: g.Deltas})
	return GraphResourceStats{
		Nodes:        len(g.Nodes),
		Edges:        len(g.Edges),
		Depth:        depth,
		Snapshots:    len(g.Snapshots),
		Deltas:       len(g.Deltas),
		HistoryBytes: len(history),
	}
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

func (g *BeliefGraph) RetractNodeCopy(nodeID string, retractedBy string, reason string) {
	g.retractNodeInternal(nodeID, retractedBy, reason)
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
