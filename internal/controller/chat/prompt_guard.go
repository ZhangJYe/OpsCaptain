package chat

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/safety"
	"context"
	"errors"
	"net/http"

	"github.com/gogf/gf/v2/frame/g"
)

// checkAndGuardPrompt runs prompt guard and returns the decision.
// If the prompt is blocked by regex (dangerous), it rejects the request.
// Otherwise, it propagates the risk score into context for downstream use.
func checkAndGuardPrompt(ctx context.Context, input string) (context.Context, safety.PromptGuardDecision, error) {
	decision := safety.CheckPrompt(ctx, input)

	// Always propagate risk score to context
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskScore, decision.RiskScore)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskLevel, decision.RiskLevel)

	if !decision.Allowed {
		g.Log().Warningf(ctx, "[prompt_guard] blocked request, pattern=%s risk_score=%.2f", decision.Pattern, decision.RiskScore)
		if req := g.RequestFromCtx(ctx); req != nil {
			req.Response.WriteStatus(http.StatusBadRequest)
		}
		return ctx, decision, errors.New(decision.Reason)
	}

	if decision.RiskLevel == "suspicious" {
		g.Log().Warningf(ctx, "[prompt_guard] suspicious input detected, risk_score=%.2f reason=%q", decision.RiskScore, decision.Reason)
	}

	return ctx, decision, nil
}
