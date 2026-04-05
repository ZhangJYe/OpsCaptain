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

	g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream request received, question length: %d", id, requestID, len(msg))

	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}

	mu := acquireSessionLock(id)
	defer releaseSessionLock(id, mu)

	sessionMem := mem.GetSimpleMemory(id)
	if shouldUseMultiAgentForChat(ctx, msg) {
		answer, detail, traceID, err := runChatMultiAgent(ctx, id, msg)
		if err != nil {
			g.Log().Errorf(ctx, "[session:%s][req:%s] ChatStream multi-agent failed: %v", id, requestID, err)
			client.SendToClient("error", err.Error())
			return nil, err
		}
		sendChatStreamMeta(client, "multi_agent", traceID, detail)
		streamTextToClient(client, answer)
		client.SendToClient("done", "Stream completed")
		g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream multi-agent completed, answer length: %d, turns: %d, trace: %s",
			id, requestID, len(answer), sessionMem.TurnCount(), traceID)
		return &v1.ChatStreamRes{}, nil
	}

	memorySvc := aiservice.NewMemoryService()
	contextPkg, contextDetail := memorySvc.BuildChatPackage(ctx, id, msg, sessionMem.GetContextMessages())

	userMessage := &chat_pipeline.UserMessage{
		ID:        id,
		Query:     msg,
		Documents: contextengine.DocumentsContent(contextPkg),
		History:   contextPkg.HistoryMessages,
	}

	runner, err := buildChatAgent(ctx)
	if err != nil {
		g.Log().Errorf(ctx, "[session:%s][req:%s] BuildChatAgent failed: %v", id, requestID, err)
		client.SendToClient("error", err.Error())
		return nil, err
	}
	sendChatStreamMeta(client, "legacy", "", contextDetail)
	sr, err := runner.Stream(ctx, userMessage, compose.WithCallbacks(log_call_back.LogCallback(nil)))
	if err != nil {
		g.Log().Errorf(ctx, "[session:%s][req:%s] Agent stream failed: %v", id, requestID, err)
		client.SendToClient("error", err.Error())
		return nil, err
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
			return &v1.ChatStreamRes{}, nil
		}
		fullResponse.WriteString(chunk.Content)
		client.SendToClient("message", chunk.Content)
	}
}

func streamTextToClient(client *sse.Client, text string) {
	for _, chunk := range splitStreamChunks(text, 160) {
		client.SendToClient("message", chunk)
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

func sendChatStreamMeta(client *sse.Client, mode, traceID string, details []string) {
	if client == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"mode":     mode,
		"trace_id": traceID,
		"detail":   details,
	})
	if err != nil {
		return
	}
	client.SendToClient("meta", string(payload))
}
