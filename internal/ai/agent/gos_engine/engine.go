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
	cfg                *Config
	experts            map[string]experts.ExpertAgent
	logger             Logger
	structuredGenerate StructuredGenerateFunc
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
	LLMCalls         int
	ToolCalls        int
	RAGCalls         int
	Steps            int
	ExpertDegraded   int
	ExpertFailed     int
	NoProgressRounds int
	RemainingBudget  PlanBudgetConfig
	PhaseLatencyMs   map[string]int64
	FrontierChanges  int
	BacktrackCount   int
	NewEvidenceCount int
	ConfidenceDelta  float64
	Graph            belief.GraphResourceStats
}

func NewGoSEngine(cfg *Config, logger Logger) *GoSEngine {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	generate := cfg.StructuredGenerate
	if generate == nil && cfg.StructuredCognition.Enabled {
		generate = newStructuredGenerate(cfg)
	}
	return &GoSEngine{
		experts:            make(map[string]experts.ExpertAgent),
		cfg:                cfg,
		logger:             logger,
		structuredGenerate: generate,
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

func (e *GoSEngine) emitUserState(ctx context.Context, stateKind string, detail string, payload map[string]any) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["state_kind"] = stateKind
	e.emit(ctx, "state_transition", detail, payload)
}

func (e *GoSEngine) Run(ctx context.Context, symptom string) (result *protocol.TaskResult) {
	startedAt := time.Now()
	stats := newRunStats()
	graph := belief.NewBeliefGraphWithPolicy(e.cfg.ToGraphPolicy())
	fsm := belief.NewBeliefFSM(e.cfg.ToFSMThresholds())
	defer func() {
		if recovered := recover(); recovered != nil {
			result = e.degradedResult(graph, fsm, startedAt, stats, "panic_recovered", fmt.Errorf("GoS recovered panic: %v", recovered), nil, false)
		}
	}()
	if err := e.cfg.ValidateGraphConfig(); err != nil {
		return e.degradedResult(graph, fsm, startedAt, stats, "graph_config_invalid", err, nil, false)
	}

	planningHistory := NewPlanningHistory()
	sessionBudgetFactor := e.cfg.SessionMaxSteps * e.cfg.StructuredCognition.MaxPlanItems
	if sessionBudgetFactor <= 0 {
		sessionBudgetFactor = 1
	}
	planningHistory.RemainingBudget = scalePlanBudget(e.cfg.StructuredCognition.PlanBudget, sessionBudgetFactor)
	stats.RemainingBudget = planningHistory.RemainingBudget
	noProgressRounds := 0

	e.emit(ctx, "ingest", "解析症状并建立候选假设", nil)
	phaseStartedAt := time.Now()
	ingestOutcome, err := e.ingest(ctx, graph, symptom)
	stats.addPhaseLatency("ingest", time.Since(phaseStartedAt))
	stats.LLMCalls += ingestOutcome.LLMCalls
	if err != nil {
		return e.degradedResult(graph, fsm, startedAt, stats, graphFailureReason("ingest_failed", err), err, nil, false)
	}
	stats.observeGraph(graph)
	e.emit(ctx, "ingest_done", fmt.Sprintf("已抽取 %d 个候选节点", len(graph.Nodes)), map[string]any{
		"node_count":        len(graph.Nodes),
		"observation_count": ingestOutcome.ObservationCount,
		"hypothesis_count":  ingestOutcome.HypothesisCount,
		"mode":              ingestOutcome.Mode,
		"fallback_reason":   ingestOutcome.FallbackReason,
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
		progressBefore := captureGraphProgress(graph, frontier)

		e.emit(ctx, "frontier_selected", fmt.Sprintf("选中 frontier: %s (score=%.2f)", frontier.Label, frontier.Score), map[string]any{
			"frontier_label": frontier.Label,
			"frontier_score": frontier.Score,
			"fsm_level":      fsm.GetCurrentLevel(),
		})
		e.emitUserState(ctx, "explore", "探索当前最优候选", map[string]any{
			"frontier_id": frontier.NodeID,
			"level":       frontier.Level,
		})

		phaseStartedAt = time.Now()
		planOutcome, err := e.plan(ctx, graph, frontier, planningHistory)
		stats.addPhaseLatency("plan", time.Since(phaseStartedAt))
		stats.LLMCalls += planOutcome.LLMCalls
		if err != nil {
			return e.degradedResult(graph, fsm, startedAt, stats, "plan_failed", err, nil, false)
		}
		plan := planOutcome.Items
		reservedBudget := PlanBudgetConfig{}
		for _, item := range plan {
			reservedBudget = addPlanBudgets(reservedBudget, item.Budget)
		}
		planningHistory.RemainingBudget, err = subtractPlanBudget(planningHistory.RemainingBudget, reservedBudget)
		if err != nil {
			return e.degradedResult(graph, fsm, startedAt, stats, "plan_budget_failed", err, nil, false)
		}
		stats.RemainingBudget = planningHistory.RemainingBudget
		for _, item := range plan {
			planningHistory.CalledGoalKeys[planGoalKey(item)] = struct{}{}
		}

		expertNames := make([]string, 0, len(plan))
		for _, p := range plan {
			expertNames = append(expertNames, p.ExpertName)
		}
		e.emit(ctx, "expert_planned", fmt.Sprintf("调度 %d 位专家: %v", len(plan), expertNames), map[string]any{
			"expert_count":    len(plan),
			"expert_names":    expertNames,
			"plan_items":      plan,
			"mode":            planOutcome.Mode,
			"fallback_reason": planOutcome.FallbackReason,
			"fallback_detail": planOutcome.FallbackDetail,
		})

		phaseStartedAt = time.Now()
		actRes, err := e.act(ctx, plan, frontier, graph, stats)
		stats.addPhaseLatency("act", time.Since(phaseStartedAt))
		if actRes != nil {
			stats.ExpertDegraded += actRes.DegradedCount
			stats.ExpertFailed += actRes.FailedCount
		}

		alreadyUpdated := false
		if actRes != nil && len(actRes.Analyses) > 0 {
			for _, analysis := range actRes.Analyses {
				if analysis == nil {
					continue
				}
				for _, toolErr := range analysis.ToolErrors {
					if strings.TrimSpace(toolErr.ToolName) != "" {
						planningHistory.FailedTools[toolErr.ToolName] = struct{}{}
					}
				}
			}
			phaseStartedAt = time.Now()
			res := e.updateGraph(ctx, graph, actRes.Analyses, frontier)
			stats.addPhaseLatency("update", time.Since(phaseStartedAt))
			if !res.Committed {
				return e.degradedResult(graph, fsm, startedAt, stats, graphFailureReason("update_failed", res.Error), res.Error, actRes, false)
			}
			alreadyUpdated = true
			e.emit(ctx, "evidence_attached", fmt.Sprintf("挂载 %d 条证据, %d 失败", len(actRes.Analyses), actRes.FailedCount), map[string]any{
				"evidence_count": len(actRes.Analyses),
				"failed_count":   actRes.FailedCount,
				"degraded_count": actRes.DegradedCount,
			})
		}

		if err != nil {
			return e.degradedResult(graph, fsm, startedAt, stats, "act_failed", err, actRes, alreadyUpdated)
		}

		progressAfter := captureGraphProgress(graph, graph.ExtractFrontier(fsm.GetCurrentLevel()))
		stats.observeProgress(progressBefore, progressAfter)
		stats.observeGraph(graph)
		if progressAfter.progressedFrom(progressBefore) {
			noProgressRounds = 0
			stats.NoProgressRounds = 0
		} else {
			noProgressRounds++
			stats.NoProgressRounds = noProgressRounds
			e.emit(ctx, "no_progress", fmt.Sprintf("连续 %d 轮没有新增节点、有效证据或 frontier 变化", noProgressRounds), map[string]any{
				"rounds": noProgressRounds,
				"limit":  e.cfg.Execution.NoProgressRoundLimit,
			})
			if e.cfg.Execution.NoProgressRoundLimit > 0 && noProgressRounds >= e.cfg.Execution.NoProgressRoundLimit {
				return e.degradedResult(graph, fsm, startedAt, stats, "no_progress_loop", fmt.Errorf("no graph progress for %d consecutive rounds", noProgressRounds), actRes, true)
			}
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

		if e.cfg.StateConversion.Enabled {
			phaseStartedAt = time.Now()
			converter := NewStateConverter(e.cfg)
			decision := converter.Decide(graph, fsm, ConversionBudget{
				UsedSteps: stats.Steps,
				MaxSteps:  e.cfg.SessionMaxSteps,
			})
			e.emit(ctx, "state_decision", fmt.Sprintf("StateConverter 决策: %s", decision.Action), map[string]any{
				"action":      decision.Action,
				"reason_code": decision.ReasonCode,
				"reason":      decision.Reason,
				"from_level":  decision.FromLevel,
				"to_level":    decision.ToLevel,
				"frontier_id": decision.FrontierID,
				"total_steps": fsm.TotalSteps,
			})
			if err := converter.Apply(graph, fsm, decision); err != nil {
				stats.addPhaseLatency("state_conversion", time.Since(phaseStartedAt))
				return e.degradedResult(graph, fsm, startedAt, stats, graphFailureReason("state_conversion_failed", err), err, nil, true)
			}
			stats.addPhaseLatency("state_conversion", time.Since(phaseStartedAt))
			switch decision.Action {
			case DecisionRefine:
				e.emitUserState(ctx, "drill_down", "进入更细粒度的根因候选", map[string]any{"from_level": decision.FromLevel, "to_level": decision.ToLevel, "frontier_id": decision.FrontierID})
			case DecisionBacktrack:
				stats.BacktrackCount++
				e.emitUserState(ctx, "backtrack", "证据变化触发路径回溯", map[string]any{"from_level": decision.FromLevel, "to_level": decision.ToLevel, "frontier_id": decision.FrontierID})
			}
			switch decision.Action {
			case DecisionReport:
				goto DONE
			case DecisionDegraded:
				return e.degradedResult(graph, fsm, startedAt, stats, decision.ReasonCode, fmt.Errorf("%s", decision.Reason), nil, true)
			case DecisionContinue, DecisionRefine, DecisionBacktrack:
				continue
			default:
				return e.degradedResult(graph, fsm, startedAt, stats, "unknown_state_decision", fmt.Errorf("unsupported action %q", decision.Action), nil, true)
			}
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
			e.emitUserState(ctx, "drill_down", "当前候选仍需继续钻取", map[string]any{"from_level": fsm.CurrentLevel, "to_level": fsm.CurrentLevel + 1})
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
	e.emitUserState(ctx, "report", "生成证据化诊断报告", map[string]any{"total_steps": stats.Steps})
	e.emit(ctx, "report", "生成信念报告", map[string]any{
		"total_steps": stats.Steps,
		"llm_calls":   stats.LLMCalls,
		"tool_calls":  stats.ToolCalls,
		"rag_calls":   stats.RAGCalls,
	})
	return e.generateReport(ctx, graph, fsm, startedAt, stats)
}

type graphProgress struct {
	activeNodes    int
	activeEvidence int
	frontierID     string
	frontierScore  float64
	supports       int
	refutes        int
}

func captureGraphProgress(graph *belief.BeliefGraph, frontier *belief.Frontier) graphProgress {
	progress := graphProgress{}
	if graph != nil {
		for _, node := range graph.GetActiveNodeCopies() {
			progress.activeNodes++
			if node.Type == belief.NodeEvidence {
				progress.activeEvidence++
			}
		}
	}
	if frontier != nil {
		progress.frontierID = frontier.NodeID
		progress.frontierScore = frontier.Score
		progress.supports = frontier.Supports
		progress.refutes = frontier.Refutes
	}
	return progress
}

func (p graphProgress) progressedFrom(before graphProgress) bool {
	return p.activeNodes > before.activeNodes ||
		p.activeEvidence > before.activeEvidence ||
		p.frontierID != before.frontierID ||
		p.frontierScore != before.frontierScore ||
		p.supports != before.supports ||
		p.refutes != before.refutes
}

func (e *GoSEngine) ingest(ctx context.Context, graph *belief.BeliefGraph, symptom string) (IngestOutcome, error) {
	ingestor := NewStructuredIngestor(graph, e.cfg, e.logger, e.structuredGenerate)
	return ingestor.IngestWithOutcome(ctx, symptom)
}

func (e *GoSEngine) plan(ctx context.Context, graph *belief.BeliefGraph, frontier *belief.Frontier, history *PlanningHistory) (PlanOutcome, error) {
	planner := NewStructuredPlanner(e.experts, e.cfg, e.logger, e.structuredGenerate)
	return planner.PlanWithContext(ctx, PlanningContext{
		Frontier:        frontier,
		Graph:           graph,
		CalledGoalKeys:  history.CalledGoalKeys,
		FailedTools:     history.FailedTools,
		RemainingBudget: history.RemainingBudget,
	})
}

func (e *GoSEngine) act(ctx context.Context, plan []PlanItem, frontier *belief.Frontier, graph *belief.BeliefGraph, stats *RunStats) (*ActResult, error) {
	analyses := make([]*experts.ExpertAnalysis, len(plan))
	var mu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)
	concurrency := e.cfg.Execution.MaxConcurrentExperts
	if concurrency <= 0 {
		concurrency = 1
	}
	g.SetLimit(concurrency)

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

			analysis := e.runExpertSafely(gCtx, agent, item, frontier, graph)
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
		if a == nil {
			continue
		}
		switch a.Status {
		case "degraded":
			result.DegradedCount++
		case "failed":
			result.FailedCount++
		}
		if e.logger != nil {
			supports, refutes, neutral := 0, 0, 0
			for _, evidence := range a.Evidence {
				switch evidence.Relation {
				case experts.EvidenceRelationSupport:
					supports++
				case experts.EvidenceRelationRefute:
					refutes++
				default:
					neutral++
				}
			}
			e.logger.Info("expert completed",
				"expert", a.ExpertName,
				"status", a.Status,
				"degradation_reason", a.DegradationReason,
				"evidence_count", len(a.Evidence),
				"support_count", supports,
				"refute_count", refutes,
				"neutral_count", neutral,
				"refinement_count", len(a.Refinements),
				"tool_error_count", len(a.ToolErrors),
			)
		}
	}

	if result.FailedCount == len(plan) || result.FailedCount+result.DegradedCount == len(plan) && !hasUsablePartialEvidence(analyses) {
		return result, fmt.Errorf("all experts failed or degraded (%d failed, %d degraded)", result.FailedCount, result.DegradedCount)
	}

	return result, nil
}

func (e *GoSEngine) runExpertSafely(
	ctx context.Context,
	agent experts.ExpertAgent,
	item PlanItem,
	frontier *belief.Frontier,
	graph *belief.BeliefGraph,
) (analysis *experts.ExpertAnalysis) {
	defer func() {
		if recovered := recover(); recovered != nil {
			analysis = &experts.ExpertAnalysis{
				ExpertName:        item.ExpertName,
				Status:            "failed",
				DegradationReason: "expert_panic_recovered",
				ToolErrors: []experts.ToolError{{
					ToolName: "expert",
					Action:   "panic_recovery",
					Error:    fmt.Sprintf("expert panic recovered: %v", recovered),
				}},
			}
		}
	}()
	if planned, ok := agent.(experts.PlannedExpertAgent); ok {
		frontierCopy := *frontier
		return planned.RunPlanned(ctx, experts.ExpertTask{
			Frontier:         &frontierCopy,
			Graph:            buildExpertGraphView(graph, frontier),
			ExpectedEvidence: append([]string(nil), item.ExpectedEvidence...),
			AllowedTools:     append([]string(nil), item.AllowedTools...),
			StopConditions:   append([]string(nil), item.StopConditions...),
			Budget:           toExpertExecutionBudget(item.Budget),
		})
	}
	return agent.Run(ctx, frontier, graph)
}

func toExpertExecutionBudget(budget PlanBudgetConfig) experts.ExecutionBudget {
	return experts.ExecutionBudget{
		LLMCalls:          budget.LLMCalls,
		ToolCalls:         budget.ToolCalls,
		RAGCalls:          budget.RAGCalls,
		Timeout:           time.Duration(budget.TimeoutMs) * time.Millisecond,
		MaxRetrievalSteps: budget.MaxRetrievalSteps,
		MaxOutputTokens:   budget.MaxOutputTokens,
	}
}

func hasUsablePartialEvidence(analyses []*experts.ExpertAnalysis) bool {
	for _, analysis := range analyses {
		if analysis == nil {
			continue
		}
		for _, evidence := range analysis.Evidence {
			if strings.TrimSpace(evidence.SourceType) != "" && strings.TrimSpace(evidence.SourceID) != "" {
				return true
			}
		}
	}
	return false
}

func buildExpertGraphView(graph *belief.BeliefGraph, frontier *belief.Frontier) *belief.BeliefGraph {
	view := belief.NewBeliefGraph()
	if graph == nil || frontier == nil {
		return view
	}
	nodes := graph.GetActiveNodeCopies()
	edges := graph.GetActiveEdgeCopies()
	byID := make(map[string]belief.Node, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	included := map[string]struct{}{frontier.NodeID: {}}
	if graph.StartSignalID != "" {
		included[graph.StartSignalID] = struct{}{}
	}
	changed := true
	for changed {
		changed = false
		for _, edge := range edges {
			_, dstIncluded := included[edge.Dst]
			if dstIncluded && (edge.Type == belief.EdgeRefines || edge.Type == belief.EdgeCausal) {
				if _, exists := included[edge.Src]; !exists {
					included[edge.Src] = struct{}{}
					changed = true
				}
			}
		}
		for _, node := range nodes {
			if node.Type != belief.NodeEvidence || node.Source == nil {
				continue
			}
			if _, targetIncluded := included[node.Source.TargetHypothesisID]; targetIncluded {
				if _, exists := included[node.ID]; !exists {
					included[node.ID] = struct{}{}
					changed = true
				}
			}
		}
	}
	for nodeID := range included {
		node, exists := byID[nodeID]
		if !exists {
			continue
		}
		copied := node
		view.Nodes[nodeID] = &copied
	}
	for _, edge := range edges {
		if _, ok := view.Nodes[edge.Src]; !ok {
			continue
		}
		if _, ok := view.Nodes[edge.Dst]; !ok {
			continue
		}
		copied := edge
		view.Edges[edge.Src+"->"+edge.Dst] = &copied
	}
	if _, ok := view.Nodes[graph.StartSignalID]; ok {
		view.StartSignalID = graph.StartSignalID
	}
	view.Belief = graph.Belief
	view.CurrentStep = graph.CurrentStep
	return view
}

func (e *GoSEngine) shouldReport(frontier *belief.Frontier) bool {
	return frontier.Score >= e.cfg.FSM.MinConfidence && frontier.Supports >= e.cfg.FSM.MinSupport
}

func (e *GoSEngine) degradedResult(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, startedAt time.Time, stats *RunStats, reason string, err error, actRes *ActResult, alreadyUpdated bool) *protocol.TaskResult {
	e.emitUserState(context.Background(), "degraded", "诊断进入降级路径", map[string]any{"reason_code": reason})
	if !alreadyUpdated && actRes != nil && len(actRes.Analyses) > 0 {
		if f := graph.ExtractFrontier(fsm.GetCurrentLevel()); f != nil {
			e.updateGraph(context.Background(), graph, actRes.Analyses, f)
		}
	}
	degradedDetail, _ := e.degradedSummary(reason, err, actRes)
	frontier := selectReportFrontier(graph, fsm)
	report := e.buildEvidenceReport(graph, frontier, stats, "执行降级："+degradedDetail)

	result := &protocol.TaskResult{
		TaskID:            uuid.NewString(),
		Agent:             "gos_engine",
		Status:            protocol.ResultStatusDegraded,
		Summary:           formatEvidenceReport(report),
		Confidence:        report.Confidence,
		DegradationReason: fmt.Sprintf("%s: %v", reason, err),
		Evidence:          report.protocolEvidence(),
		NextActions:       report.NextActions,
		Metadata: map[string]any{
			"belief_graph":          graph.ToDict(),
			"fsm_history":           fsm.History,
			"graph_valid":           validateBeliefGraph(graph) == nil,
			"error_phase":           reason,
			"llm_calls":             stats.LLMCalls,
			"tool_calls":            stats.ToolCalls,
			"rag_calls":             stats.RAGCalls,
			"steps":                 stats.Steps,
			"expert_degraded_count": stats.ExpertDegraded,
			"expert_failed_count":   stats.ExpertFailed,
			"no_progress_rounds":    stats.NoProgressRounds,
			"remaining_budget":      stats.RemainingBudget,
			"evidence_report":       report,
		},
		StartedAt:  startedAt.UnixMilli(),
		FinishedAt: time.Now().UnixMilli(),
	}
	return e.finalizeResult(result, graph, startedAt, stats)
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
	reportStartedAt := time.Now()
	if err := validateBeliefGraph(graph); err != nil {
		stats.addPhaseLatency("report", time.Since(reportStartedAt))
		return e.degradedResult(graph, fsm, startedAt, stats, "graph_invalid", err, nil, true)
	}
	frontier := selectReportFrontier(graph, fsm)
	if frontier == nil {
		stats.addPhaseLatency("report", time.Since(reportStartedAt))
		return e.degradedResult(graph, fsm, startedAt, stats, "no_frontier", fmt.Errorf("no frontier found"), nil, false)
	}
	report := e.buildEvidenceReport(graph, frontier, stats)
	status := protocol.ResultStatusSucceeded
	degradationReason := ""
	if !report.Sufficient {
		status = protocol.ResultStatusDegraded
		degradationReason = "evidence_report_insufficient: " + strings.Join(report.ReasonCodes, ",")
		e.emitUserState(ctx, "degraded", "证据不足或存在关键冲突，报告已降级", map[string]any{"reason_codes": report.ReasonCodes})
	}

	result := &protocol.TaskResult{
		TaskID:            uuid.NewString(),
		Agent:             "gos_engine",
		Status:            status,
		Summary:           formatEvidenceReport(report),
		Confidence:        report.Confidence,
		DegradationReason: degradationReason,
		Evidence:          report.protocolEvidence(),
		NextActions:       report.NextActions,
		Metadata: map[string]any{
			"belief_graph":          graph.ToDict(),
			"fsm_history":           fsm.History,
			"graph_valid":           true,
			"frontier":              frontier,
			"llm_calls":             stats.LLMCalls,
			"tool_calls":            stats.ToolCalls,
			"rag_calls":             stats.RAGCalls,
			"steps":                 stats.Steps,
			"expert_degraded_count": stats.ExpertDegraded,
			"expert_failed_count":   stats.ExpertFailed,
			"no_progress_rounds":    stats.NoProgressRounds,
			"remaining_budget":      stats.RemainingBudget,
			"evidence_report":       report,
		},
		StartedAt:  startedAt.UnixMilli(),
		FinishedAt: time.Now().UnixMilli(),
	}
	stats.addPhaseLatency("report", time.Since(reportStartedAt))
	return e.finalizeResult(result, graph, startedAt, stats)
}

type PlanItem struct {
	ExpertName         string           `json:"expert_name"`
	TargetHypothesisID string           `json:"target_hypothesis_id,omitempty"`
	Reason             string           `json:"reason"`
	ExpectedEvidence   []string         `json:"expected_evidence"`
	AllowedTools       []string         `json:"allowed_tools"`
	StopConditions     []string         `json:"stop_conditions"`
	Budget             PlanBudgetConfig `json:"budget"`
	FallbackReason     string           `json:"fallback_reason,omitempty"`
}
