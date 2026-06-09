package gos_engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/protocol"
)

type GoSEngine struct {
	cfg     *Config
	experts map[string]experts.ExpertAgent
	logger  Logger
}

type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

type ActResult struct {
	Analyses      []*experts.ExpertAnalysis
	DegradedCount int
	FailedCount   int
}

type RunStats struct {
	LLMCalls  int
	ToolCalls int
	RAGCalls  int
	Steps     int
}

func NewGoSEngine(cfg *Config, logger Logger) *GoSEngine {
	return &GoSEngine{
		experts: make(map[string]experts.ExpertAgent),
		cfg:     cfg,
		logger:  logger,
	}
}

func (e *GoSEngine) RegisterExpert(name string, agent experts.ExpertAgent) {
	e.experts[name] = agent
}

func (e *GoSEngine) SetEmitter(emit EventEmitter) {
	e.cfg.Emit = emit
}

func (e *GoSEngine) emit(ctx context.Context, message string, detail string, payload map[string]any) {
	if e.cfg.Emit == nil {
		return
	}
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["stage"] = message
	e.cfg.Emit(ctx, detail, payload)
}

func (e *GoSEngine) Run(ctx context.Context, symptom string) *protocol.TaskResult {
	startedAt := time.Now()
	stats := &RunStats{}

	graph := belief.NewBeliefGraph()
	fsm := belief.NewBeliefFSM(e.cfg.ToFSMThresholds())

	e.emit(ctx, "ingest", "解析症状并建立候选假设", nil)
	if err := e.ingest(ctx, graph, symptom); err != nil {
		return e.degradedResult(graph, fsm, startedAt, stats, "ingest_failed", err, nil, false)
	}
	e.emit(ctx, "ingest_done", fmt.Sprintf("已抽取 %d 个候选节点", len(graph.Nodes)), map[string]any{
		"node_count": len(graph.Nodes),
	})

	for {
		if fsm.IsFinalState() {
			break
		}

		frontier := graph.ExtractFrontier(fsm.GetCurrentLevel())
		if frontier == nil {
			fsm.MarkDone("no frontier")
			break
		}

		e.emit(ctx, "frontier_selected", fmt.Sprintf("选中 frontier: %s (score=%.2f)", frontier.Label, frontier.Score), map[string]any{
			"frontier_label": frontier.Label,
			"frontier_score": frontier.Score,
			"fsm_level":      fsm.GetCurrentLevel(),
		})

		plan, err := e.plan(ctx, frontier)
		if err != nil {
			return e.degradedResult(graph, fsm, startedAt, stats, "plan_failed", err, nil, false)
		}

		expertNames := make([]string, 0, len(plan))
		for _, p := range plan {
			expertNames = append(expertNames, p.ExpertName)
		}
		e.emit(ctx, "expert_planned", fmt.Sprintf("调度 %d 位专家: %v", len(plan), expertNames), map[string]any{
			"expert_count": len(plan),
			"expert_names": expertNames,
		})

		actRes, err := e.act(ctx, plan, frontier, graph, stats)

		alreadyUpdated := false
		if actRes != nil && len(actRes.Analyses) > 0 {
			if res := e.updateGraph(ctx, graph, actRes.Analyses, frontier); res.Committed {
				alreadyUpdated = true
			}
			e.emit(ctx, "evidence_attached", fmt.Sprintf("挂载 %d 条证据, %d 失败", len(actRes.Analyses), actRes.FailedCount), map[string]any{
				"evidence_count": len(actRes.Analyses),
				"failed_count":   actRes.FailedCount,
				"degraded_count": actRes.DegradedCount,
			})
		}

		if err != nil {
			return e.degradedResult(graph, fsm, startedAt, stats, "act_failed", err, actRes, alreadyUpdated)
		}

		graph.GenerateBeliefText()
		fsm.TickStep(1)
		stats.Steps++

		updatedFrontier := graph.ExtractFrontier(fsm.GetCurrentLevel())
		if updatedFrontier != nil {
			e.emit(ctx, "confidence_updated", fmt.Sprintf("置信度 %.2f, supports=%d", updatedFrontier.Score, updatedFrontier.Supports), map[string]any{
				"confidence": updatedFrontier.Score,
				"supports":   updatedFrontier.Supports,
				"steps":      stats.Steps,
			})
		}

		decision := fsm.Decide(graph)
		e.emit(ctx, "fsm_decision", fmt.Sprintf("FSM 决策: %s", decision.Action), map[string]any{
			"action":      decision.Action,
			"reason":      decision.Reason,
			"fsm_level":   fsm.GetCurrentLevel(),
			"total_steps": fsm.TotalSteps,
		})

		switch decision.Action {
		case "report":
			if updatedFrontier != nil && e.shouldReport(updatedFrontier) {
				fsm.MarkDone("sufficient granularity")
				goto DONE
			}
			fsm.DrillDown(fmt.Sprintf("drill to level %d", fsm.CurrentLevel+1))
		case "done":
			goto DONE
		}

		if fsm.TotalSteps >= e.cfg.SessionMaxSteps {
			fsm.MarkDone("max steps")
			break
		}
	}

DONE:
	e.emit(ctx, "report", "生成信念报告", map[string]any{
		"total_steps": stats.Steps,
		"llm_calls":   stats.LLMCalls,
		"tool_calls":  stats.ToolCalls,
		"rag_calls":   stats.RAGCalls,
	})
	return e.generateReport(ctx, graph, fsm, startedAt, stats)
}

func (e *GoSEngine) ingest(ctx context.Context, graph *belief.BeliefGraph, symptom string) error {
	ingestor := NewIngestor(graph, e.logger)
	return ingestor.Ingest(ctx, symptom)
}

func (e *GoSEngine) plan(ctx context.Context, frontier *belief.Frontier) ([]PlanItem, error) {
	planner := NewPlanner(e.experts, e.cfg, e.logger)
	return planner.Plan(ctx, frontier)
}

func (e *GoSEngine) act(ctx context.Context, plan []PlanItem, frontier *belief.Frontier, graph *belief.BeliefGraph, stats *RunStats) (*ActResult, error) {
	analyses := make([]*experts.ExpertAnalysis, len(plan))
	var mu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(3)

	for i, item := range plan {
		i, item := i, item
		g.Go(func() error {
			agent, exists := e.experts[item.ExpertName]
			if !exists {
				analyses[i] = &experts.ExpertAnalysis{
					ExpertName:        item.ExpertName,
					Status:            "failed",
					DegradationReason: "expert_not_found",
				}
				return nil
			}

			// Call Run directly within the errgroup goroutine.
			// agent.Run must respect context cancellation to avoid goroutine leaks.
			analysis := agent.Run(gCtx, frontier, graph)
			if analysis == nil {
				analysis = &experts.ExpertAnalysis{
					ExpertName:        item.ExpertName,
					Status:            "failed",
					DegradationReason: "expert_nil_result",
				}
			}
			if gCtx.Err() != nil && analysis.Status != "failed" {
				analysis.Status = "degraded"
				analysis.DegradationReason = "context_cancelled"
			}
			analyses[i] = analysis

			mu.Lock()
			stats.LLMCalls += analysis.LLMCalls
			stats.ToolCalls += analysis.ToolCalls
			stats.RAGCalls += analysis.RAGCalls
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	result := &ActResult{Analyses: analyses}
	for _, a := range analyses {
		switch a.Status {
		case "degraded":
			result.DegradedCount++
		case "failed":
			result.FailedCount++
		}
	}

	if result.FailedCount+result.DegradedCount == len(plan) {
		return result, fmt.Errorf("all experts failed or degraded (%d failed, %d degraded)", result.FailedCount, result.DegradedCount)
	}

	return result, nil
}

func (e *GoSEngine) updateGraph(ctx context.Context, graph *belief.BeliefGraph, analyses []*experts.ExpertAnalysis, frontier *belief.Frontier) *belief.GraphUpdateResult {
	return graph.UpdateCopyOnWrite(func(cp *belief.BeliefGraph) error {
		bestAnalysis := ""
		bestConfidence := 0.0
		for _, a := range analyses {
			for _, ev := range a.Evidence {
				src := &belief.EvidenceSource{
					SourceType:     ev.SourceType,
					SourceID:       ev.SourceID,
					SummarySnippet: ev.Snippet,
				}
				attrs := map[string]interface{}{
					"score": ev.Score,
				}
				eid := cp.AddNodeCopy(belief.NodeEvidence, ev.Title, ev.Score, 0, attrs, src)

				edgeType := belief.EdgeSupport
				if a.Confidence < 0.5 {
					edgeType = belief.EdgeRefute
				}
				cp.AddEdgeCopy(eid, frontier.NodeID, edgeType, ev.Score, "expert_analysis")
			}
			if a.Confidence > bestConfidence {
				bestConfidence = a.Confidence
				bestAnalysis = a.Analysis
			}
		}

		if bestConfidence > 0 {
			node := cp.Nodes[frontier.NodeID]
			if node != nil {
				if node.Attrs == nil {
					node.Attrs = make(map[string]interface{})
				}
				node.Attrs["analysis"] = bestAnalysis
				node.Attrs["confidence"] = bestConfidence
				node.Attrs["why"] = bestAnalysis
				if bestConfidence > node.Score {
					node.Score = bestConfidence
				}
			}
		}

		return nil
	})
}

func (e *GoSEngine) shouldReport(frontier *belief.Frontier) bool {
	return frontier.Score >= e.cfg.FSM.MinConfidence && frontier.Supports >= e.cfg.FSM.MinSupport
}

func (e *GoSEngine) degradedResult(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, startedAt time.Time, stats *RunStats, reason string, err error, actRes *ActResult, alreadyUpdated bool) *protocol.TaskResult {
	if !alreadyUpdated && actRes != nil && len(actRes.Analyses) > 0 {
		if f := graph.ExtractFrontier(fsm.GetCurrentLevel()); f != nil {
			e.updateGraph(context.Background(), graph, actRes.Analyses, f)
		}
	}
	summary, confidence := e.degradedSummary(reason, err, actRes)

	return &protocol.TaskResult{
		TaskID:            uuid.NewString(),
		Agent:             "gos_engine",
		Status:            protocol.ResultStatusDegraded,
		Summary:           summary,
		Confidence:        confidence,
		DegradationReason: fmt.Sprintf("%s: %v", reason, err),
		Evidence:          e.collectEvidence(graph),
		Metadata: map[string]any{
			"belief_graph": graph.ToDict(),
			"fsm_history":  fsm.History,
			"error_phase":  reason,
			"llm_calls":    stats.LLMCalls,
			"tool_calls":   stats.ToolCalls,
			"rag_calls":    stats.RAGCalls,
			"steps":        stats.Steps,
		},
		StartedAt:  startedAt.UnixMilli(),
		FinishedAt: time.Now().UnixMilli(),
	}
}

func (e *GoSEngine) degradedSummary(reason string, err error, actRes *ActResult) (string, float64) {
	var gaps []string
	if actRes != nil {
		for _, analysis := range actRes.Analyses {
			if analysis == nil {
				continue
			}
			text := strings.TrimSpace(analysis.Analysis)
			if text != "" && !isWeakDegradedText(text) {
				return text, analysis.Confidence
			}
			for _, toolErr := range analysis.ToolErrors {
				item := strings.TrimSpace(fmt.Sprintf("%s %s: %s", toolErr.ToolName, toolErr.Action, toolErr.Error))
				if item != "" {
					gaps = append(gaps, item)
				}
			}
		}
	}
	if len(gaps) == 0 {
		gaps = append(gaps, fmt.Sprintf("%s: %v", reason, err))
	}
	var b strings.Builder
	b.WriteString("GoS 未获得足够可用证据，无法形成可信根因。\n\n")
	b.WriteString("缺少或不可用的证据：\n")
	for _, gap := range compactStrings(gaps, 4) {
		b.WriteString("- ")
		b.WriteString(gap)
		b.WriteString("\n")
	}
	b.WriteString("\n下一步建议：\n")
	b.WriteString("- 补充服务名、告警时间窗、关键日志片段和指标快照。\n")
	b.WriteString("- 确认知识库 collection 已导入 runbook/历史案例，并检查 Milvus schema 与文档数。\n")
	b.WriteString("- 确认日志 MCP `/healthz` 和 `/tools/query_logs` 可用后重试。")
	return b.String(), 0
}

func isWeakDegradedText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	switch trimmed {
	case "信息不足，无法完成分析", "诊断降级":
		return true
	}
	return len([]rune(trimmed)) < 8
}

func compactStrings(items []string, limit int) []string {
	out := make([]string, 0, limit)
	seen := make(map[string]bool)
	for _, item := range items {
		cleaned := strings.Join(strings.Fields(strings.TrimSpace(item)), " ")
		if cleaned == "" || seen[cleaned] {
			continue
		}
		out = append(out, cleaned)
		seen[cleaned] = true
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (e *GoSEngine) generateReport(ctx context.Context, graph *belief.BeliefGraph, fsm *belief.BeliefFSM, startedAt time.Time, stats *RunStats) *protocol.TaskResult {
	frontier := graph.ExtractFrontier(fsm.GetCurrentLevel())
	if frontier == nil {
		for lvl := fsm.GetCurrentLevel() - 1; lvl >= 0; lvl-- {
			frontier = graph.ExtractFrontier(lvl)
			if frontier != nil {
				break
			}
		}
	}
	if frontier == nil {
		return e.degradedResult(graph, fsm, startedAt, stats, "no_frontier", fmt.Errorf("no frontier found"), nil, false)
	}

	summary := frontier.Label
	confidence := frontier.Score

	analysisText := ""
	if attrs := graph.Nodes[frontier.NodeID].Attrs; attrs != nil {
		if a, ok := attrs["analysis"].(string); ok {
			analysisText = a
		}
	}
	if analysisText != "" {
		summary = analysisText
	}

	if attrs := graph.Nodes[frontier.NodeID].Attrs; attrs != nil {
		if c, ok := attrs["confidence"].(float64); ok {
			confidence = c
		}
	}

	return &protocol.TaskResult{
		TaskID:     uuid.NewString(),
		Agent:      "gos_engine",
		Status:     protocol.ResultStatusSucceeded,
		Summary:    summary,
		Confidence: confidence,
		Evidence:   e.collectEvidence(graph),
		Metadata: map[string]any{
			"belief_graph": graph.ToDict(),
			"fsm_history":  fsm.History,
			"frontier":     frontier,
			"llm_calls":    stats.LLMCalls,
			"tool_calls":   stats.ToolCalls,
			"rag_calls":    stats.RAGCalls,
			"steps":        stats.Steps,
		},
		StartedAt:  startedAt.UnixMilli(),
		FinishedAt: time.Now().UnixMilli(),
	}
}

func (e *GoSEngine) collectEvidence(graph *belief.BeliefGraph) []protocol.EvidenceItem {
	var evidence []protocol.EvidenceItem
	for _, n := range graph.GetActiveNodeCopies() {
		if n.Type == belief.NodeEvidence {
			if n.Source == nil {
				continue
			}
			item := protocol.EvidenceItem{
				SourceType: "graph",
				Title:      n.Label,
				Snippet:    n.Label,
				Score:      n.Score,
			}
			if n.Source != nil {
				item.SourceType = n.Source.SourceType
				item.SourceID = n.Source.SourceID
				item.Snippet = n.Source.SummarySnippet
			}
			if score, ok := n.Attrs["score"].(float64); ok {
				item.Score = score
			}
			evidence = append(evidence, item)
		}
	}
	return evidence
}

type PlanItem struct {
	ExpertName string
	Reason     string
}
