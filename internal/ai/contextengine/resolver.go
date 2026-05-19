package contextengine

import (
	"context"
	"time"

	"SuperBizAgent/utility/mem"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultChatMaxHistoryMessages = 10
	defaultChatMaxMemoryItems     = 5
	defaultAIOpsMaxMemoryItems    = 5
	defaultReporterMaxToolItems   = 8
	defaultMinMemoryConfidence    = 0.50
)

type PolicyResolver struct{}

func NewPolicyResolver() *PolicyResolver {
	return &PolicyResolver{}
}

func (r *PolicyResolver) Resolve(ctx context.Context, req ContextRequest) ContextProfile {
	budget := mem.GetTokenBudget()
	base := ContextProfile{
		Name:                "chat-default",
		AllowHistory:        true,
		AllowMemory:         true,
		AllowDocs:           true,
		AllowToolResults:    false,
		Staged:              true,
		MaxHistoryMessages:  loadPositiveInt(ctx, "context.chat_max_history_messages", defaultChatMaxHistoryMessages),
		MaxMemoryItems:      loadPositiveInt(ctx, "context.chat_max_memory_items", defaultChatMaxMemoryItems),
		MaxToolItems:        0,
		MinMemoryConfidence: loadPositiveFloat(ctx, "context.min_memory_confidence", defaultMinMemoryConfidence),
		AllowedMemoryScopes: []string{"session", "user", "project", "global"},
		Budget: ContextBudget{
			MaxTotalTokens: budget.MaxTokens,
			SystemTokens:   budget.SystemReserve,
			HistoryTokens:  budget.HistoryReserve,
			MemoryTokens:   budget.MemoryReserve,
			DocumentTokens: budget.DocumentReserve,
			ToolTokens:     int(float64(budget.MaxTokens) * 0.10),
			ReservedTokens: budget.MaxTokens - budget.SystemReserve - budget.HistoryReserve - budget.MemoryReserve - budget.DocumentReserve - int(float64(budget.MaxTokens)*0.10),
		},
	}

	switch req.Mode {
	case "aiops", "specialist":
		base.Name = "aiops-default"
		base.AllowHistory = false
		base.AllowDocs = false
		base.AllowToolResults = false
		base.Staged = false
		base.MaxHistoryMessages = 0
		base.MaxMemoryItems = loadPositiveInt(ctx, "context.aiops_max_memory_items", defaultAIOpsMaxMemoryItems)
		base.Budget.HistoryTokens = 0
		base.Budget.ToolTokens = 0
	case "aiops_diagnosis":
		base.Name = "aiops-diagnosis"
		base.AllowHistory = true
		base.AllowDocs = true
		base.AllowToolResults = true
		base.Staged = true
		base.MaxHistoryMessages = loadPositiveInt(ctx, "context.chat_max_history_messages", defaultChatMaxHistoryMessages)
		base.MaxMemoryItems = loadPositiveInt(ctx, "context.aiops_max_memory_items", defaultAIOpsMaxMemoryItems)
		base.MaxToolItems = loadPositiveInt(ctx, "context.reporter_max_tool_items", defaultReporterMaxToolItems)
		base.Budget.ToolTokens = int(float64(budget.MaxTokens) * 0.15)
	case "chat":
	}

	return base
}

func loadPositiveInt(ctx context.Context, key string, fallback int) int {
	v, err := g.Cfg().Get(ctx, key)
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return fallback
}

func loadPositiveFloat(ctx context.Context, key string, fallback float64) float64 {
	v, err := g.Cfg().Get(ctx, key)
	if err == nil && v.Float64() > 0 {
		return normalizeUnitFloat(v.Float64(), fallback)
	}
	return fallback
}

func normalizeUnitFloat(value, fallback float64) float64 {
	if value <= 0 || value > 1 {
		return fallback
	}
	return value
}

func (r *PolicyResolver) ResolveByProfile(ctx context.Context, req ContextRequest, profileName string) ContextProfile {
	tmpReq := req
	tmpReq.Mode = profileName
	return r.Resolve(ctx, tmpReq)
}

func LoadToolRerankConfig(ctx context.Context) *ToolRerankConfig {
	cfg := &ToolRerankConfig{
		Enabled:        false,
		MinCandidates:  defaultToolRerankMinCandidates,
		CandidateLimit: defaultToolRerankCandidateMax,
		TimeoutMs:      int(defaultToolRerankTimeout / time.Millisecond),
		CacheTTLSecs:   300,
		Model:          "chat_model_fast",
	}
	v, err := g.Cfg().Get(ctx, "context.tool_rerank.enabled")
	if err == nil && !v.IsNil() {
		cfg.Enabled = v.Bool()
	}
	v, err = g.Cfg().Get(ctx, "context.tool_rerank.min_candidates")
	if err == nil && v.Int() > 0 {
		cfg.MinCandidates = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "context.tool_rerank.candidate_limit")
	if err == nil && v.Int() > 0 {
		cfg.CandidateLimit = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "context.tool_rerank.timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.TimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "context.tool_rerank.cache_ttl_seconds")
	if err == nil && v.Int() > 0 {
		cfg.CacheTTLSecs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "context.tool_rerank.model")
	if err == nil && v.String() != "" {
		cfg.Model = v.String()
	}
	v, err = g.Cfg().Get(ctx, "context.tool_rerank.profiles")
	if err == nil && v.Strings() != nil {
		cfg.Profiles = v.Strings()
	}
	return cfg
}
