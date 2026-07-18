package gos_engine

import (
	"fmt"
	"sort"
	"strings"

	"SuperBizAgent/internal/ai/belief"
)

type DecisionAction string

const (
	DecisionContinue  DecisionAction = "continue"
	DecisionRefine    DecisionAction = "refine"
	DecisionBacktrack DecisionAction = "backtrack"
	DecisionReport    DecisionAction = "report"
	DecisionDegraded  DecisionAction = "degraded"
)

type ConversionBudget struct {
	UsedSteps int `json:"used_steps"`
	MaxSteps  int `json:"max_steps"`
}

type StateDecision struct {
	Action       DecisionAction        `json:"action"`
	ReasonCode   string                `json:"reason_code"`
	Reason       string                `json:"reason"`
	FromLevel    int                   `json:"from_level"`
	ToLevel      int                   `json:"to_level"`
	FrontierID   string                `json:"frontier_id,omitempty"`
	BranchRootID string                `json:"branch_root_id,omitempty"`
	Candidates   []RefinementCandidate `json:"candidates,omitempty"`
}

type StateConverter struct {
	cfg *Config
}

func NewStateConverter(cfg *Config) *StateConverter {
	return &StateConverter{cfg: cfg}
}

func (c *StateConverter) Decide(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, budget ConversionBudget) StateDecision {
	if err := c.validateInputs(graph, fsm, budget); err != nil {
		return c.degraded(fsm, "invalid_state_conversion_input", err.Error(), "")
	}

	level := fsm.GetCurrentLevel()
	candidates := graph.GetActiveHypothesisCopies(level)
	if len(candidates) == 0 {
		return c.degraded(fsm, "no_candidates", fmt.Sprintf("no active hypotheses at level %d", level), "")
	}
	frontier := graph.ExtractFrontier(level)
	if frontier == nil {
		return c.degraded(fsm, "no_frontier", fmt.Sprintf("cannot select frontier at level %d", level), "")
	}

	backtrack, err := c.checkBacktrack(graph, fsm, frontier)
	if err != nil {
		return c.degraded(fsm, "invalid_refines_ancestry", err.Error(), frontier.NodeID)
	}
	if backtrack != nil {
		if budget.UsedSteps >= budget.MaxSteps {
			return c.degraded(fsm, "budget_exhausted", "budget exhausted before invalid branch could be re-evaluated", frontier.NodeID)
		}
		return *backtrack
	}

	gap := 1.0
	if len(candidates) > 1 {
		gap = candidates[0].Score - candidates[1].Score
	}
	tied := len(candidates) > 1 && gap <= c.cfg.StateConversion.TieEpsilon
	sufficient := frontier.Score >= c.cfg.FSM.MinConfidence && frontier.Supports >= c.cfg.FSM.MinSupport
	evidenceDominant := !tied && sufficient && supportLead(graph, candidates, frontier.NodeID) >= c.cfg.FSM.MinSupport
	ambiguous := tied || len(candidates) > 1 && gap < c.cfg.FSM.GapDelta && !evidenceDominant
	granular := c.checkGranularity(graph, frontier)

	if sufficient && granular && !ambiguous {
		return StateDecision{
			Action:     DecisionReport,
			ReasonCode: "actionable_root_cause",
			Reason:     "frontier is evidence-backed and actionable",
			FromLevel:  level,
			ToLevel:    level,
			FrontierID: frontier.NodeID,
		}
	}
	if budget.UsedSteps >= budget.MaxSteps {
		return c.degraded(fsm, "budget_exhausted", fmt.Sprintf("step budget %d exhausted", budget.MaxSteps), frontier.NodeID)
	}
	if ambiguous {
		reasonCode := "ambiguous_frontier"
		if tied {
			reasonCode = "score_tie"
		}
		if fsm.LevelSteps[level] >= c.cfg.FSM.MaxSteps {
			return c.degraded(fsm, reasonCode, fmt.Sprintf("candidate gap %.4f remains below %.4f", gap, c.cfg.FSM.GapDelta), frontier.NodeID)
		}
		return StateDecision{
			Action:     DecisionContinue,
			ReasonCode: reasonCode,
			Reason:     fmt.Sprintf("candidate gap %.4f is below %.4f", gap, c.cfg.FSM.GapDelta),
			FromLevel:  level,
			ToLevel:    level,
			FrontierID: frontier.NodeID,
		}
	}
	if sufficient && !granular {
		if level >= c.cfg.StateConversion.MaxDepth {
			return c.degraded(fsm, "max_depth_without_actionable_root_cause", fmt.Sprintf("level %d reached max depth without actionable granularity", level), frontier.NodeID)
		}
		if existing := activeRefinesChildren(graph, frontier.NodeID, level+1); len(existing) > 0 {
			return StateDecision{
				Action:     DecisionRefine,
				ReasonCode: "validated_graph_proposal",
				Reason:     "validated graph proposal already contains active refinement candidates",
				FromLevel:  level,
				ToLevel:    level + 1,
				FrontierID: frontier.NodeID,
				Candidates: refinementCandidatesFromNodes(existing),
			}
		}
		rule, ok := c.refinementRule(frontier.Label)
		if !ok {
			return c.degraded(fsm, "no_refinement_rule", fmt.Sprintf("no refinement rule for %q", frontier.Label), frontier.NodeID)
		}
		return StateDecision{
			Action:     DecisionRefine,
			ReasonCode: "coarse_hypothesis",
			Reason:     "frontier is supported but requires a more actionable hypothesis",
			FromLevel:  level,
			ToLevel:    level + 1,
			FrontierID: frontier.NodeID,
			Candidates: append([]RefinementCandidate(nil), rule.Children...),
		}
	}
	if fsm.LevelSteps[level] >= c.cfg.FSM.MaxSteps {
		return c.degraded(fsm, "level_step_limit", fmt.Sprintf("level %d exhausted %d steps without sufficient evidence", level, c.cfg.FSM.MaxSteps), frontier.NodeID)
	}
	return StateDecision{
		Action:     DecisionContinue,
		ReasonCode: "insufficient_evidence",
		Reason:     "frontier needs more evidence",
		FromLevel:  level,
		ToLevel:    level,
		FrontierID: frontier.NodeID,
	}
}

func supportLead(graph *belief.BeliefGraph, candidates []belief.Node, frontierID string) int {
	if graph == nil || len(candidates) < 2 {
		return 0
	}
	supports := make(map[string]int, len(candidates))
	for _, edge := range graph.GetActiveEdgeCopies() {
		if edge.Type == belief.EdgeSupport {
			supports[edge.Dst]++
		}
	}
	maxCompetingSupports := 0
	for _, candidate := range candidates {
		if candidate.ID != frontierID && supports[candidate.ID] > maxCompetingSupports {
			maxCompetingSupports = supports[candidate.ID]
		}
	}
	return supports[frontierID] - maxCompetingSupports
}

func (c *StateConverter) Apply(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, decision StateDecision) error {
	if err := c.validateDecision(fsm, decision); err != nil {
		return err
	}

	switch decision.Action {
	case DecisionContinue:
		return nil
	case DecisionRefine:
		return c.refineHypothesis(graph, fsm, decision)
	case DecisionBacktrack:
		return c.backtrack(graph, fsm, decision)
	case DecisionReport, DecisionDegraded:
		fsm.MarkDone(decision.ReasonCode + ": " + decision.Reason)
		return nil
	default:
		return fmt.Errorf("unsupported state decision %q", decision.Action)
	}
}

func (c *StateConverter) validateInputs(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, budget ConversionBudget) error {
	if c == nil || c.cfg == nil {
		return fmt.Errorf("state conversion config is required")
	}
	if graph == nil || fsm == nil {
		return fmt.Errorf("graph and fsm are required")
	}
	if fsm.IsFinalState() {
		return fmt.Errorf("fsm is already final")
	}
	if budget.MaxSteps <= 0 || budget.UsedSteps < 0 {
		return fmt.Errorf("invalid conversion budget")
	}
	if c.cfg.StateConversion.MaxDepth < 1 {
		return fmt.Errorf("state_conversion.max_depth must be positive")
	}
	if c.cfg.StateConversion.TieEpsilon < 0 || c.cfg.StateConversion.TieEpsilon > 1 {
		return fmt.Errorf("state_conversion.tie_epsilon must be within [0,1]")
	}
	if c.cfg.FSM.GapDelta < 0 || c.cfg.FSM.GapDelta > 1 || c.cfg.FSM.MinConfidence < 0 || c.cfg.FSM.MinConfidence > 1 {
		return fmt.Errorf("fsm gap_delta and min_confidence must be within [0,1]")
	}
	if c.cfg.FSM.MinSupport < 0 || c.cfg.FSM.MaxSteps <= 0 {
		return fmt.Errorf("fsm min_support and max_steps are invalid")
	}
	parents := make(map[string]bool)
	for _, rule := range c.cfg.StateConversion.RefinementRules {
		parent := strings.TrimSpace(rule.Parent)
		if parent == "" || len(rule.Children) == 0 {
			return fmt.Errorf("refinement rule parent and children are required")
		}
		if parents[parent] {
			return fmt.Errorf("duplicate refinement rule parent %q", parent)
		}
		parents[parent] = true
		seen := make(map[string]bool)
		for _, child := range rule.Children {
			label := strings.TrimSpace(child.Label)
			if label == "" || child.Score < 0 || child.Score > 1 {
				return fmt.Errorf("refinement child label and score are invalid for %q", rule.Parent)
			}
			if seen[label] {
				return fmt.Errorf("duplicate refinement child %q", label)
			}
			seen[label] = true
		}
	}
	return validateBeliefGraph(graph)
}

func (c *StateConverter) checkBacktrack(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, frontier *belief.Frontier) (*StateDecision, error) {
	current := graph.Nodes[frontier.NodeID]
	if current == nil || current.Type != belief.NodeHypothesis || current.Status != belief.StatusActive {
		return nil, fmt.Errorf("frontier %q is not an active hypothesis", frontier.NodeID)
	}

	for level := fsm.GetCurrentLevel() - 1; level >= 1; level-- {
		parents := graph.GetActiveRefinesParentCopies(current.ID)
		if len(parents) != 1 {
			return nil, fmt.Errorf("hypothesis %q must have exactly one active refines parent", current.ID)
		}
		parent := parents[0]
		if parent.Level != level {
			return nil, fmt.Errorf("hypothesis %q parent level is %d, expected %d", current.ID, parent.Level, level)
		}
		best := graph.ExtractFrontier(level)
		if best == nil {
			return nil, fmt.Errorf("no active frontier at ancestor level %d", level)
		}
		if best.NodeID != parent.ID {
			return &StateDecision{
				Action:       DecisionBacktrack,
				ReasonCode:   "ancestor_no_longer_best",
				Reason:       fmt.Sprintf("ancestor %q is no longer the best hypothesis at level %d", parent.Label, level),
				FromLevel:    fsm.GetCurrentLevel(),
				ToLevel:      level,
				FrontierID:   frontier.NodeID,
				BranchRootID: parent.ID,
			}, nil
		}
		current = graph.Nodes[parent.ID]
	}
	return nil, nil
}

func (c *StateConverter) checkGranularity(graph *belief.BeliefGraph, frontier *belief.Frontier) bool {
	node := graph.Nodes[frontier.NodeID]
	if node == nil || node.Attrs == nil {
		return false
	}
	actionable, _ := node.Attrs["actionable"].(bool)
	return actionable
}

func (c *StateConverter) refinementRule(parent string) (RefinementRule, bool) {
	parent = strings.TrimSpace(parent)
	for _, rule := range c.cfg.StateConversion.RefinementRules {
		if strings.EqualFold(strings.TrimSpace(rule.Parent), parent) {
			return rule, true
		}
	}
	return RefinementRule{}, false
}

func (c *StateConverter) validateDecision(fsm *belief.BeliefFSM, decision StateDecision) error {
	if fsm == nil || fsm.IsFinalState() {
		return fmt.Errorf("cannot apply decision to final or nil fsm")
	}
	if decision.FromLevel != fsm.GetCurrentLevel() {
		return fmt.Errorf("stale decision from level %d, current level is %d", decision.FromLevel, fsm.GetCurrentLevel())
	}
	switch decision.Action {
	case DecisionContinue, DecisionReport, DecisionDegraded:
		if decision.ToLevel != decision.FromLevel {
			return fmt.Errorf("%s decision cannot change level", decision.Action)
		}
	case DecisionRefine:
		if fsm.GetState() != belief.StateDrilling {
			return fmt.Errorf("cannot refine from fsm state %d", fsm.GetState())
		}
		if decision.ToLevel != decision.FromLevel+1 || decision.FrontierID == "" || len(decision.Candidates) == 0 {
			return fmt.Errorf("invalid refine transition")
		}
	case DecisionBacktrack:
		if fsm.GetState() != belief.StateDrilling {
			return fmt.Errorf("cannot backtrack from fsm state %d", fsm.GetState())
		}
		if decision.ToLevel < 1 || decision.ToLevel >= decision.FromLevel || decision.BranchRootID == "" {
			return fmt.Errorf("invalid backtrack transition")
		}
	default:
		return fmt.Errorf("unknown state decision %q", decision.Action)
	}
	return nil
}

func (c *StateConverter) refineHypothesis(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, decision StateDecision) error {
	childIDs := make([]string, 0, len(decision.Candidates))
	result := graph.UpdateCopyOnWrite(func(cp *belief.BeliefGraph) error {
		parent := cp.Nodes[decision.FrontierID]
		if parent == nil || parent.Type != belief.NodeHypothesis || parent.Status != belief.StatusActive || parent.Level != decision.FromLevel {
			return fmt.Errorf("refine parent %q is invalid", decision.FrontierID)
		}

		existing := activeRefinesChildren(cp, decision.FrontierID, decision.ToLevel)
		if len(existing) > 0 {
			if !sameCandidateLabels(existing, decision.Candidates) {
				return fmt.Errorf("existing refinement children do not match decision")
			}
			for _, child := range existing {
				childIDs = append(childIDs, child.ID)
			}
			return validateBeliefGraph(cp)
		}

		for _, candidate := range decision.Candidates {
			attrs := map[string]interface{}{
				"why":          candidate.Why,
				"actionable":   candidate.Actionable,
				"refined_from": decision.FrontierID,
			}
			childID := cp.AddNodeCopy(belief.NodeHypothesis, strings.TrimSpace(candidate.Label), candidate.Score, decision.ToLevel, attrs, nil)
			cp.AddEdgeCopy(decision.FrontierID, childID, belief.EdgeRefines, candidate.Score, "state_converter_v1")
			childIDs = append(childIDs, childID)
		}
		return validateBeliefGraph(cp)
	})
	if !result.Committed {
		return fmt.Errorf("refine graph update failed: %w", result.Error)
	}
	if len(childIDs) == 0 || graph.ExtractFrontier(decision.ToLevel) == nil {
		return fmt.Errorf("refine committed without an active level %d frontier", decision.ToLevel)
	}
	fsm.DrillDown(decision.Reason)
	return nil
}

func (c *StateConverter) backtrack(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, decision StateDecision) error {
	result := graph.UpdateCopyOnWrite(func(cp *belief.BeliefGraph) error {
		root := cp.Nodes[decision.BranchRootID]
		if root == nil || root.Type != belief.NodeHypothesis || root.Level != decision.ToLevel {
			return fmt.Errorf("backtrack branch root %q is invalid", decision.BranchRootID)
		}
		retractIDs := descendantSubgraphIDs(cp, decision.BranchRootID, decision.ToLevel)
		if len(retractIDs) == 0 {
			return fmt.Errorf("backtrack branch %q has no active descendants", decision.BranchRootID)
		}
		for _, nodeID := range retractIDs {
			cp.RetractNodeCopy(nodeID, "state_converter", decision.Reason)
		}
		return validateBeliefGraph(cp)
	})
	if !result.Committed {
		return fmt.Errorf("backtrack graph update failed: %w", result.Error)
	}
	if err := fsm.BacktrackTo(decision.ToLevel, decision.Reason); err != nil {
		return err
	}
	return nil
}

func activeRefinesChildren(graph *belief.BeliefGraph, parentID string, level int) []belief.Node {
	children := make([]belief.Node, 0)
	for _, edge := range graph.Edges {
		if edge.Src != parentID || edge.Type != belief.EdgeRefines || edge.Status != belief.StatusActive {
			continue
		}
		child := graph.Nodes[edge.Dst]
		if child != nil && child.Type == belief.NodeHypothesis && child.Status == belief.StatusActive && child.Level == level {
			children = append(children, *child)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Label < children[j].Label })
	return children
}

func sameCandidateLabels(nodes []belief.Node, candidates []RefinementCandidate) bool {
	if len(nodes) != len(candidates) {
		return false
	}
	labels := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		labels = append(labels, strings.TrimSpace(candidate.Label))
	}
	sort.Strings(labels)
	for i := range nodes {
		if nodes[i].Label != labels[i] {
			return false
		}
	}
	return true
}

func refinementCandidatesFromNodes(nodes []belief.Node) []RefinementCandidate {
	candidates := make([]RefinementCandidate, 0, len(nodes))
	for _, node := range nodes {
		why, _ := node.Attrs["why"].(string)
		actionable, _ := node.Attrs["actionable"].(bool)
		candidates = append(candidates, RefinementCandidate{
			Label:      node.Label,
			Score:      node.Score,
			Why:        why,
			Actionable: actionable,
		})
	}
	return candidates
}

func descendantSubgraphIDs(graph *belief.BeliefGraph, branchRootID string, targetLevel int) []string {
	descendants := make(map[string]bool)
	queue := []string{branchRootID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range graph.Edges {
			if edge.Src != current || edge.Type != belief.EdgeRefines || edge.Status != belief.StatusActive {
				continue
			}
			child := graph.Nodes[edge.Dst]
			if child == nil || child.Type != belief.NodeHypothesis || child.Status != belief.StatusActive || child.Level <= targetLevel || descendants[child.ID] {
				continue
			}
			descendants[child.ID] = true
			queue = append(queue, child.ID)
		}
	}

	for _, node := range graph.Nodes {
		if node.Type != belief.NodeEvidence || node.Status != belief.StatusActive || node.Source == nil {
			continue
		}
		if descendants[node.Source.TargetHypothesisID] {
			descendants[node.ID] = true
		}
	}
	ids := make([]string, 0, len(descendants))
	for id := range descendants {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (c *StateConverter) degraded(fsm *belief.BeliefFSM, code string, reason string, frontierID string) StateDecision {
	level := 0
	if fsm != nil {
		level = fsm.GetCurrentLevel()
	}
	return StateDecision{
		Action:     DecisionDegraded,
		ReasonCode: code,
		Reason:     reason,
		FromLevel:  level,
		ToLevel:    level,
		FrontierID: frontierID,
	}
}
