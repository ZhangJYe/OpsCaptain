package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/contextengine"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/cache"
	"SuperBizAgent/utility/log_call_back"
	"SuperBizAgent/utility/mem"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

type sessionLockEntry struct {
	mu       sync.Mutex
	refCount int
	lastUsed time.Time
}

var (
	sessionLocks   = make(map[string]*sessionLockEntry)
	sessionLocksMu sync.Mutex
)
var (
	buildChatAgent          = chat_pipeline.BuildChatAgentWithQuery
	getDegradationDecision  = aiservice.GetDegradationDecision
	shouldUseChatMultiAgent = aiservice.ShouldUseMultiAgentForChat
	runChatMultiAgent       = aiservice.RunChatMultiAgent
)

func acquireSessionLock(id string) *sessionLockEntry {
	sessionLocksMu.Lock()
	entry, ok := sessionLocks[id]
	if !ok {
		entry = &sessionLockEntry{}
		sessionLocks[id] = entry
	}
	entry.refCount++
	entry.lastUsed = time.Now()
	sessionLocksMu.Unlock()
	entry.mu.Lock()
	return entry
}

func releaseSessionLock(id string, entry *sessionLockEntry) {
	if entry == nil {
		return
	}
	entry.mu.Unlock()
	sessionLocksMu.Lock()
	defer sessionLocksMu.Unlock()
	current, ok := sessionLocks[id]
	if !ok || current != entry {
		return
	}
	if entry.refCount > 0 {
		entry.refCount--
	}
	entry.lastUsed = time.Now()
	if entry.refCount == 0 {
		delete(sessionLocks, id)
	}
}

func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	id := req.Id
	msg := req.Question
	selectedSkillIDs := chat_pipeline.NormalizeSelectedSkillIDs(req.SelectedSkillIds)

	if err := mem.ValidateSessionID(id); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, id)
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = skills.WithSelectedSkillIDs(ctx, selectedSkillIDs)
	ctx = enrichRequestContext(ctx, id, requestID)
	selectedSkillIDs = skills.SelectedSkillIDsFromContext(ctx)

	g.Log().Infof(ctx, "[session:%s][req:%s] Chat request received, question length: %d, selected_skills=%v", id, requestID, len(msg), selectedSkillIDs)

	if err := rejectSuspiciousPrompt(ctx, msg); err != nil {
		return nil, err
	}
	if decision := getDegradationDecision(ctx, "chat"); decision.Enabled {
		return &v1.ChatRes{
			Answer:            decision.Message,
			Detail:            []string{decision.Reason},
			Mode:              "degraded",
			Degraded:          true,
			DegradationReason: decision.Reason,
		}, nil
	}

	mu := acquireSessionLock(id)
	defer releaseSessionLock(id, mu)

	sessionMem := mem.GetSimpleMemory(id)
	bypassResponseCache := shouldBypassChatResponseCache(msg)

	if !bypassResponseCache {
		if entry, found, cacheErr := cache.LoadChatResponse(ctx, id, msg, selectedSkillIDs...); cacheErr != nil {
			g.Log().Warningf(ctx, "[session:%s][req:%s] cache lookup failed: %v", id, requestID, cacheErr)
		} else if found {
			answer, detail := filterAssistantPayload(ctx, entry.Answer, entry.Detail)
			return &v1.ChatRes{
				Answer: answer,
				Detail: detail,
				Mode:   "cache",
				Cached: true,
			}, nil
		}
	} else {
		g.Log().Debugf(ctx, "[session:%s][req:%s] bypass chat cache for lightweight social input", id, requestID)
	}

	if shouldUseChatMultiAgent(ctx, msg) {
		response, err := runChatMultiAgent(ctx, id, msg)
		if err != nil {
			if fallback := userFacingChatError(ctx, err); fallback != nil {
				return fallback, nil
			}
			return nil, err
		}
		answer, detail := filterAssistantPayload(ctx, response.Content, response.Detail)
		if !bypassResponseCache {
			if cacheErr := cache.StoreChatResponse(ctx, id, msg, cache.ChatResponseEntry{
				Answer: answer,
				Detail: detail,
				Mode:   "multi_agent",
			}, selectedSkillIDs...); cacheErr != nil {
				g.Log().Warningf(ctx, "[session:%s][req:%s] cache store failed: %v", id, requestID, cacheErr)
			}
		}
		return &v1.ChatRes{
			Answer:            answer,
			TraceID:           response.TraceID,
			Detail:            detail,
			Mode:              "multi_agent",
			Degraded:          response.Degraded(),
			DegradationReason: response.DegradationReason,
		}, nil
	}

	memorySvc := aiservice.NewMemoryService()
	contextPkg, contextDetail := memorySvc.BuildChatPackage(ctx, id, msg, sessionMem.GetContextMessages())

	userMessage := &chat_pipeline.UserMessage{
		ID:        id,
		Query:     msg,
		Documents: contextengine.DocumentsContent(contextPkg),
		History:   contextPkg.HistoryMessages,
	}

	runner, err := buildChatAgent(ctx, msg)
	if err != nil {
		g.Log().Errorf(ctx, "[session:%s][req:%s] BuildChatAgent failed: %v", id, requestID, err)
		return nil, err
	}

	out, err := runner.Invoke(ctx, userMessage, compose.WithCallbacks(log_call_back.LogCallback(nil)))
	if err != nil {
		if fallback := userFacingChatError(ctx, err); fallback != nil {
			return fallback, nil
		}
		g.Log().Errorf(ctx, "[session:%s][req:%s] Agent invoke failed: %v", id, requestID, err)
		return nil, err
	}

	answer, detail := filterAssistantPayload(ctx, out.Content, contextDetail)
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

	res = &v1.ChatRes{
		Answer: answer,
		Detail: detail,
		Mode:   "chat",
	}
	return res, nil
}

func shouldBypassChatResponseCache(query string) bool {
	normalized := strings.TrimSpace(strings.ToLower(query))
	if normalized == "" {
		return false
	}
	if strings.ContainsAny(normalized, "\n\t") {
		return false
	}
	switch normalized {
	case "hi", "hello", "hey", "你好", "您好", "嗨", "哈喽", "在吗", "在么", "早", "早上好", "晚上好", "午安":
		return true
	default:
		return false
	}
}
