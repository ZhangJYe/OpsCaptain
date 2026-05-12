package gos_engine

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

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

func (e *GoSEngine) Run(ctx context.Context, symptom string) *protocol.TaskResult {
	startedAt := time.Now()
	stats := &RunStats{}

	graph := belief.NewBeliefGraph()
	fsm := belief.NewBeliefFSM(e.cfg.ToFSMThresholds())

	if err := e.ingest(ctx, graph, symptom); err != nil {
		return e.degradedResult(graph, fsm, startedAt, stats, "ingest_failed", err, nil, false)
	}
	stats.LLMCalls++

	for {
		if fsm.IsFinalState() {
			break
		}

		frontier := graph.ExtractFrontier(fsm.GetCurrentLevel())
		if frontier == nil {
			fsm.MarkDone("no frontier")
			break
		}

		plan, err := e.plan(ctx, frontier)
		if err != nil {
			return e.degradedResult(graph, fsm, startedAt, stats, "plan_failed", err, nil, false)
		}
		stats.LLMCalls++

		actRes, err := e.act(ctx, plan, frontier, graph, stats)

		alreadyUpdated := false
		if actRes != nil && len(actRes.Analyses) > 0 {
			if res := e.updateGraph(ctx, graph, actRes.Analyses, frontier); res.Committed {
				alreadyUpdated = true
			}
		}

		if err != nil {
			return e.degradedResult(graph, fsm, startedAt, stats, "act_failed", err, actRes, alreadyUpdated)
		}

		graph.GenerateBeliefText()
		fsm.TickStep(1)
		stats.Steps++

		updatedFrontier := graph.ExtractFrontier(fsm.GetCurrentLevel())

		decision := fsm.Decide(graph)
		stats.LLMCalls++
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
	result := &ActResult{
		Analyses: make([]*experts.ExpertAnalysis, 0, len(plan)),
	}

	for _, item := range plan {
		agent, exists := e.experts[item.ExpertName]
		if !exists {
			result.Analyses = append(result.Analyses, &experts.ExpertAnalysis{
				ExpertName:        item.ExpertName,
				Status:            "failed",
				DegradationReason: "expert_not_found",
			})
			result.FailedCount++
			continue
		}

		analysis := agent.Run(ctx, frontier, graph)
		if analysis == nil {
			analysis = &experts.ExpertAnalysis{
				ExpertName:        item.ExpertName,
				Status:            "failed",
				DegradationReason: "expert_nil_result",
			}
		}

		result.Analyses = append(result.Analyses, analysis)
		stats.LLMCalls += len(analysis.ToolErrors) + 1
		stats.ToolCalls += countToolCalls(analysis)
		stats.RAGCalls += countRAGCalls(analysis)

		switch analysis.Status {
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

func countToolCalls(analysis *experts.ExpertAnalysis) int {
	count := 0
	for _, ev := range analysis.Evidence {
		if ev.SourceType == "tool" {
			count++
		}
	}
	return count
}

func countRAGCalls(analysis *experts.ExpertAnalysis) int {
	count := 0
	for _, ev := range analysis.Evidence {
		if ev.SourceType == "rag" {
			count++
		}
	}
	return count
}

func (e *GoSEngine) updateGraph(ctx context.Context, graph *belief.BeliefGraph, analyses []*experts.ExpertAnalysis, frontier *belief.Frontier) *belief.GraphUpdateResult {
	return graph.UpdateCopyOnWrite(func(cp *belief.BeliefGraph) error {
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
			if a.Confidence > 0 {
				cp.UpdateNodeCopy(frontier.NodeID, a.Confidence, a.Analysis)
			}
		}
		return nil
	})
}

func (e *GoSEngine) shouldReport(frontier *belief.Frontier) bool {
	return frontier.Score >= 0.7 && frontier.Supports >= 2
}

func (e *GoSEngine) degradedResult(graph *belief.BeliefGraph, fsm *belief.BeliefFSM, startedAt time.Time, stats *RunStats, reason string, err error, actRes *ActResult, alreadyUpdated bool) *protocol.TaskResult {
	if !alreadyUpdated && actRes != nil && len(actRes.Analyses) > 0 {
		if f := graph.ExtractFrontier(fsm.GetCurrentLevel()); f != nil {
			e.updateGraph(context.Background(), graph, actRes.Analyses, f)
		}
	}

	return &protocol.TaskResult{
		TaskID:            uuid.NewString(),
		Agent:             "gos_engine",
		Status:            protocol.ResultStatusDegraded,
		Summary:           "诊断降级",
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

func (e *GoSEngine) generateReport(ctx context.Context, graph *belief.BeliefGraph, fsm *belief.BeliefFSM, startedAt time.Time, stats *RunStats) *protocol.TaskResult {
	frontier := graph.ExtractFrontier(fsm.GetCurrentLevel())
	if frontier == nil {
		return e.degradedResult(graph, fsm, startedAt, stats, "no_frontier", fmt.Errorf("no frontier found"), nil, false)
	}

	return &protocol.TaskResult{
		TaskID:     uuid.NewString(),
		Agent:      "gos_engine",
		Status:     protocol.ResultStatusSucceeded,
		Summary:    frontier.Label,
		Confidence: frontier.Score,
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
