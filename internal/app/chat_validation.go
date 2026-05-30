package app

import (
	"SuperBizAgent/internal/ai/memory"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/utility/resilience"
	"SuperBizAgent/utility/safety"
	traceutil "SuperBizAgent/utility/tracing"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel/attribute"
)

func enrichContext(ctx context.Context, sessionID, requestID string) context.Context {
	traceutil.SetAttributes(
		ctx,
		attribute.String("session.id", strings.TrimSpace(sessionID)),
		attribute.String("request.id", strings.TrimSpace(requestID)),
	)
	return traceutil.ContextWithTraceID(ctx)
}

func filterOutput(ctx context.Context, content string, details []string) (string, []string) {
	filtered := safety.FilterOutput(ctx, content)
	if filtered.Redacted {
		g.Log().Warningf(ctx, "[output_filter] redacted response, reasons=%s", strings.Join(filtered.Reasons, ","))
	}
	return filtered.Content, safety.FilterDetails(ctx, details)
}

func shouldBypassCache(query string) bool {
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

func classifyError(err error) (int, string) {
	if aiservice.IsDailyTokenLimitError(err) {
		return 429, "daily token limit exceeded for this session"
	}
	if resilience.IsConcurrencyLimitError(err) {
		return 503, "AI is temporarily busy. Please retry shortly."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 504, "AI response timeout. The request may still be processing — please retry."
	}
	if errors.Is(err, context.Canceled) {
		return 0, ""
	}
	if errors.Is(err, resilience.ErrCircuitBreakerOpen) {
		return 503, "AI service temporarily unavailable. Please retry later."
	}
	return 0, ""
}

// ValidateChatInput checks session ID validity and prompt safety.
func (a *ChatApp) ValidateChatInput(ctx context.Context, sessionID, question string) error {
	if err := memory.ValidateSessionID(sessionID); err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	decision := safety.CheckPrompt(ctx, question)
	if !decision.Allowed {
		return &PromptRejectedError{
			Reason:    decision.Reason,
			RiskScore: decision.RiskScore,
			RiskLevel: decision.RiskLevel,
			Pattern:   decision.Pattern,
		}
	}
	return nil
}
