package gos_engine

import (
	"context"
	"sort"
	"strings"

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
	if len(p.experts) == 0 {
		p.logger.Info("plan generated", "expert_count", 0)
		return nil, nil
	}

	expertName := p.pickExpert(frontier)
	if _, ok := p.experts[expertName]; !ok {
		expertName = p.firstExpert()
	}

	plan := []PlanItem{{
		ExpertName: expertName,
		Reason:     "frontier_matched",
	}}
	p.logger.Info("plan generated", "expert_count", len(plan))
	return plan, nil
}

func (p *Planner) pickExpert(frontier *belief.Frontier) string {
	text := ""
	if frontier != nil {
		text = strings.ToLower(frontier.Label + " " + frontier.Why)
	}
	switch {
	case strings.Contains(text, "网络") || strings.Contains(text, "network") || strings.Contains(text, "latency") || strings.Contains(text, "timeout"):
		return "network_sre"
	case strings.Contains(text, "数据库") || strings.Contains(text, "mysql") || strings.Contains(text, "redis") || strings.Contains(text, "cache") || strings.Contains(text, "kafka"):
		return "database_sre"
	default:
		return "linux_sre"
	}
}

func (p *Planner) firstExpert() string {
	names := make([]string, 0, len(p.experts))
	for name := range p.experts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names[0]
}
