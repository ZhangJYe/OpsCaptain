package gos_engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/belief"
)

const confidenceFormulaVersion = "bounded-linear-v1"

type confidenceContribution struct {
	supportProduct float64
	refuteProduct  float64
	evidenceKeys   []string
	details        []confidenceContributionDetail
}

type confidenceContributionDetail struct {
	EvidenceKey string  `json:"evidence_key"`
	Relation    string  `json:"relation"`
	Strength    float64 `json:"strength"`
}

type analysisCandidate struct {
	text       string
	confidence float64
}

type GraphProposal struct {
	ExpertName                  string                         `json:"expert_name"`
	Analysis                    string                         `json:"analysis"`
	Confidence                  float64                        `json:"confidence"`
	Evidence                    []experts.EvidenceItem         `json:"evidence"`
	Refinements                 []experts.HypothesisRefinement `json:"refinements,omitempty"`
	CurrentHypothesisActionable *bool                          `json:"current_hypothesis_actionable,omitempty"`
}

func newConfidenceContribution() *confidenceContribution {
	return &confidenceContribution{supportProduct: 1, refuteProduct: 1}
}

func (c *confidenceContribution) add(relation experts.EvidenceRelation, strength float64, evidenceKey string) {
	switch relation {
	case experts.EvidenceRelationSupport:
		c.supportProduct *= 1 - strength
	case experts.EvidenceRelationRefute:
		c.refuteProduct *= 1 - strength
	}
	c.evidenceKeys = append(c.evidenceKeys, evidenceKey)
	c.details = append(c.details, confidenceContributionDetail{
		EvidenceKey: evidenceKey,
		Relation:    string(relation),
		Strength:    strength,
	})
}

func (c *confidenceContribution) supportStrength() float64 {
	return 1 - c.supportProduct
}

func (c *confidenceContribution) refuteStrength() float64 {
	return 1 - c.refuteProduct
}

func (e *GoSEngine) updateGraph(ctx context.Context, graph *belief.BeliefGraph, analyses []*experts.ExpertAnalysis, frontier *belief.Frontier) *belief.GraphUpdateResult {
	if err := ctx.Err(); err != nil {
		return &belief.GraphUpdateResult{Committed: false, Error: err}
	}
	if graph == nil || frontier == nil || strings.TrimSpace(frontier.NodeID) == "" {
		return &belief.GraphUpdateResult{Committed: false, Error: fmt.Errorf("graph and frontier are required")}
	}
	if err := e.validateConfidenceConfig(); err != nil {
		return &belief.GraphUpdateResult{Committed: false, Error: err}
	}
	proposals := e.graphProposals(analyses)

	return graph.UpdateCopyOnWrite(func(cp *belief.BeliefGraph) error {
		if err := validateBeliefGraph(cp); err != nil {
			return fmt.Errorf("graph invalid before proposal: %w", err)
		}
		if err := e.validateGraphProposals(cp, frontier, proposals); err != nil {
			return err
		}

		contributions := make(map[string]*confidenceContribution)
		bestAnalyses := make(map[string]analysisCandidate)

		for _, proposal := range proposals {
			acceptedTargets := make(map[string]bool)
			for _, evidence := range proposal.Evidence {
				dedupKey := evidenceDedupKey(evidence)
				if e.cfg.Confidence.Deduplicate && evidenceExists(cp, dedupKey) {
					continue
				}

				observedAt := evidence.ObservationTime
				if observedAt.IsZero() {
					observedAt = time.Now().UTC()
				}
				source := &belief.EvidenceSource{
					SourceType:         evidence.SourceType,
					SourceID:           evidence.SourceID,
					SignalType:         evidence.SignalType,
					Entity:             evidence.Entity,
					ToolName:           evidence.ToolName,
					Timestamp:          observedAt,
					SummarySnippet:     evidence.Snippet,
					ArtifactRef:        evidence.ArtifactRef,
					Relation:           string(evidence.Relation),
					TargetHypothesisID: evidence.TargetHypothesisID,
					Strength:           evidence.Strength,
				}
				evidenceScore := evidence.Score
				if evidenceScore == 0 {
					evidenceScore = evidence.Strength
				}
				attrs := map[string]interface{}{
					"score":                evidenceScore,
					"signal_type":          evidence.SignalType,
					"entity":               evidence.Entity,
					"observation_time":     observedAt,
					"artifact_ref":         evidence.ArtifactRef,
					"relation":             string(evidence.Relation),
					"target_hypothesis_id": evidence.TargetHypothesisID,
					"strength":             evidence.Strength,
					"dedup_key":            dedupKey,
				}
				evidenceID := cp.AddNodeCopy(belief.NodeEvidence, evidence.Title, evidenceScore, 0, attrs, source)
				acceptedTargets[evidence.TargetHypothesisID] = true
				contribution := contributions[evidence.TargetHypothesisID]
				if contribution == nil {
					contribution = newConfidenceContribution()
					contributions[evidence.TargetHypothesisID] = contribution
				}
				contribution.add(evidence.Relation, evidence.Strength, dedupKey)

				if evidence.Relation == experts.EvidenceRelationNeutral {
					continue
				}
				edgeType := belief.EdgeSupport
				if evidence.Relation == experts.EvidenceRelationRefute {
					edgeType = belief.EdgeRefute
				}
				cp.AddEdgeCopy(evidenceID, evidence.TargetHypothesisID, edgeType, evidence.Strength, "expert_evidence_v1")
			}

			for targetID := range acceptedTargets {
				current := bestAnalyses[targetID]
				if strings.TrimSpace(proposal.Analysis) != "" && proposal.Confidence > current.confidence {
					bestAnalyses[targetID] = analysisCandidate{text: proposal.Analysis, confidence: proposal.Confidence}
				}
			}

			for _, refinement := range proposal.Refinements {
				attrs := map[string]interface{}{
					"why":             strings.TrimSpace(refinement.Why),
					"actionable":      refinement.Actionable,
					"proposal_expert": proposal.ExpertName,
					"semantic_type":   "hypothesis_refinement",
				}
				childID := cp.AddNodeCopy(
					belief.NodeHypothesis,
					strings.TrimSpace(refinement.Label),
					refinement.Score,
					cp.Nodes[frontier.NodeID].Level+1,
					attrs,
					nil,
				)
				cp.AddEdgeCopy(frontier.NodeID, childID, belief.EdgeRefines, refinement.Score, "expert_graph_proposal_v1")
			}
			if proposal.CurrentHypothesisActionable != nil && *proposal.CurrentHypothesisActionable {
				parent := cp.Nodes[frontier.NodeID]
				parent.Attrs["actionable"] = true
				parent.Attrs["actionable_source"] = "expert_support_evidence_v1"
			}
		}

		targetIDs := make([]string, 0, len(contributions))
		for targetID := range contributions {
			targetIDs = append(targetIDs, targetID)
		}
		sort.Strings(targetIDs)
		for _, targetID := range targetIDs {
			node := cp.Nodes[targetID]
			if node == nil {
				return fmt.Errorf("target hypothesis %q disappeared during update", targetID)
			}
			contribution := contributions[targetID]
			before := node.Score
			after := e.aggregateConfidence(before, contribution)
			node.Score = after
			if node.Attrs == nil {
				node.Attrs = make(map[string]interface{})
			}
			node.Attrs["confidence"] = after
			node.Attrs["confidence_before"] = before
			node.Attrs["confidence_formula"] = confidenceFormulaVersion
			node.Attrs["support_strength"] = contribution.supportStrength()
			node.Attrs["refute_strength"] = contribution.refuteStrength()
			node.Attrs["evidence_keys"] = append([]string(nil), contribution.evidenceKeys...)
			node.Attrs["confidence_contributions"] = append([]confidenceContributionDetail(nil), contribution.details...)
			if best := bestAnalyses[targetID]; strings.TrimSpace(best.text) != "" {
				node.Attrs["analysis"] = best.text
				node.Attrs["why"] = best.text
			}
		}

		return validateBeliefGraph(cp)
	})
}

func (e *GoSEngine) graphProposals(analyses []*experts.ExpertAnalysis) []GraphProposal {
	proposals := make([]GraphProposal, 0, len(analyses))
	for _, analysis := range analyses {
		if analysis == nil {
			continue
		}
		proposal := GraphProposal{
			ExpertName:                  analysis.ExpertName,
			Analysis:                    analysis.Analysis,
			Confidence:                  analysis.Confidence,
			Evidence:                    append([]experts.EvidenceItem(nil), analysis.Evidence...),
			CurrentHypothesisActionable: analysis.CurrentHypothesisActionable,
		}
		if e.cfg.StructuredCognition.Enabled {
			proposal.Refinements = append([]experts.HypothesisRefinement(nil), analysis.Refinements...)
		}
		proposals = append(proposals, proposal)
	}
	sort.SliceStable(proposals, func(i, j int) bool {
		return proposals[i].ExpertName < proposals[j].ExpertName
	})
	return proposals
}

func (e *GoSEngine) validateGraphProposals(graph *belief.BeliefGraph, frontier *belief.Frontier, proposals []GraphProposal) error {
	parent := graph.Nodes[frontier.NodeID]
	if parent == nil || parent.Type != belief.NodeHypothesis || parent.Status != belief.StatusActive {
		return fmt.Errorf("frontier hypothesis %q is missing or inactive", frontier.NodeID)
	}

	ancestorLabels, err := activeAncestorLabels(graph, parent.ID)
	if err != nil {
		return err
	}
	existingChildLabels := activeRefinementChildLabels(graph, parent.ID)
	proposedLabels := make(map[string]struct{})
	refinementCount := 0
	for _, proposal := range proposals {
		if proposal.Confidence < 0 || proposal.Confidence > 1 {
			return fmt.Errorf("expert %s confidence must be within [0,1]", proposal.ExpertName)
		}
		for _, evidence := range proposal.Evidence {
			if err := validateEvidence(evidence, graph); err != nil {
				return fmt.Errorf("expert %s evidence invalid: %w", proposal.ExpertName, err)
			}
			if proposal.CurrentHypothesisActionable != nil && *proposal.CurrentHypothesisActionable && !proposalHasSupportingEvidence(parent.ID, proposal.Evidence) {
				return fmt.Errorf("expert %s actionable promotion has no source-backed support evidence for frontier %q", proposal.ExpertName, parent.ID)
			}
		}
		if len(proposal.Refinements) == 0 {
			continue
		}
		if strings.TrimSpace(proposal.ExpertName) == "" {
			return fmt.Errorf("refinement proposal requires expert_name")
		}
		if parent.Level >= e.cfg.StateConversion.MaxDepth {
			return fmt.Errorf("frontier level %d reached max refinement depth %d", parent.Level, e.cfg.StateConversion.MaxDepth)
		}
		if !proposalHasSourceBackedEvidence(graph, parent.ID, proposal.Evidence) {
			return fmt.Errorf("expert %s refinement has no source-backed evidence for frontier %q", proposal.ExpertName, parent.ID)
		}
		for _, refinement := range proposal.Refinements {
			refinementCount++
			label := strings.TrimSpace(refinement.Label)
			why := strings.TrimSpace(refinement.Why)
			key := normalizeHypothesisLabel(label)
			switch {
			case label == "" || why == "":
				return fmt.Errorf("expert %s refinement label and why are required", proposal.ExpertName)
			case refinement.Score <= 0 || refinement.Score > 1:
				return fmt.Errorf("expert %s refinement score must be within (0,1]", proposal.ExpertName)
			case key == "":
				return fmt.Errorf("expert %s refinement label is invalid", proposal.ExpertName)
			}
			if _, exists := ancestorLabels[key]; exists {
				return fmt.Errorf("expert %s refinement %q repeats an active ancestor and would create a semantic refines cycle", proposal.ExpertName, label)
			}
			if _, exists := existingChildLabels[key]; exists {
				return fmt.Errorf("expert %s refinement %q already exists under frontier", proposal.ExpertName, label)
			}
			if _, exists := proposedLabels[key]; exists {
				return fmt.Errorf("refinement label %q is duplicated across proposals", label)
			}
			proposedLabels[key] = struct{}{}
		}
	}
	if refinementCount > e.cfg.StructuredCognition.MaxHypotheses {
		return fmt.Errorf("graph proposal contains %d refinements, exceeds max_hypotheses %d", refinementCount, e.cfg.StructuredCognition.MaxHypotheses)
	}
	return nil
}

func proposalHasSupportingEvidence(targetID string, proposed []experts.EvidenceItem) bool {
	for _, evidence := range proposed {
		if evidence.TargetHypothesisID == targetID && evidence.Relation == experts.EvidenceRelationSupport &&
			strings.TrimSpace(evidence.SourceType) != "" && strings.TrimSpace(evidence.SourceID) != "" {
			return true
		}
	}
	return false
}

func proposalHasSourceBackedEvidence(graph *belief.BeliefGraph, targetID string, proposed []experts.EvidenceItem) bool {
	for _, evidence := range proposed {
		if evidence.TargetHypothesisID == targetID && evidence.Relation != experts.EvidenceRelationNeutral &&
			strings.TrimSpace(evidence.SourceType) != "" && strings.TrimSpace(evidence.SourceID) != "" {
			return true
		}
	}
	for _, node := range graph.Nodes {
		if node == nil || node.Type != belief.NodeEvidence || node.Status != belief.StatusActive || node.Source == nil {
			continue
		}
		if node.Source.TargetHypothesisID == targetID && node.Source.Relation != string(experts.EvidenceRelationNeutral) &&
			strings.TrimSpace(node.Source.SourceType) != "" && strings.TrimSpace(node.Source.SourceID) != "" {
			return true
		}
	}
	return false
}

func activeAncestorLabels(graph *belief.BeliefGraph, startID string) (map[string]struct{}, error) {
	labels := make(map[string]struct{})
	visiting := make(map[string]bool)
	visited := make(map[string]bool)
	var visit func(string) error
	visit = func(nodeID string) error {
		if visiting[nodeID] {
			return fmt.Errorf("active refines graph contains cycle at %q", nodeID)
		}
		if visited[nodeID] {
			return nil
		}
		visiting[nodeID] = true
		node := graph.Nodes[nodeID]
		if node == nil || node.Type != belief.NodeHypothesis || node.Status != belief.StatusActive {
			return fmt.Errorf("active refinement ancestor %q is missing or inactive", nodeID)
		}
		labels[normalizeHypothesisLabel(node.Label)] = struct{}{}
		for _, edge := range graph.Edges {
			if edge != nil && edge.Dst == nodeID && edge.Type == belief.EdgeRefines && edge.Status == belief.StatusActive {
				parent := graph.Nodes[edge.Src]
				if parent != nil && parent.Type == belief.NodeHypothesis {
					if err := visit(parent.ID); err != nil {
						return err
					}
				}
			}
		}
		visiting[nodeID] = false
		visited[nodeID] = true
		return nil
	}
	return labels, visit(startID)
}

func activeRefinementChildLabels(graph *belief.BeliefGraph, parentID string) map[string]struct{} {
	labels := make(map[string]struct{})
	for _, edge := range graph.Edges {
		if edge == nil || edge.Src != parentID || edge.Type != belief.EdgeRefines || edge.Status != belief.StatusActive {
			continue
		}
		child := graph.Nodes[edge.Dst]
		if child != nil && child.Type == belief.NodeHypothesis && child.Status == belief.StatusActive {
			labels[normalizeHypothesisLabel(child.Label)] = struct{}{}
		}
	}
	return labels
}

func normalizeHypothesisLabel(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
}

func (e *GoSEngine) validateConfidenceConfig() error {
	if e == nil || e.cfg == nil {
		return fmt.Errorf("gos confidence config is required")
	}
	if e.cfg.Confidence.SupportWeight < 0 || e.cfg.Confidence.SupportWeight > 1 {
		return fmt.Errorf("support_weight must be within [0,1]")
	}
	if e.cfg.Confidence.RefuteWeight < 0 || e.cfg.Confidence.RefuteWeight > 1 {
		return fmt.Errorf("refute_weight must be within [0,1]")
	}
	return nil
}

func (e *GoSEngine) aggregateConfidence(current float64, contribution *confidenceContribution) float64 {
	current = clamp01(current)
	supportDelta := e.cfg.Confidence.SupportWeight * contribution.supportStrength() * (1 - current)
	refuteDelta := e.cfg.Confidence.RefuteWeight * contribution.refuteStrength() * current
	return clamp01(current + supportDelta - refuteDelta)
}

func validateEvidence(evidence experts.EvidenceItem, graph *belief.BeliefGraph) error {
	if strings.TrimSpace(evidence.SourceType) == "" || strings.TrimSpace(evidence.SourceID) == "" {
		return fmt.Errorf("source_type and source_id are required")
	}
	if strings.TrimSpace(evidence.TargetHypothesisID) == "" {
		return fmt.Errorf("target_hypothesis_id is required")
	}
	target := graph.Nodes[evidence.TargetHypothesisID]
	if target == nil || target.Type != belief.NodeHypothesis || target.Status != belief.StatusActive {
		return fmt.Errorf("target hypothesis %q is missing or inactive", evidence.TargetHypothesisID)
	}
	switch evidence.Relation {
	case experts.EvidenceRelationSupport, experts.EvidenceRelationRefute:
		if evidence.Strength <= 0 || evidence.Strength > 1 {
			return fmt.Errorf("support/refute strength must be within (0,1]")
		}
	case experts.EvidenceRelationNeutral:
		if evidence.Strength < 0 || evidence.Strength > 1 {
			return fmt.Errorf("neutral strength must be within [0,1]")
		}
	default:
		return fmt.Errorf("unsupported relation %q", evidence.Relation)
	}
	if evidence.Score < 0 || evidence.Score > 1 {
		return fmt.Errorf("score must be within [0,1]")
	}
	return nil
}

func evidenceDedupKey(evidence experts.EvidenceItem) string {
	return strings.Join([]string{
		strings.TrimSpace(evidence.SourceType),
		strings.TrimSpace(evidence.SourceID),
		strings.TrimSpace(evidence.TargetHypothesisID),
	}, "\x1f")
}

func evidenceExists(graph *belief.BeliefGraph, dedupKey string) bool {
	for _, node := range graph.Nodes {
		if node.Type != belief.NodeEvidence || node.Attrs == nil {
			continue
		}
		if key, _ := node.Attrs["dedup_key"].(string); key == dedupKey {
			return true
		}
	}
	return false
}

func validateBeliefGraph(graph *belief.BeliefGraph) error {
	if graph == nil {
		return fmt.Errorf("belief graph is required")
	}
	dedupKeys := make(map[string]string)
	activeEvidenceEdges := make(map[string]int)
	for nodeID, node := range graph.Nodes {
		if node == nil {
			return fmt.Errorf("node %q is nil", nodeID)
		}
		if node.ID != nodeID {
			return fmt.Errorf("node map key %q does not match id %q", nodeID, node.ID)
		}
		switch node.Type {
		case belief.NodeSignal, belief.NodeEvidence, belief.NodeHypothesis:
		default:
			return fmt.Errorf("node %q has invalid type %q", nodeID, node.Type)
		}
		if node.Score < 0 || node.Score > 1 {
			return fmt.Errorf("node %q score must be within [0,1]", nodeID)
		}
		switch node.Status {
		case belief.StatusActive, belief.StatusRetracted, belief.StatusSuperseded:
		default:
			return fmt.Errorf("node %q has invalid status %q", nodeID, node.Status)
		}
		if node.Type != belief.NodeEvidence {
			continue
		}
		if node.Source == nil || strings.TrimSpace(node.Source.SourceType) == "" || strings.TrimSpace(node.Source.SourceID) == "" {
			return fmt.Errorf("evidence node %q has no source provenance", nodeID)
		}
		if node.Source.Timestamp.IsZero() {
			return fmt.Errorf("evidence node %q has no observation time", nodeID)
		}
		relation := experts.EvidenceRelation(node.Source.Relation)
		switch relation {
		case experts.EvidenceRelationSupport, experts.EvidenceRelationRefute:
			if node.Source.Strength <= 0 || node.Source.Strength > 1 {
				return fmt.Errorf("evidence node %q has invalid support/refute strength", nodeID)
			}
		case experts.EvidenceRelationNeutral:
			if node.Source.Strength < 0 || node.Source.Strength > 1 {
				return fmt.Errorf("evidence node %q has invalid neutral strength", nodeID)
			}
		default:
			return fmt.Errorf("evidence node %q has invalid relation %q", nodeID, node.Source.Relation)
		}
		target := graph.Nodes[node.Source.TargetHypothesisID]
		if target == nil || target.Type != belief.NodeHypothesis {
			return fmt.Errorf("evidence node %q has invalid target hypothesis %q", nodeID, node.Source.TargetHypothesisID)
		}
		if node.Status == belief.StatusActive && target.Status != belief.StatusActive {
			return fmt.Errorf("active evidence node %q targets inactive hypothesis %q", nodeID, target.ID)
		}
		if node.Attrs == nil {
			return fmt.Errorf("evidence node %q has no attributes", nodeID)
		}
		attrRelation, _ := node.Attrs["relation"].(string)
		attrTarget, _ := node.Attrs["target_hypothesis_id"].(string)
		attrStrength, ok := node.Attrs["strength"].(float64)
		if attrRelation != node.Source.Relation || attrTarget != node.Source.TargetHypothesisID || !ok || attrStrength != node.Source.Strength {
			return fmt.Errorf("evidence node %q attributes do not match provenance", nodeID)
		}
		dedupKey, _ := node.Attrs["dedup_key"].(string)
		if strings.TrimSpace(dedupKey) == "" {
			return fmt.Errorf("evidence node %q has no dedup key", nodeID)
		}
		if existingID, exists := dedupKeys[dedupKey]; exists {
			return fmt.Errorf("evidence nodes %q and %q share dedup key", existingID, nodeID)
		}
		expectedDedupKey := strings.Join([]string{
			strings.TrimSpace(node.Source.SourceType),
			strings.TrimSpace(node.Source.SourceID),
			strings.TrimSpace(node.Source.TargetHypothesisID),
		}, "\x1f")
		if dedupKey != expectedDedupKey {
			return fmt.Errorf("evidence node %q dedup key does not match provenance", nodeID)
		}
		dedupKeys[dedupKey] = nodeID
	}

	for _, edge := range graph.Edges {
		if edge == nil {
			return fmt.Errorf("belief graph contains nil edge")
		}
		source := graph.Nodes[edge.Src]
		target := graph.Nodes[edge.Dst]
		if source == nil || target == nil {
			return fmt.Errorf("edge %q -> %q has missing endpoint", edge.Src, edge.Dst)
		}
		if edge.Confidence < 0 || edge.Confidence > 1 {
			return fmt.Errorf("edge %q -> %q confidence must be within [0,1]", edge.Src, edge.Dst)
		}
		switch edge.Type {
		case belief.EdgeSupport, belief.EdgeRefute:
			if source.Type != belief.NodeEvidence || target.Type != belief.NodeHypothesis {
				return fmt.Errorf("%s edge must connect evidence to hypothesis", edge.Type)
			}
			if source.Source == nil || source.Source.TargetHypothesisID != target.ID {
				return fmt.Errorf("%s edge target does not match evidence provenance", edge.Type)
			}
			expectedRelation := string(experts.EvidenceRelationSupport)
			if edge.Type == belief.EdgeRefute {
				expectedRelation = string(experts.EvidenceRelationRefute)
			}
			if source.Source.Relation != expectedRelation {
				return fmt.Errorf("%s edge does not match evidence relation %q", edge.Type, source.Source.Relation)
			}
			if edge.Confidence != source.Source.Strength {
				return fmt.Errorf("%s edge confidence does not match evidence strength", edge.Type)
			}
			if edge.Status == belief.StatusActive {
				activeEvidenceEdges[source.ID]++
			}
		case belief.EdgeRefines:
			if target.Type != belief.NodeHypothesis {
				return fmt.Errorf("refines edge must target hypothesis")
			}
			switch source.Type {
			case belief.NodeSignal:
				if target.Level != 1 {
					return fmt.Errorf("signal refines edge must target level 1 hypothesis")
				}
			case belief.NodeHypothesis:
				if target.Level != source.Level+1 {
					return fmt.Errorf("hypothesis refines edge must advance exactly one level")
				}
			default:
				return fmt.Errorf("refines edge must start from signal or hypothesis")
			}
		case belief.EdgeCausal:
		default:
			return fmt.Errorf("edge %q -> %q has invalid type %q", edge.Src, edge.Dst, edge.Type)
		}
		switch edge.Status {
		case belief.StatusActive, belief.StatusRetracted, belief.StatusSuperseded:
		default:
			return fmt.Errorf("edge %q -> %q has invalid status %q", edge.Src, edge.Dst, edge.Status)
		}
	}
	for nodeID, node := range graph.Nodes {
		if node.Type != belief.NodeEvidence || node.Status != belief.StatusActive {
			continue
		}
		relation := experts.EvidenceRelation(node.Source.Relation)
		switch relation {
		case experts.EvidenceRelationSupport, experts.EvidenceRelationRefute:
			if activeEvidenceEdges[nodeID] != 1 {
				return fmt.Errorf("active %s evidence node %q must have exactly one active relation edge", relation, nodeID)
			}
		case experts.EvidenceRelationNeutral:
			if activeEvidenceEdges[nodeID] != 0 {
				return fmt.Errorf("neutral evidence node %q cannot have support/refute edge", nodeID)
			}
		}
	}
	return nil
}

func ValidateBeliefGraph(graph *belief.BeliefGraph) error {
	return validateBeliefGraph(graph)
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
