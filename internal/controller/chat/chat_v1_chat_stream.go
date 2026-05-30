package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/app"
	"SuperBizAgent/internal/logic/sse"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

type sseStreamSink struct {
	client *sse.Client
}

func (s *sseStreamSink) SendMeta(event app.ChatStreamMetaEvent) {
	if s.client == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	s.client.SendToClient("meta", string(payload))
}

func (s *sseStreamSink) SendText(text string) {
	if s.client == nil {
		return
	}
	for _, chunk := range splitStreamChunks(text, 160) {
		s.client.SendToClient("message", chunk)
	}
}

func (s *sseStreamSink) SendDetails(details []string) {
	if s.client == nil {
		return
	}
	for _, detail := range details {
		trimmed := strings.TrimSpace(detail)
		if trimmed == "" {
			continue
		}
		s.client.SendToClient("thought", trimmed)
	}
}

func (s *sseStreamSink) SendEvent(eventType, data string) {
	if s.client == nil {
		return
	}
	s.client.SendToClient(eventType, data)
}

func (c *ControllerV1) ChatStream(ctx context.Context, req *v1.ChatStreamReq) (res *v1.ChatStreamRes, err error) {
	if err := c.chatApp.ValidateChatInput(ctx, req.Id, req.Question); err != nil {
		var rejected *app.PromptRejectedError
		if errors.As(err, &rejected) {
			if r := g.RequestFromCtx(ctx); r != nil {
				r.Response.WriteStatus(http.StatusBadRequest)
			}
		}
		return nil, err
	}

	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}
	sink := &sseStreamSink{client: client}

	result, err := c.chatApp.HandleChatStream(ctx, &app.ChatStreamInput{
		SessionID: req.Id,
		Question:  req.Question,
		SkillIDs:  req.SelectedSkillIds,
	}, sink)
	if err != nil {
		return nil, err
	}
	_ = result
	return &v1.ChatStreamRes{}, nil
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
