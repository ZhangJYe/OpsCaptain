package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/utility/safety"
	traceutil "SuperBizAgent/utility/tracing"
	"context"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel/attribute"
)

func enrichRequestContext(ctx context.Context, sessionID, requestID string) context.Context {
	traceutil.SetAttributes(
		ctx,
		attribute.String("session.id", strings.TrimSpace(sessionID)),
		attribute.String("request.id", strings.TrimSpace(requestID)),
	)
	return traceutil.ContextWithTraceID(ctx)
}

func filterAssistantPayload(ctx context.Context, content string, details []string) (string, []string) {
	filtered := safety.FilterOutput(ctx, content)
	if filtered.Redacted {
		g.Log().Warningf(ctx, "[output_filter] redacted response, reasons=%s", strings.Join(filtered.Reasons, ","))
	}
	return filtered.Content, safety.FilterDetails(ctx, details)
}

func userFacingChatError(ctx context.Context, err error) *v1.ChatRes {
	status, message := userFacingExecutionError(err)
	if status == 0 {
		return nil
	}
	writeStatus(ctx, status)
	return &v1.ChatRes{
		Answer:            message,
		Detail:            []string{message},
		Mode:              "degraded",
		Degraded:          true,
		DegradationReason: message,
	}
}

func userFacingAIOpsError(ctx context.Context, err error) *v1.AIOpsRes {
	status, message := userFacingExecutionError(err)
	if status == 0 {
		return nil
	}
	writeStatus(ctx, status)
	return &v1.AIOpsRes{
		Result:            message,
		Detail:            []string{message},
		Degraded:          true,
		DegradationReason: message,
	}
}

func userFacingExecutionError(err error) (int, string) {
	switch {
	case aiservice.IsDailyTokenLimitError(err):
		return http.StatusTooManyRequests, "daily token limit exceeded for this session"
	case strings.Contains(strings.ToLower(err.Error()), "llm concurrency queue timeout"):
		return http.StatusServiceUnavailable, "AI is temporarily busy. Please retry shortly."
	default:
		return 0, ""
	}
}

func writeStatus(ctx context.Context, status int) {
	if req := g.RequestFromCtx(ctx); req != nil {
		req.Response.WriteStatus(status)
	}
}
