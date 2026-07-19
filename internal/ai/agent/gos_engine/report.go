package gos_engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/protocol"
)

type EvidenceReport struct {
	Conclusion             ReportConclusion `json:"conclusion"`
	Confidence             float64          `json:"confidence"`
	SupportingEvidence     []ReportEvidence `json:"supporting_evidence"`
	RefutingEvidence       []ReportEvidence `json:"refuting_evidence"`
	NeutralEvidence        []ReportEvidence `json:"neutral_evidence"`
	Conflicts              []ReportConflict `json:"conflicts"`
	UnresolvedGaps         []string         `json:"unresolved_gaps"`
	NextActions            []string         `json:"next_actions"`
	ReasonCodes            []string         `json:"reason_codes,omitempty"`
	Sufficient             bool             `json:"sufficient"`
	SnippetDisplayMaxChars int              `json:"-"`
	EvaluationEvidence     []ReportEvidence `json:"-"`
}

type ReportConclusion struct {
	Text         string `json:"text"`
	HypothesisID string `json:"hypothesis_id,omitempty"`
	Rationale    string `json:"rationale,omitempty"`
}

type ReportEvidence struct {
	NodeID             string    `json:"node_id"`
	EdgeRef            string    `json:"edge_ref,omitempty"`
	Relation           string    `json:"relation"`
	Strength           float64   `json:"strength"`
	TargetHypothesisID string    `json:"target_hypothesis_id"`
	SourceType         string    `json:"source_type"`
	SourceID           string    `json:"source_id"`
	SignalType         string    `json:"signal_type,omitempty"`
	Entity             string    `json:"entity,omitempty"`
	ToolName           string    `json:"tool_name,omitempty"`
	ArtifactRef        string    `json:"artifact_ref,omitempty"`
	Title              string    `json:"title"`
	Snippet            string    `json:"snippet"`
	ObservationTime    time.Time `json:"observation_time,omitempty"`
}

type ReportConflict struct {
	TargetHypothesisID string   `json:"target_hypothesis_id"`
	SupportNodeIDs     []string `json:"support_node_ids"`
	RefuteNodeIDs      []string `json:"refute_node_ids"`
	Reason             string   `json:"reason"`
}

func selectReportFrontier(graph *belief.BeliefGraph, fsm *belief.BeliefFSM) *belief.Frontier {
	if graph == nil {
		return nil
	}
	level := 0
	if fsm != nil {
		level = fsm.GetCurrentLevel()
	}
	for current := level; current >= 0; current-- {
		if frontier := graph.ExtractFrontier(current); frontier != nil {
			return frontier
		}
	}
	return nil
}

func (e *GoSEngine) buildEvidenceReport(graph *belief.BeliefGraph, frontier *belief.Frontier, stats *RunStats, additionalGaps ...string) EvidenceReport {
	report := EvidenceReport{SnippetDisplayMaxChars: e.cfg.Report.EvidenceSnippetMaxChars}
	if frontier == nil {
		report.Conclusion.Text = "尚未形成可验证的根因候选"
		report.UnresolvedGaps = appendUniqueStrings(report.UnresolvedGaps, "没有可用 frontier")
		report.ReasonCodes = append(report.ReasonCodes, "no_frontier")
	} else {
		report.Confidence = clamp01(frontier.Score)
		report.Conclusion.HypothesisID = frontier.NodeID
		report.Conclusion.Rationale = compactReportText(frontier.Why, e.cfg.Report.EvidenceSnippetMaxChars)
	}

	targetIDs := reportHypothesisScopeIDs(graph, frontier)
	allEvidence := reportEvidenceForTargets(graph, targetIDs)
	report.EvaluationEvidence = append([]ReportEvidence(nil), allEvidence...)
	allSupports, allRefutes, allNeutral := splitReportEvidence(allEvidence)
	maxItems := e.cfg.Report.MaxEvidenceItems
	if maxItems <= 0 {
		maxItems = 1
	}
	selected := selectReportEvidence(allSupports, allRefutes, allNeutral, maxItems)
	report.SupportingEvidence, report.RefutingEvidence, report.NeutralEvidence = splitReportEvidence(selected)

	requiredSupports := e.cfg.FSM.MinSupport
	if requiredSupports < 1 {
		requiredSupports = 1
	}
	directSupports := 0
	if frontier != nil {
		for _, item := range allSupports {
			if item.TargetHypothesisID == frontier.NodeID && item.Strength > 0 {
				directSupports++
			}
		}
	}
	if directSupports < requiredSupports {
		report.UnresolvedGaps = appendUniqueStrings(report.UnresolvedGaps, fmt.Sprintf("当前候选仅有 %d 条直接支持证据，需要至少 %d 条", directSupports, requiredSupports))
		report.ReasonCodes = appendUniqueStrings(report.ReasonCodes, "insufficient_source_backed_support")
	}
	if frontier != nil && report.Confidence < e.cfg.FSM.MinConfidence {
		report.UnresolvedGaps = appendUniqueStrings(report.UnresolvedGaps, fmt.Sprintf("图聚合置信度 %.2f 低于报告阈值 %.2f", report.Confidence, e.cfg.FSM.MinConfidence))
		report.ReasonCodes = appendUniqueStrings(report.ReasonCodes, "low_graph_confidence")
	}
	report.Conflicts = buildReportConflicts(allSupports, allRefutes, e.cfg.Report.ConflictStrengthThreshold)
	if len(report.Conflicts) > 0 {
		report.UnresolvedGaps = appendUniqueStrings(report.UnresolvedGaps, "存在达到阈值的反驳证据，需先解决证据冲突")
		report.ReasonCodes = appendUniqueStrings(report.ReasonCodes, "critical_evidence_conflict")
	}
	if stats != nil && (stats.ExpertDegraded > 0 || stats.ExpertFailed > 0) {
		report.UnresolvedGaps = appendUniqueStrings(report.UnresolvedGaps, fmt.Sprintf("专家执行存在 %d 个降级、%d 个失败", stats.ExpertDegraded, stats.ExpertFailed))
		report.ReasonCodes = appendUniqueStrings(report.ReasonCodes, "partial_expert_failure")
	}
	for _, gap := range additionalGaps {
		report.UnresolvedGaps = appendUniqueStrings(report.UnresolvedGaps, gap)
	}

	report.Sufficient = frontier != nil && directSupports >= requiredSupports && report.Confidence >= e.cfg.FSM.MinConfidence && len(report.Conflicts) == 0 && (stats == nil || stats.ExpertDegraded == 0 && stats.ExpertFailed == 0)
	switch {
	case frontier == nil:
	case report.Sufficient:
		report.Conclusion.Text = "根因候选：" + frontier.Label
	case directSupports > 0:
		report.Conclusion.Text = "当前最优候选：" + frontier.Label + "（仍需验证）"
	default:
		report.Conclusion.Text = "当前候选：" + frontier.Label + "（证据不足，不能确认为根因）"
	}
	report.NextActions = e.reportNextActions(report)
	if e.logger != nil {
		e.logger.Info("evidence report generated",
			"support_count", len(report.SupportingEvidence),
			"refute_count", len(report.RefutingEvidence),
			"neutral_count", len(report.NeutralEvidence),
			"protocol_evidence_count", len(report.protocolEvidence()),
			"sufficient", report.Sufficient,
		)
	}
	return report
}

func reportHypothesisScopeIDs(graph *belief.BeliefGraph, frontier *belief.Frontier) map[string]struct{} {
	return activeHypothesisPathIDs(graph, frontier)
}

func activeHypothesisPathIDs(graph *belief.BeliefGraph, frontier *belief.Frontier) map[string]struct{} {
	ids := make(map[string]struct{})
	if graph == nil || frontier == nil {
		return ids
	}
	ids[frontier.NodeID] = struct{}{}
	edges := graph.GetActiveEdgeCopies()
	nodes := graph.GetActiveNodeCopies()
	byID := make(map[string]belief.Node, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	changed := true
	for changed {
		changed = false
		for _, edge := range edges {
			if edge.Type != belief.EdgeRefines {
				continue
			}
			if _, childIncluded := ids[edge.Dst]; !childIncluded {
				continue
			}
			parent, exists := byID[edge.Src]
			if !exists || parent.Type != belief.NodeHypothesis || parent.Status != belief.StatusActive {
				continue
			}
			if _, exists := ids[parent.ID]; !exists {
				ids[parent.ID] = struct{}{}
				changed = true
			}
		}
	}
	return ids
}

func reportEvidenceForTargets(graph *belief.BeliefGraph, targetIDs map[string]struct{}) []ReportEvidence {
	if graph == nil || len(targetIDs) == 0 {
		return nil
	}
	items := make([]ReportEvidence, 0)
	for _, node := range graph.GetActiveNodeCopies() {
		if node.Type != belief.NodeEvidence || node.Source == nil {
			continue
		}
		if _, ok := targetIDs[node.Source.TargetHypothesisID]; !ok {
			continue
		}
		edgeRef := ""
		if node.Source.Relation == string(experts.EvidenceRelationSupport) || node.Source.Relation == string(experts.EvidenceRelationRefute) {
			edgeRef = node.ID + "->" + node.Source.TargetHypothesisID
		}
		items = append(items, ReportEvidence{
			NodeID:             node.ID,
			EdgeRef:            edgeRef,
			Relation:           node.Source.Relation,
			Strength:           node.Source.Strength,
			TargetHypothesisID: node.Source.TargetHypothesisID,
			SourceType:         node.Source.SourceType,
			SourceID:           node.Source.SourceID,
			SignalType:         node.Source.SignalType,
			Entity:             node.Source.Entity,
			ToolName:           node.Source.ToolName,
			ArtifactRef:        node.Source.ArtifactRef,
			Title:              node.Label,
			Snippet:            node.Source.SummarySnippet,
			ObservationTime:    node.Source.Timestamp,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		leftPriority := reportRelationPriority(items[i].Relation)
		rightPriority := reportRelationPriority(items[j].Relation)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if items[i].Strength != items[j].Strength {
			return items[i].Strength > items[j].Strength
		}
		if items[i].SourceID != items[j].SourceID {
			return items[i].SourceID < items[j].SourceID
		}
		return items[i].NodeID < items[j].NodeID
	})
	return items
}

func reportRelationPriority(relation string) int {
	switch experts.EvidenceRelation(relation) {
	case experts.EvidenceRelationSupport:
		return 0
	case experts.EvidenceRelationRefute:
		return 1
	default:
		return 2
	}
}

func splitReportEvidence(items []ReportEvidence) (supports, refutes, neutral []ReportEvidence) {
	for _, item := range items {
		switch experts.EvidenceRelation(item.Relation) {
		case experts.EvidenceRelationSupport:
			supports = append(supports, item)
		case experts.EvidenceRelationRefute:
			refutes = append(refutes, item)
		default:
			neutral = append(neutral, item)
		}
	}
	return supports, refutes, neutral
}

func selectReportEvidence(supports, refutes, neutral []ReportEvidence, limit int) []ReportEvidence {
	groups := [][]ReportEvidence{supports, refutes, neutral}
	selected := make([]ReportEvidence, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, group := range groups {
		if len(group) == 0 || len(selected) >= limit {
			continue
		}
		selected = append(selected, group[0])
		seen[group[0].NodeID] = struct{}{}
	}
	for _, group := range groups {
		for _, item := range group {
			if len(selected) >= limit {
				return selected
			}
			if _, exists := seen[item.NodeID]; exists {
				continue
			}
			selected = append(selected, item)
			seen[item.NodeID] = struct{}{}
		}
	}
	return selected
}

func buildReportConflicts(supports, refutes []ReportEvidence, threshold float64) []ReportConflict {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.5
	}
	supportByTarget := make(map[string][]string)
	refuteByTarget := make(map[string][]string)
	for _, item := range supports {
		supportByTarget[item.TargetHypothesisID] = append(supportByTarget[item.TargetHypothesisID], item.NodeID)
	}
	for _, item := range refutes {
		if item.Strength >= threshold {
			refuteByTarget[item.TargetHypothesisID] = append(refuteByTarget[item.TargetHypothesisID], item.NodeID)
		}
	}
	targets := make([]string, 0, len(refuteByTarget))
	for targetID := range refuteByTarget {
		targets = append(targets, targetID)
	}
	sort.Strings(targets)
	conflicts := make([]ReportConflict, 0, len(targets))
	for _, targetID := range targets {
		conflicts = append(conflicts, ReportConflict{
			TargetHypothesisID: targetID,
			SupportNodeIDs:     append([]string(nil), supportByTarget[targetID]...),
			RefuteNodeIDs:      append([]string(nil), refuteByTarget[targetID]...),
			Reason:             fmt.Sprintf("存在强度不低于 %.2f 的有效反驳证据", threshold),
		})
	}
	return conflicts
}

func (e *GoSEngine) reportNextActions(report EvidenceReport) []string {
	actions := make([]string, 0, 3)
	if containsString(report.ReasonCodes, "insufficient_source_backed_support") || containsString(report.ReasonCodes, "low_graph_confidence") {
		actions = append(actions, "补充与当前候选直接关联的指标、日志或调用链证据")
	}
	if containsString(report.ReasonCodes, "critical_evidence_conflict") {
		actions = append(actions, "复核反驳证据的时间窗、实体和来源，并重新评估当前候选")
	}
	if containsString(report.ReasonCodes, "partial_expert_failure") {
		actions = append(actions, "修复或重试失败的授权工具后重新执行受影响的取证步骤")
	}
	if len(actions) == 0 {
		actions = append(actions, "在执行变更前由值班人员核对证据来源与影响范围")
	}
	limit := e.cfg.Report.MaxNextActions
	if limit <= 0 {
		limit = 1
	}
	if len(actions) > limit {
		actions = actions[:limit]
	}
	return actions
}

func formatEvidenceReport(report EvidenceReport) string {
	var b strings.Builder
	b.WriteString("## 结论\n\n")
	b.WriteString(report.Conclusion.Text)
	if report.Conclusion.HypothesisID != "" {
		b.WriteString(fmt.Sprintf("（graph node: `%s`）", report.Conclusion.HypothesisID))
	}
	if report.Conclusion.Rationale != "" {
		b.WriteString("\n\n诊断依据：" + report.Conclusion.Rationale)
	}
	b.WriteString(fmt.Sprintf("\n\n## 图聚合置信度\n\n%.0f%%\n", report.Confidence*100))
	writeReportEvidenceSection(&b, "支持证据", report.SupportingEvidence, report.SnippetDisplayMaxChars)
	writeReportEvidenceSection(&b, "反驳与冲突证据", report.RefutingEvidence, report.SnippetDisplayMaxChars)
	writeReportEvidenceSection(&b, "待判定观测", report.NeutralEvidence, report.SnippetDisplayMaxChars)
	b.WriteString("\n## 未解决缺口\n\n")
	writeStringList(&b, report.UnresolvedGaps, "无关键缺口")
	b.WriteString("\n## 建议下一步\n\n")
	writeStringList(&b, report.NextActions, "由值班人员复核")
	return strings.TrimSpace(b.String())
}

func writeReportEvidenceSection(b *strings.Builder, title string, items []ReportEvidence, snippetMaxChars int) {
	b.WriteString("\n## " + title + "\n\n")
	if len(items) == 0 {
		b.WriteString("- 无\n")
		return
	}
	for _, item := range items {
		detail := item.Title
		snippet := compactReportText(item.Snippet, snippetMaxChars)
		if snippet != "" && snippet != item.Title {
			detail += " — " + snippet
		}
		b.WriteString(fmt.Sprintf("- `%s` / `%s:%s` → `%s`，strength=%.2f：%s\n", item.NodeID, item.SourceType, item.SourceID, item.TargetHypothesisID, item.Strength, detail))
	}
}

func compactReportText(value string, maxChars int) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || maxChars <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	return string(runes[:maxChars]) + "…"
}

func writeStringList(b *strings.Builder, items []string, emptyText string) {
	if len(items) == 0 {
		b.WriteString("- " + emptyText + "\n")
		return
	}
	for _, item := range items {
		b.WriteString("- " + item + "\n")
	}
}

func (report EvidenceReport) protocolEvidence() []protocol.EvidenceItem {
	evidence := report.EvaluationEvidence
	if evidence == nil {
		evidence = make([]ReportEvidence, 0, len(report.SupportingEvidence)+len(report.RefutingEvidence)+len(report.NeutralEvidence))
		evidence = append(evidence, report.SupportingEvidence...)
		evidence = append(evidence, report.RefutingEvidence...)
		evidence = append(evidence, report.NeutralEvidence...)
	}
	items := make([]protocol.EvidenceItem, 0, len(evidence))
	appendEvidence := func(evidence []ReportEvidence) {
		for _, item := range evidence {
			items = append(items, protocol.EvidenceItem{
				SourceType:      item.SourceType,
				SourceID:        item.SourceID,
				SignalType:      item.SignalType,
				Entity:          item.Entity,
				Title:           item.Title,
				Snippet:         item.Snippet,
				Score:           item.Strength,
				URI:             item.ArtifactRef,
				ObservationTime: item.ObservationTime,
			})
		}
	}
	appendEvidence(evidence)
	return items
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		values = append(values, value)
		seen[value] = struct{}{}
	}
	return values
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
