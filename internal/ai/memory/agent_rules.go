package memory

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type RuleMemoryAgent struct{}

func NewRuleMemoryAgent() *RuleMemoryAgent {
	return &RuleMemoryAgent{}
}

func (a *RuleMemoryAgent) Decide(ctx context.Context, event MemoryEvent) (*MemoryDecision, error) {
	if ctx != nil && ctx.Err() != nil {
		return &MemoryDecision{}, ctx.Err()
	}
	actions := make([]MemoryAction, 0)
	for _, candidate := range ExtractMemoryCandidates(event.Query, event.Answer) {
		action := MemoryAction{
			Op:            MemoryActionUpsert,
			Type:          candidate.Type,
			Content:       candidate.Content,
			Scope:         candidate.Scope,
			ScopeID:       candidate.ScopeID,
			Confidence:    candidate.Confidence,
			ConflictGroup: inferMemoryConflictGroup(candidate),
			Reason:        "rule extractor produced a durable memory candidate",
		}
		if candidate.Type == MemoryTypePreference && strings.TrimSpace(event.UserID) != "" {
			action.Scope = MemoryScopeUser
			action.ScopeID = strings.TrimSpace(event.UserID)
		}
		if strings.TrimSpace(action.ScopeID) == "" {
			action.ScopeID = defaultActionScopeID(event, action.Scope)
		}
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		actions = append(actions, MemoryAction{
			Op:     MemoryActionSkip,
			Reason: "no durable memory candidate found",
		})
	}
	return &MemoryDecision{Actions: actions}, nil
}

type LLMMemoryAgent struct {
	chat     MemoryChatModel
	fallback MemoryAgent
}

func NewLLMMemoryAgent(chat MemoryChatModel, fallback MemoryAgent) *LLMMemoryAgent {
	if fallback == nil {
		fallback = NewRuleMemoryAgent()
	}
	return &LLMMemoryAgent{
		chat:     chat,
		fallback: fallback,
	}
}

func (a *LLMMemoryAgent) Decide(ctx context.Context, event MemoryEvent) (*MemoryDecision, error) {
	if a == nil || a.chat == nil {
		return a.fallbackDecision(ctx, event)
	}
	if ctx != nil && ctx.Err() != nil {
		return &MemoryDecision{}, ctx.Err()
	}
	event = withExistingMemories(normalizeMemoryEvent(event))
	resp, err := a.chat.Generate(ctx, []*schema.Message{
		schema.SystemMessage(memoryAgentSystemPrompt()),
		schema.UserMessage(memoryAgentUserPrompt(event)),
	})
	if err != nil {
		return a.fallbackDecision(memoryFallbackContext(ctx), event)
	}
	if resp == nil {
		return a.fallbackDecision(memoryFallbackContext(ctx), event)
	}
	decision, err := parseMemoryDecisionJSON(resp.Content)
	if err != nil {
		return a.fallbackDecision(memoryFallbackContext(ctx), event)
	}
	return sanitizeMemoryDecision(decision, event), nil
}

func (a *LLMMemoryAgent) fallbackDecision(ctx context.Context, event MemoryEvent) (*MemoryDecision, error) {
	if a != nil && a.fallback != nil {
		return a.fallback.Decide(ctx, event)
	}
	return NewRuleMemoryAgent().Decide(ctx, event)
}

func memoryFallbackContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if ctx.Err() != nil {
		return context.WithoutCancel(ctx)
	}
	return ctx
}