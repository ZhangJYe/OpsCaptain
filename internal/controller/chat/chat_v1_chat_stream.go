package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/contextengine"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/internal/logic/sse"
	"SuperBizAgent/utility/log_call_back"
	"SuperBizAgent/utility/mem"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

func (c *ControllerV1) ChatStream(ctx context.Context, req *v1.ChatStreamReq) (res *v1.ChatStreamRes, err error) {
	id := req.Id
	msg := req.Question

	if err := mem.ValidateSessionID(id); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, id)
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = context.WithValue(ctx, consts.CtxKeyClientID, req.Id)
	ctx = enrichRequestContext(ctx, id, requestID)

	g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream request received, question length: %d", id, requestID, len(msg))

	if err := rejectSuspiciousPrompt(ctx, msg); err != nil {
		return nil, err
	}

	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}
	if decision := getDegradationDecision(ctx, "chat_stream"); decision.Enabled {
		_, filteredReason := filterAssistantPayload(ctx, "", []string{decision.Reason})
		sendChatStreamMeta(client, "degraded", "", filteredReason, true, decision.Reason)
		streamDetailsToClient(client, filteredReason)
		streamTextToClient(client, decision.Message)
		client.SendToClient("done", "Stream completed")
		return &v1.ChatStreamRes{}, nil
	}

	mu := acquireSessionLock(id)
	defer releaseSessionLock(id, mu)

	if shouldUseChatMultiAgent(ctx, msg) {
		response, execErr := runChatMultiAgent(ctx, id, msg)
		if execErr != nil {
			if fallback := userFacingChatError(ctx, execErr); fallback != nil {
				_, filteredDetail := filterAssistantPayload(ctx, "", fallback.Detail)
				sendChatStreamMeta(client, fallback.Mode, fallback.TraceID, filteredDetail, fallback.Degraded, fallback.DegradationReason)
				streamDetailsToClient(client, filteredDetail)
				streamTextToClient(client, fallback.Answer)
				client.SendToClient("done", "Stream completed")
				return &v1.ChatStreamRes{}, nil
			}
			client.SendToClient("error", execErr.Error())
			client.SendToClient("done", "Stream completed")
			return &v1.ChatStreamRes{}, nil
		}
		_, filteredDetail := filterAssistantPayload(ctx, "", response.Detail)
		sendChatStreamMeta(client, "multi_agent", response.TraceID, filteredDetail, response.Degraded(), response.DegradationReason)
		streamDetailsToClient(client, filteredDetail)
		filteredContent, _ := filterAssistantPayload(ctx, response.Content, nil)
		streamTextToClient(client, filteredContent)
		client.SendToClient("done", "Stream completed")
		return &v1.ChatStreamRes{}, nil
	}

	sessionMem := mem.GetSimpleMemory(id)

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
		if fallback := userFacingChatError(ctx, err); fallback != nil {
			_, filteredDetail := filterAssistantPayload(ctx, "", fallback.Detail)
			sendChatStreamMeta(client, fallback.Mode, "", filteredDetail, fallback.Degraded, fallback.DegradationReason)
			streamDetailsToClient(client, filteredDetail)
			streamTextToClient(client, fallback.Answer)
			client.SendToClient("done", "Stream completed")
			return &v1.ChatStreamRes{}, nil
		}
		g.Log().Errorf(ctx, "[session:%s][req:%s] BuildChatAgent failed: %v", id, requestID, err)
		client.SendToClient("error", err.Error())
		client.SendToClient("done", "Stream completed")
		return &v1.ChatStreamRes{}, nil
	}
	_, filteredDetail := filterAssistantPayload(ctx, "", contextDetail)
	sendChatStreamMeta(client, "chat", "", filteredDetail, false, "")
	streamDetailsToClient(client, filteredDetail)
	sr, err := runner.Stream(ctx, userMessage, compose.WithCallbacks(log_call_back.LogCallback(nil)))
	if err != nil {
		if fallback := userFacingChatError(ctx, err); fallback != nil {
			_, detailFiltered := filterAssistantPayload(ctx, "", fallback.Detail)
			sendChatStreamMeta(client, fallback.Mode, "", detailFiltered, fallback.Degraded, fallback.DegradationReason)
			streamDetailsToClient(client, detailFiltered)
			streamTextToClient(client, fallback.Answer)
			client.SendToClient("done", "Stream completed")
			return &v1.ChatStreamRes{}, nil
		}
		g.Log().Errorf(ctx, "[session:%s][req:%s] Agent stream failed: %v", id, requestID, err)
		client.SendToClient("error", err.Error())
		client.SendToClient("done", "Stream completed")
		return &v1.ChatStreamRes{}, nil
	}
	defer sr.Close()

	var fullResponse strings.Builder

	defer func() {
		completeResponse := fullResponse.String()
		if completeResponse != "" {
			memorySvc.PersistOutcome(ctx, id, msg, completeResponse)
			g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream completed, answer length: %d, turns: %d",
				id, requestID, len(completeResponse), sessionMem.TurnCount())
		}
	}()

	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			client.SendToClient("done", "Stream completed")
			return &v1.ChatStreamRes{}, nil
		}
		if err != nil {
			g.Log().Errorf(ctx, "[session:%s][req:%s] Stream recv error: %v", id, requestID, err)
			client.SendToClient("error", err.Error())
			client.SendToClient("done", "Stream completed")
			return &v1.ChatStreamRes{}, nil
		}
		filteredChunk, _ := filterAssistantPayload(ctx, chunk.Content, nil)
		fullResponse.WriteString(filteredChunk)
		client.SendToClient("message", filteredChunk)
	}
}

func streamTextToClient(client *sse.Client, text string) {
	for _, chunk := range splitStreamChunks(text, 160) {
		client.SendToClient("message", chunk)
	}
}

func streamDetailsToClient(client *sse.Client, details []string) {
	for _, detail := range details {
		trimmed := strings.TrimSpace(detail)
		if trimmed == "" {
			continue
		}
		client.SendToClient("thought", trimmed)
	}
}

func splitStreamChunks(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 160
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	chunks := make([]string, 0, len(runes)/maxRunes+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func sendChatStreamMeta(client *sse.Client, mode, traceID string, details []string, degraded bool, degradationReason string) {
	if client == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"mode":               mode,
		"trace_id":           traceID,
		"detail":             details,
		"degraded":           degraded,
		"degradation_reason": degradationReason,
	})
	if err != nil {
		return
	}
	client.SendToClient("meta", string(payload))
}
