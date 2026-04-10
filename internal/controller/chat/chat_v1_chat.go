package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/contextengine"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/cache"
	"SuperBizAgent/utility/log_call_back"
	"SuperBizAgent/utility/mem"
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

var sessionLocks sync.Map
var (
	buildChatAgent         = chat_pipeline.BuildChatAgentWithQuery
	getDegradationDecision = aiservice.GetDegradationDecision
)

func acquireSessionLock(id string) *sync.Mutex {
	val, _ := sessionLocks.LoadOrStore(id, &sync.Mutex{})
	mu := val.(*sync.Mutex)
	mu.Lock()
	return mu
}

func releaseSessionLock(id string, mu *sync.Mutex) {
	mu.Unlock()
}

func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	id := req.Id
	msg := req.Question

	if err := mem.ValidateSessionID(id); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, id)
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = enrichRequestContext(ctx, id, requestID)

	g.Log().Infof(ctx, "[session:%s][req:%s] Chat request received, question length: %d", id, requestID, len(msg))

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

	if entry, found, cacheErr := cache.LoadChatResponse(ctx, id, msg); cacheErr != nil {
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
	if cacheErr := cache.StoreChatResponse(ctx, id, msg, cache.ChatResponseEntry{
		Answer: answer,
		Detail: detail,
		Mode:   "chat",
	}); cacheErr != nil {
		g.Log().Warningf(ctx, "[session:%s][req:%s] cache store failed: %v", id, requestID, cacheErr)
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
