package service

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/metrics"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const tokenAuditTTL = 48 * time.Hour

type SessionTokenAudit struct {
	Date             string `json:"date"`
	SessionID        string `json:"session_id"`
	UserID           string `json:"user_id,omitempty"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Calls            int    `json:"calls"`
	LastModel        string `json:"last_model,omitempty"`
	UpdatedAt        int64  `json:"updated_at,omitempty"`
}

type DailyTokenLimitError struct {
	SessionID string
	Limit     int
	Current   int
}

func (e *DailyTokenLimitError) Error() string {
	return fmt.Sprintf("daily token limit exceeded for session %s: %d/%d", e.SessionID, e.Current, e.Limit)
}

func IsDailyTokenLimitError(err error) bool {
	_, ok := err.(*DailyTokenLimitError)
	return ok
}

func RecordSessionTokenUsage(ctx context.Context, sessionID, model string, promptTokens, completionTokens int) error {
	if strings.TrimSpace(sessionID) == "" || !tokenAuditEnabled(ctx) {
		return nil
	}

	key := tokenAuditKey(time.Now(), sessionID)
	total := promptTokens + completionTokens
	if promptTokens > 0 {
		if _, err := g.Redis().Do(ctx, "HINCRBY", key, "prompt_tokens", promptTokens); err != nil {
			return err
		}
	}
	if completionTokens > 0 {
		if _, err := g.Redis().Do(ctx, "HINCRBY", key, "completion_tokens", completionTokens); err != nil {
			return err
		}
	}
	if total > 0 {
		if _, err := g.Redis().Do(ctx, "HINCRBY", key, "total_tokens", total); err != nil {
			return err
		}
		if _, err := g.Redis().Do(ctx, "HINCRBY", key, "calls", 1); err != nil {
			return err
		}
	}

	fields := []any{
		"session_id", sessionID,
		"date", time.Now().Format("2006-01-02"),
		"updated_at", time.Now().Unix(),
	}
	if model != "" {
		fields = append(fields, "last_model", model)
	}
	if userID, ok := ctx.Value(consts.CtxKeyUserID).(string); ok && strings.TrimSpace(userID) != "" {
		fields = append(fields, "user_id", strings.TrimSpace(userID))
	}
	if _, err := g.Redis().Do(ctx, "HSET", append([]any{key}, fields...)...); err != nil {
		return err
	}
	userID, _ := ctx.Value(consts.CtxKeyUserID).(string)
	metrics.AddSessionTokens(sessionID, userID, total)
	_, err := g.Redis().Do(ctx, "EXPIRE", key, int(tokenAuditTTL.Seconds()))
	return err
}

func GetSessionTokenAudit(ctx context.Context, sessionID string, date time.Time) (SessionTokenAudit, error) {
	if strings.TrimSpace(sessionID) == "" {
		return SessionTokenAudit{}, fmt.Errorf("session_id is required")
	}
	if date.IsZero() {
		date = time.Now()
	}
	key := tokenAuditKey(date, sessionID)
	value, err := g.Redis().Do(ctx, "HGETALL", key)
	if err != nil {
		return SessionTokenAudit{}, err
	}
	fields := value.MapStrStr()
	if len(fields) == 0 {
		return SessionTokenAudit{
			Date:      date.Format("2006-01-02"),
			SessionID: sessionID,
		}, nil
	}
	return SessionTokenAudit{
		Date:             firstNonEmpty(fields["date"], date.Format("2006-01-02")),
		SessionID:        firstNonEmpty(fields["session_id"], sessionID),
		UserID:           fields["user_id"],
		PromptTokens:     atoi(fields["prompt_tokens"]),
		CompletionTokens: atoi(fields["completion_tokens"]),
		TotalTokens:      atoi(fields["total_tokens"]),
		Calls:            atoi(fields["calls"]),
		LastModel:        fields["last_model"],
		UpdatedAt:        int64(atoi(fields["updated_at"])),
	}, nil
}

func EnforceDailyTokenLimit(ctx context.Context, sessionID string) error {
	limit := dailyTokenLimit(ctx)
	if limit <= 0 || strings.TrimSpace(sessionID) == "" || !tokenAuditEnabled(ctx) {
		return nil
	}

	audit, err := GetSessionTokenAudit(ctx, sessionID, time.Now())
	if err != nil {
		g.Log().Warningf(ctx, "token audit lookup failed for session %s: %v", sessionID, err)
		return nil
	}
	if audit.TotalTokens < limit {
		return nil
	}
	return &DailyTokenLimitError{
		SessionID: sessionID,
		Limit:     limit,
		Current:   audit.TotalTokens,
	}
}

func EnforceTokenLimitFromContext(ctx context.Context) error {
	sessionID, _ := ctx.Value(consts.CtxKeySessionID).(string)
	return EnforceDailyTokenLimit(ctx, sessionID)
}

func RecordTokenUsageFromContext(ctx context.Context, model string, promptTokens, completionTokens int) {
	sessionID, _ := ctx.Value(consts.CtxKeySessionID).(string)
	if err := RecordSessionTokenUsage(ctx, sessionID, model, promptTokens, completionTokens); err != nil {
		g.Log().Warningf(ctx, "token audit record failed for session %s: %v", sessionID, err)
	}
}

func dailyTokenLimit(ctx context.Context) int {
	v, err := g.Cfg().Get(ctx, "cost.daily_limit_tokens")
	if err != nil || v.Int() <= 0 {
		return 0
	}
	return v.Int()
}

func tokenAuditEnabled(ctx context.Context) bool {
	addr, err := g.Cfg().Get(ctx, "redis.default.address")
	if err != nil {
		return false
	}
	_, ok := common.ResolveOptionalEnv(addr.String())
	return ok
}

func tokenAuditKey(date time.Time, sessionID string) string {
	if date.IsZero() {
		date = time.Now()
	}
	return fmt.Sprintf("token_audit:%s:%s", date.Format("2006-01-02"), strings.TrimSpace(sessionID))
}

func atoi(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
