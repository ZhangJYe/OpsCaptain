package rag

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type PlannerConfig struct {
	Enabled               bool
	Model                 string
	TimeoutMs             int
	MaxSubQueries         int
	MinQueryLength        int
	ExecTimeoutMs         int
	DecompositionKeywords []string
}

func DefaultPlannerConfig() PlannerConfig {
	return PlannerConfig{
		Enabled:           false,
		Model:             "chat_model_fast",
		TimeoutMs:         5000,
		MaxSubQueries:     4,
		MinQueryLength:    15,
		ExecTimeoutMs:     5000,
		DecompositionKeywords: []string{
			"和", "以及", "还有", "跟", "为什么", "怎么回事", "导致", "关系",
		},
	}
}

func LoadPlannerConfig(ctx context.Context) PlannerConfig {
	cfg := DefaultPlannerConfig()
	v, err := g.Cfg().Get(ctx, "rag.planner.enabled")
	if err == nil && !v.IsNil() {
		cfg.Enabled = v.Bool()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.model")
	if err == nil && v.String() != "" {
		cfg.Model = v.String()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.TimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.max_sub_queries")
	if err == nil && v.Int() > 0 {
		cfg.MaxSubQueries = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.min_query_length")
	if err == nil && v.Int() > 0 {
		cfg.MinQueryLength = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.exec_timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.ExecTimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.decomposition_keywords")
	if err == nil && v.Strings() != nil && len(v.Strings()) > 0 {
		cfg.DecompositionKeywords = v.Strings()
	}
	return cfg
}

func plannerTimeout(ctx context.Context, cfg PlannerConfig) time.Duration {
	return time.Duration(cfg.TimeoutMs) * time.Millisecond
}

func plannerExecTimeout(ctx context.Context, cfg PlannerConfig) time.Duration {
	return time.Duration(cfg.ExecTimeoutMs) * time.Millisecond
}
