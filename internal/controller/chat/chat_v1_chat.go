package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/app"
	"context"
	"errors"
	"net/http"

	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	result, err := c.chatApp.HandleChat(ctx, &app.ChatInput{
		SessionID: req.Id,
		Question:  req.Question,
		SkillIDs:  req.SelectedSkillIds,
	})
	if err != nil {
		var rejected *app.PromptRejectedError
		if errors.As(err, &rejected) {
			if r := g.RequestFromCtx(ctx); r != nil {
				r.Response.WriteStatus(http.StatusBadRequest)
			}
			return nil, err
		}
		return nil, err
	}
	if result.HTTPStatus != 0 {
		if r := g.RequestFromCtx(ctx); r != nil {
			r.Response.WriteStatus(result.HTTPStatus)
		}
	}
	return &v1.ChatRes{
		Answer:            result.Answer,
		Detail:            result.Detail,
		TraceID:           result.TraceID,
		Mode:              result.Mode,
		Degraded:          result.Degraded,
		DegradationReason: result.DegradationReason,
		Cached:            result.Cached,
	}, nil
}
