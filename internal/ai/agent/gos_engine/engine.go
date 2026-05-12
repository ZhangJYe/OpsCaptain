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
	graph     *belief.BeliefGraph
	fsm       *belief.BeliefFSM
	experts   map[string]experts.ExpertAgent
	cfg       *Config
	startedAt time.Time
	logger    Logger
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

func NewGoSEngine(cfg *Config, logger Logger) *GoSEngine {
	return &GoSEngine{
		graph:   belief.NewBeliefGraph(),
		fsm:     belief.NewBeliefFSM(cfg.ToFSMThresholds()),
		experts: make(map[string]experts.ExpertAgent),
		cfg:     cfg,
		logger:  logger,
	}
}

func (e *GoSEngine) RegisterExpert(name string, agent experts.ExpertAgent) {
	e.experts[name] = agent
}

func (e *GoSEngine) Run(ctx context.Context, symptom string) *protocol.TaskResult {
	e.startedAt = time.Now()

	if err := e.ingest(ctx, symptom); err != nil {
		return e.degradedResult("ingest_failed", err, nil, false)
	}

	for {
		if e.fsm.IsFinalState() {
			break
		}

		frontier := e.graph.ExtractFrontier(e.fsm.GetCurrentLevel())
		if frontier == nil {
			e.fsm.MarkDone("no frontier")
			break
		}

		plan, err := e.plan(ctx, frontier)
		if err != nil {
			return e.degradedResult("plan_failed", err, nil, false)
		}

		actRes, err := e.act(ctx, plan, frontier)

		alreadyUpdated := false
		if actRes != nil && len(actRes.Analyses) > 0 {
			if res := e.updateGraph(ctx, actRes.Analyses, frontier); res.Committed {
				alreadyUpdated = true
			}
		}

		if err != nil {
			return e.degradedResult("act_failed", err, actRes, alreadyUpdated)
		}

		e.graph.GenerateBeliefText()
		e.fsm.TickStep(1)

		decision := e.fsm.Decide(e.graph)
		switch decision.Action {
		case "report":
			if e.shouldReport(frontier) {
				e.fsm.MarkDone("sufficient granularity")
				goto DONE
			}
			e.fsm.DrillDown(fmt.Sprintf("drill to level %d", e.fsm.CurrentLevel+1))
		case "done":
			goto DONE
		}

		if e.fsm.TotalSteps >= e.cfg.SessionMaxSteps {
			e.fsm.MarkDone("max steps")
			break
		}
	}

DONE:
	return e.generateReport(ctx)
}

func (e *GoSEngine) ingest(ctx context.Context, symptom string) error {
	ingestor := NewIngestor(e.graph, e.logger)
	return ingestor.Ingest(ctx, symptom)
}

func (e *GoSEngine) plan(ctx context.Context, frontier *belief.Frontier) ([]PlanItem, error) {
	planner := NewPlanner(e.experts, e.cfg, e.logger)
	return planner.Plan(ctx, frontier)
}

func (e *GoSEngine) act(ctx context.Context, plan []PlanItem, frontier *belief.Frontier) (*ActResult, error) {
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

		analysis := agent.Run(ctx, frontier, e.graph)
		result.Analyses = append(result.Analyses, analysis)

		switch analysis.Status {
		case "degraded":
			result.DegradedCount++
		case "failed":
			result.FailedCount++
		}
	}

	if result.FailedCount == len(plan) {
		return result, fmt.Errorf("all experts failed (%d/%d)", result.FailedCount, len(plan))
	}

	return result, nil
}

func (e *GoSEngine) updateGraph(ctx context.Context, analyses []*experts.ExpertAnalysis, frontier *belief.Frontier) *belief.GraphUpdateResult {
	return e.graph.UpdateCopyOnWrite(func(cp *belief.BeliefGraph) error {
		for _, a := range analyses {
			for _, ev := range a.Evidence {
				src := &belief.EvidenceSource{
					SourceType:     ev.SourceType,
					SourceID:       ev.SourceID,
					SummarySnippet: ev.Snippet,
				}
				eid := cp.AddEvidenceCopy(ev.Title, src)

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

func (e *GoSEngine) degradedResult(reason string, err error, actRes *ActResult, alreadyUpdated bool) *protocol.TaskResult {
	if !alreadyUpdated && actRes != nil && len(actRes.Analyses) > 0 {
		if f := e.graph.ExtractFrontier(e.fsm.GetCurrentLevel()); f != nil {
			e.updateGraph(context.Background(), actRes.Analyses, f)
		}
	}

	return &protocol.TaskResult{
		TaskID:            uuid.NewString(),
		Agent:             "gos_engine",
		Status:            protocol.ResultStatusDegraded,
		Summary:           "诊断降级",
		DegradationReason: fmt.Sprintf("%s: %v", reason, err),
		Evidence:          e.collectEvidence(),
		Metadata: map[string]any{
			"belief_graph": e.graph.ToDict(),
			"fsm_history":  e.fsm.History,
			"error_phase":  reason,
		},
		StartedAt:  e.startedAt.UnixMilli(),
		FinishedAt: time.Now().UnixMilli(),
	}
}

func (e *GoSEngine) generateReport(ctx context.Context) *protocol.TaskResult {
	frontier := e.graph.ExtractFrontier(e.fsm.GetCurrentLevel())
	if frontier == nil {
		return e.degradedResult("no_frontier", fmt.Errorf("no frontier found"), nil, false)
	}

	return &protocol.TaskResult{
		TaskID:     uuid.NewString(),
		Agent:      "gos_engine",
		Status:     protocol.ResultStatusSucceeded,
		Summary:    frontier.Label,
		Confidence: frontier.Score,
		Evidence:   e.collectEvidence(),
		Metadata: map[string]any{
			"belief_graph": e.graph.ToDict(),
			"fsm_history":  e.fsm.History,
			"frontier":     frontier,
		},
		StartedAt:  e.startedAt.UnixMilli(),
		FinishedAt: time.Now().UnixMilli(),
	}
}

func (e *GoSEngine) collectEvidence() []protocol.EvidenceItem {
	var evidence []protocol.EvidenceItem
	for _, n := range e.graph.Nodes {
		if n.Type == belief.NodeEvidence && n.Status == belief.StatusActive {
			evidence = append(evidence, protocol.EvidenceItem{
				SourceType: "graph",
				Title:      n.Label,
				Snippet:    n.Label,
			})
		}
	}
	return evidence
}

type PlanItem struct {
	ExpertName string
	Reason     string
}
