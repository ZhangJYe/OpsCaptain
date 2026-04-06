package chat

import (
	"SuperBizAgent/utility/safety"
	"context"
	"errors"
	"net/http"

	"github.com/gogf/gf/v2/frame/g"
)

func rejectSuspiciousPrompt(ctx context.Context, input string) error {
	decision := safety.CheckPrompt(ctx, input)
	if decision.Allowed {
		return nil
	}

	g.Log().Warningf(ctx, "[prompt_guard] blocked request, pattern=%s", decision.Pattern)
	if req := g.RequestFromCtx(ctx); req != nil {
		req.Response.WriteStatus(http.StatusBadRequest)
	}
	return errors.New(decision.Reason)
}
