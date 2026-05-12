package gos_engine

import (
	"context"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/belief"
)

type Planner struct {
	experts map[string]experts.ExpertAgent
	cfg     *Config
	logger  Logger
}

func NewPlanner(experts map[string]experts.ExpertAgent, cfg *Config, logger Logger) *Planner {
	return &Planner{
		experts: experts,
		cfg:     cfg,
		logger:  logger,
	}
}

func (p *Planner) Plan(ctx context.Context, frontier *belief.Frontier) ([]PlanItem, error) {
	var plan []PlanItem

	for name := range p.experts {
		plan = append(plan, PlanItem{
			ExpertName: name,
			Reason:     "auto_planned",
		})
	}

	p.logger.Info("plan generated", "expert_count", len(plan))
	return plan, nil
}
