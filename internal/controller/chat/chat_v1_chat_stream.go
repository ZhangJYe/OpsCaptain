package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/contextengine"
	"SuperBizAgent/internal/ai/events"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/ai/skills"
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
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

func (c *ControllerV1) ChatStream(ctx context.Context, req *v1.ChatStreamReq) (res *v1.ChatStreamRes, err error) {
	id := req.Id
	msg := req.Question
	selectedSkillIDs := chat_pipeline.NormalizeSelectedSkillIDs(req.SelectedSkillIds)

	if err := mem.ValidateSessionID(id); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, id)
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = context.WithValue(ctx, consts.CtxKeyClientID, req.Id)
	ctx = skills.WithSelectedSkillIDs(ctx, selectedSkillIDs)
	ctx = enrichRequestContext(ctx, id, requestID)
	selectedSkillIDs = skills.SelectedSkillIDsFromContext(ctx)

	phaseStart := time.Now()
	g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream start, question length: %d, selected_skills=%v", id, requestID, len(msg), selectedSkillIDs)

	if err := rejectSuspiciousPrompt(ctx, msg); err != nil {
		return nil, err
	}

	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		g.Log().Errorf(ctx, "[session:%s][req:%s] ChatStream init failed: SSE client create error: %v", id, requestID, err)
		return nil, err
	}
	g.Log().Debugf(ctx, "[session:%s][req:%s] ChatStream phase=sse_init duration=%dms", id, requestID, time.Since(phaseStart).Milliseconds())
	if decision := getDegradationDecision(ctx, "chat_stream"); decision.Enabled {
		g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream phase=degraded reason=%s duration=%dms",
			id, requestID, decision.Reason, time.Since(phaseStart).Milliseconds())
		_, filteredReason := filterAssistantPayload(ctx, "", []string{decision.Reason})
		sendChatStreamMeta(client, "degraded", "", filteredReason, true, decision.Reason)
		streamDetailsToClient(client, filteredReason)
		streamTextToClient(client, decision.Message)
		client.SendToClient("done", "Stream completed")
		return &v1.ChatStreamRes{}, nil
	}

	mu := acquireSessionLock(id)
	defer releaseSessionLock(id, mu)

	sessionMem := mem.GetSimpleMemory(id)

	memorySvc := aiservice.NewMemoryService()
	ctxBuildStart := time.Now()
	contextPkg, contextDetail := memorySvc.BuildChatPackage(ctx, id, msg, sessionMem.GetContextMessages())
	g.Log().Debugf(ctx, "[session:%s][req:%s] ChatStream phase=context_built history=%d memory=%d docs=%d tools=%d duration=%dms",
		id, requestID, len(contextPkg.HistoryMessages), len(contextPkg.MemoryItems),
		len(contextPkg.DocumentItems), len(contextPkg.ToolItems), time.Since(ctxBuildStart).Milliseconds())

	userMessage := &chat_pipeline.UserMessage{
		ID:        id,
		Query:     msg,
		Documents: contextengine.DocumentsContent(contextPkg),
		History:   contextPkg.HistoryMessages,
	}

	// 创建事件发射器（在 buildChatAgent 之前，以便工具包装）
	sseEmitter := events.NewSSEEmitter(client, requestID)
	traceEmitter := events.NewTraceEmitter(requestID)
	resultCollector := events.NewResultCollector()
	multiEmitter := events.NewMultiEmitter(sseEmitter, traceEmitter, events.GlobalHealthCollector(), resultCollector)
	ctx = chat_pipeline.WithChatToolEmitter(ctx, multiEmitter, requestID)

	runner, agentBuildErr := buildChatAgent(ctx, msg)
	if agentBuildErr != nil {
		g.Log().Errorf(ctx, "[session:%s][req:%s] ChatStream phase=agent_build_failed error=%v duration=%dms",
			id, requestID, agentBuildErr, time.Since(phaseStart).Milliseconds())
		if fallback := userFacingChatError(ctx, agentBuildErr); fallback != nil {
			g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream phase=agent_build_fallback reason=%s",
				id, requestID, fallback.DegradationReason)
			_, filteredDetail := filterAssistantPayload(ctx, "", fallback.Detail)
			sendChatStreamMeta(client, fallback.Mode, "", filteredDetail, fallback.Degraded, fallback.DegradationReason)
			streamDetailsToClient(client, filteredDetail)
			streamTextToClient(client, fallback.Answer)
			client.SendToClient("done", "Stream completed")
			return &v1.ChatStreamRes{}, nil
		}
		g.Log().Errorf(ctx, "[session:%s][req:%s] BuildChatAgent failed: %v", id, requestID, agentBuildErr)
		client.SendToClient("error", agentBuildErr.Error())
		client.SendToClient("done", "Stream completed")
		return &v1.ChatStreamRes{}, nil
	}
	_, filteredDetail := filterAssistantPayload(ctx, "", contextDetail)
	g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream phase=agent_built duration=%dms",
		id, requestID, time.Since(phaseStart).Milliseconds())
	sendChatStreamMeta(client, "chat", "", filteredDetail, false, "")
	streamDetailsToClient(client, filteredDetail)
	callbackEmitter := events.NewModelCallbackEmitter(multiEmitter, requestID)
	sr, err := runner.Stream(ctx, userMessage, compose.WithCallbacks(
		log_call_back.LogCallback(nil),
		callbackEmitter.Handler(),
	))
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

			// 输出校验：检查关键指标是否有工具数据来源
			if resultCollector.HasToolCalls() {
				toolResults := resultCollector.ToolResults()
				if warnings := events.ValidateOutputAgainstToolResults(completeResponse, toolResults); len(warnings) > 0 {
					g.Log().Warningf(ctx, "[session:%s][req:%s] output validation warnings: %v", id, requestID, warnings)
				}
			} else if isOpsRelatedQuery(msg) {
				// 运维相关问题但没有调用任何工具 → 可能是幻觉
				g.Log().Warningf(ctx, "[session:%s][req:%s] ops-related query but no tool calls detected, possible hallucination risk", id, requestID)
			}
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

			if fullResponse.Len() > 0 {
				g.Log().Infof(ctx, "[session:%s][req:%s] Stream interrupted but partial content (%d chars) already received, ending gracefully",
					id, requestID, fullResponse.Len())
				client.SendToClient("done", "Stream completed")
				return &v1.ChatStreamRes{}, nil
			}

			if fallback := userFacingChatError(ctx, err); fallback != nil {
				g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream phase=stream_fallback reason=%s",
					id, requestID, fallback.DegradationReason)
				_, filteredDetail := filterAssistantPayload(ctx, "", fallback.Detail)
				sendChatStreamMeta(client, fallback.Mode, fallback.TraceID, filteredDetail, fallback.Degraded, fallback.DegradationReason)
				streamDetailsToClient(client, filteredDetail)
				streamTextToClient(client, fallback.Answer)
				client.SendToClient("done", "Stream completed")
				return &v1.ChatStreamRes{}, nil
			}

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

var opsKeywords = []string{
	"告警", "alert", "prometheus", "日志", "log", "排查", "故障", "incident",
	"指标", "metric", "延迟", "latency", "错误率", "error rate", "超时", "timeout",
	"服务异常", "服务挂了", "报警", "cpu", "内存", "memory", "磁盘",
	"网络", "network", "数据库", "mysql", "redis", "连接池", "队列", "queue",
}

func isOpsRelatedQuery(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return false
	}
	for _, kw := range opsKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
