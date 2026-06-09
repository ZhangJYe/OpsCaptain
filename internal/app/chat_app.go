package app

import (
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/contextengine"
	"SuperBizAgent/internal/ai/memory"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/cache"
	"SuperBizAgent/utility/log_call_back"
	"SuperBizAgent/utility/safety"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

type sessionLockEntry struct {
	mu       sync.Mutex
	refCount int
	lastUsed time.Time
}

// ChatApp orchestrates the synchronous chat business use case.
type ChatApp struct {
	sessionLocks   map[string]*sessionLockEntry
	sessionLocksMu sync.Mutex

	buildChatAgent   func(ctx context.Context, query string) (compose.Runnable[*chat_pipeline.UserMessage, *schema.Message], error)
	degradationCheck func(ctx context.Context, entrypoint string) aiservice.DegradationDecision
}

const (
	sessionLockTTL     = 10 * time.Minute
	sessionLockCleanup = 5 * time.Minute
)

// NewChatApp creates a ChatApp with default dependencies.
func NewChatApp() *ChatApp {
	a := &ChatApp{
		sessionLocks:     make(map[string]*sessionLockEntry),
		buildChatAgent:   chat_pipeline.BuildChatAgentWithQuery,
		degradationCheck: aiservice.GetDegradationDecision,
	}
	go a.cleanupStaleSessionLocks()
	return a
}

// SetBuildChatAgent overrides the agent builder for testing.
func (a *ChatApp) SetBuildChatAgent(fn func(ctx context.Context, query string) (compose.Runnable[*chat_pipeline.UserMessage, *schema.Message], error)) {
	a.buildChatAgent = fn
}

// SetDegradationCheck overrides the degradation check for testing.
func (a *ChatApp) SetDegradationCheck(fn func(ctx context.Context, entrypoint string) aiservice.DegradationDecision) {
	a.degradationCheck = fn
}

// HandleChat executes the full synchronous chat flow.
func (a *ChatApp) HandleChat(ctx context.Context, input *ChatInput) (*ChatResult, error) {
	if d := chatTimeout(ctx); d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}

	id := input.SessionID
	msg := input.Question
	selectedSkillIDs := chat_pipeline.NormalizeSelectedSkillIDs(input.SkillIDs)

	if err := memory.ValidateSessionID(id); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, id)
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = skills.WithSelectedSkillIDs(ctx, selectedSkillIDs)
	ctx = enrichContext(ctx, id, requestID)
	selectedSkillIDs = skills.SelectedSkillIDsFromContext(ctx)

	g.Log().Infof(ctx, "[session:%s][req:%s] Chat request received, question length: %d, selected_skills=%v", id, requestID, len(msg), selectedSkillIDs)

	decision := safety.CheckPrompt(ctx, msg)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskScore, decision.RiskScore)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskLevel, decision.RiskLevel)
	if !decision.Allowed {
		g.Log().Warningf(ctx, "[prompt_guard] blocked request, pattern=%s risk_score=%.2f", decision.Pattern, decision.RiskScore)
		return nil, &PromptRejectedError{
			Reason:    decision.Reason,
			RiskScore: decision.RiskScore,
			RiskLevel: decision.RiskLevel,
			Pattern:   decision.Pattern,
		}
	}
	if decision.RiskLevel == "suspicious" {
		g.Log().Warningf(ctx, "[prompt_guard] suspicious input detected, risk_score=%.2f reason=%q", decision.RiskScore, decision.Reason)
	}

	if d := a.degradationCheck(ctx, "chat"); d.Enabled {
		return &ChatResult{
			Answer:            d.Message,
			Detail:            []string{d.Reason},
			Mode:              "degraded",
			Degraded:          true,
			DegradationReason: d.Reason,
		}, nil
	}

	mu := a.acquireSessionLock(id)
	defer a.releaseSessionLock(id, mu)

	sessionMem := memory.GetSimpleMemory(id)
	bypassResponseCache := shouldBypassCache(msg)

	if !bypassResponseCache {
		if entry, found, cacheErr := cache.LoadChatResponse(ctx, id, msg, selectedSkillIDs...); cacheErr != nil {
			g.Log().Warningf(ctx, "[session:%s][req:%s] cache lookup failed: %v", id, requestID, cacheErr)
		} else if found {
			answer, detail := filterOutput(ctx, entry.Answer, entry.Detail)
			return &ChatResult{
				Answer: answer,
				Detail: detail,
				Mode:   "cache",
				Cached: true,
			}, nil
		}
	} else {
		g.Log().Debugf(ctx, "[session:%s][req:%s] bypass chat cache for lightweight social input", id, requestID)
	}

	memorySvc := aiservice.NewMemoryService()
	contextPkg, contextDetail := memorySvc.BuildChatPackage(ctx, id, msg, sessionMem.GetContextMessages())

	userMessage := &chat_pipeline.UserMessage{
		ID:        id,
		Query:     msg,
		Documents: contextengine.DocumentsContent(contextPkg),
		History:   contextPkg.HistoryMessages,
	}

	runner, err := a.buildChatAgent(ctx, msg)
	if err != nil {
		g.Log().Errorf(ctx, "[session:%s][req:%s] BuildChatAgent failed: %v", id, requestID, err)
		return nil, err
	}

	out, err := runner.Invoke(ctx, userMessage, compose.WithCallbacks(log_call_back.LogCallback(nil)))
	if err != nil {
		if status, message := classifyError(err); status != 0 {
			return &ChatResult{
				Answer:            message,
				Detail:            []string{message},
				Mode:              "degraded",
				Degraded:          true,
				DegradationReason: message,
				HTTPStatus:        status,
			}, nil
		}
		g.Log().Errorf(ctx, "[session:%s][req:%s] Agent invoke failed: %v", id, requestID, err)
		return nil, err
	}

	answer, detail := filterOutput(ctx, out.Content, contextDetail)
	memorySvc.PersistOutcome(ctx, id, msg, answer)
	if !bypassResponseCache {
		if cacheErr := cache.StoreChatResponse(ctx, id, msg, cache.ChatResponseEntry{
			Answer: answer,
			Detail: detail,
			Mode:   "chat",
		}, selectedSkillIDs...); cacheErr != nil {
			g.Log().Warningf(ctx, "[session:%s][req:%s] cache store failed: %v", id, requestID, cacheErr)
		}
	}

	g.Log().Infof(ctx, "[session:%s][req:%s] Chat completed, answer length: %d, turns: %d",
		id, requestID, len(answer), sessionMem.TurnCount())

	return &ChatResult{
		Answer: answer,
		Detail: detail,
		Mode:   "chat",
	}, nil
}

func (a *ChatApp) acquireSessionLock(id string) *sessionLockEntry {
	a.sessionLocksMu.Lock()
	entry, ok := a.sessionLocks[id]
	if !ok {
		entry = &sessionLockEntry{}
		a.sessionLocks[id] = entry
	}
	entry.refCount++
	entry.lastUsed = time.Now()
	a.sessionLocksMu.Unlock()
	entry.mu.Lock()
	return entry
}

func (a *ChatApp) releaseSessionLock(id string, entry *sessionLockEntry) {
	if entry == nil {
		return
	}
	entry.mu.Unlock()
	a.sessionLocksMu.Lock()
	defer a.sessionLocksMu.Unlock()
	current, ok := a.sessionLocks[id]
	if !ok || current != entry {
		return
	}
	if entry.refCount > 0 {
		entry.refCount--
	}
	entry.lastUsed = time.Now()
	if entry.refCount == 0 {
		delete(a.sessionLocks, id)
	}
}

// cleanupStaleSessionLocks periodically removes session lock entries
// that have not been used within the TTL window.
func (a *ChatApp) cleanupStaleSessionLocks() {
	ticker := time.NewTicker(sessionLockCleanup)
	defer ticker.Stop()
	for range ticker.C {
		a.sessionLocksMu.Lock()
		now := time.Now()
		for id, entry := range a.sessionLocks {
			if entry.refCount == 0 && now.Sub(entry.lastUsed) > sessionLockTTL {
				delete(a.sessionLocks, id)
			}
		}
		a.sessionLocksMu.Unlock()
	}
}

func chatTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "chat.timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return 0
}
