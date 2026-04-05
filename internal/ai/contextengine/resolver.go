package contextengine

import (
	"context"

	"SuperBizAgent/utility/mem"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultChatMaxHistoryMessages = 10
	defaultChatMaxMemoryItems     = 5
	defaultAIOpsMaxMemoryItems    = 5
	defaultReporterMaxToolItems   = 8
)

type PolicyResolver struct{}

func NewPolicyResolver() *PolicyResolver {
	return &PolicyResolver{}
}

func (r *PolicyResolver) Resolve(ctx context.Context, req ContextRequest) ContextProfile {
	budget := mem.GetTokenBudget()
	base := ContextProfile{
		Name:               "chat-default",
		AllowHistory:       true,
		AllowMemory:        true,
		AllowDocs:          true,
		AllowToolResults:   false,
		Staged:             true,
		MaxHistoryMessages: loadPositiveInt(ctx, "context.chat_max_history_messages", defaultChatMaxHistoryMessages),
		MaxMemoryItems:     loadPositiveInt(ctx, "context.chat_max_memory_items", defaultChatMaxMemoryItems),
		MaxToolItems:       0,
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
	case "aiops", "chat_multi_agent", "specialist":
		base.Name = "aiops-default"
		base.AllowHistory = false
		base.AllowDocs = false
		base.AllowToolResults = false
		base.Staged = false
		base.MaxHistoryMessages = 0
		base.MaxMemoryItems = loadPositiveInt(ctx, "context.aiops_max_memory_items", defaultAIOpsMaxMemoryItems)
		base.Budget.HistoryTokens = 0
		base.Budget.ToolTokens = 0
	case "reporter":
		base.Name = "reporter-default"
		base.AllowHistory = false
		base.AllowMemory = false
		base.AllowDocs = false
		base.AllowToolResults = true
		base.Staged = false
		base.MaxHistoryMessages = 0
		base.MaxMemoryItems = 0
		base.MaxToolItems = loadPositiveInt(ctx, "context.reporter_max_tool_items", defaultReporterMaxToolItems)
		base.Budget.HistoryTokens = 0
		base.Budget.MemoryTokens = 0
		base.Budget.DocumentTokens = 0
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
