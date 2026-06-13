package rag

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type AgentConfig struct {
	Enabled             bool
	Model               string
	MaxRounds           int
	ConfidenceThreshold float64
	EvalTimeoutMs       int
	PlanTimeoutMs       int
	TotalTimeoutMs      int
	MaxTotalTokens      int
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Enabled:             false,
		Model:               "chat_model_fast",
		MaxRounds:           3,
		ConfidenceThreshold: 0.7,
		EvalTimeoutMs:       3000,
		PlanTimeoutMs:       2000,
		TotalTimeoutMs:      30000,
		MaxTotalTokens:      8000,
	}
}

func LoadAgentConfig(ctx context.Context) AgentConfig {
	cfg := DefaultAgentConfig()

	v, err := g.Cfg().Get(ctx, "rag.agent.enabled")
	if err == nil && !v.IsNil() {
		cfg.Enabled = v.Bool()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.model")
	if err == nil && v.String() != "" {
		cfg.Model = v.String()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.max_rounds")
	if err == nil && v.Int() > 0 {
		cfg.MaxRounds = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.confidence_threshold")
	if err == nil && v.Float64() > 0 {
		cfg.ConfidenceThreshold = v.Float64()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.eval_timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.EvalTimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.plan_timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.PlanTimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.total_timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.TotalTimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.max_total_tokens")
	if err == nil && v.Int() > 0 {
		cfg.MaxTotalTokens = v.Int()
	}

	return cfg
}

func agentEvalTimeout(cfg AgentConfig) time.Duration {
	return time.Duration(cfg.EvalTimeoutMs) * time.Millisecond
}

func agentPlanTimeout(cfg AgentConfig) time.Duration {
	return time.Duration(cfg.PlanTimeoutMs) * time.Millisecond
}
