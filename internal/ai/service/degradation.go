package service

import (
	"context"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultDegradationRedisKey = "oncallai:degradation:kill_switch"
	defaultDegradationMessage  = "AI capabilities are temporarily degraded. Please retry later."
)

type ExecutionResponse struct {
	Content           string
	Detail            []string
	TraceID           string
	Status            protocol.ResultStatus
	DegradationReason string
	ApprovalRequired  bool
	ApprovalRequestID string
	ApprovalStatus    string
	ExecutionPlan     []string
}

func (r ExecutionResponse) Degraded() bool {
	return r.Status == protocol.ResultStatusDegraded
}

type DegradationDecision struct {
	Enabled bool
	Message string
	Reason  string
	Source  string
}

var (
	degradationConfigBool = func(ctx context.Context, key string) bool {
		v, err := g.Cfg().Get(ctx, key)
		return err == nil && v.Bool()
	}
	degradationConfigString = func(ctx context.Context, key string) string {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(v.String())
	}
	degradationRedisGet = func(ctx context.Context, key string) (string, error) {
		value, err := g.Redis().Do(ctx, "GET", key)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(value.String()), nil
	}
)

func GetDegradationDecision(ctx context.Context, entrypoint string) DegradationDecision {
	message := degradationMessage(ctx)
	if degradationConfigBool(ctx, "degradation.kill_switch") {
		return DegradationDecision{
			Enabled: true,
			Message: message,
			Reason:  fmt.Sprintf("global kill switch enabled for %s via config", strings.TrimSpace(entrypoint)),
			Source:  "config",
		}
	}

	if degradationConfigString(ctx, "redis.default.address") == "" {
		return DegradationDecision{Message: message}
	}

	key := degradationConfigString(ctx, "degradation.redis_key")
	if key == "" {
		key = defaultDegradationRedisKey
	}
	raw, err := degradationRedisGet(ctx, key)
	if err != nil {
		g.Log().Warningf(ctx, "degradation kill switch redis lookup failed: %v", err)
		return DegradationDecision{Message: message}
	}
	if !isTruthyFlag(raw) {
		return DegradationDecision{Message: message}
	}

	return DegradationDecision{
		Enabled: true,
		Message: message,
		Reason:  fmt.Sprintf("global kill switch enabled for %s via redis key %s", strings.TrimSpace(entrypoint), key),
		Source:  "redis",
	}
}

func NewDegradedExecutionResponse(decision DegradationDecision) ExecutionResponse {
	detail := make([]string, 0, 1)
	if strings.TrimSpace(decision.Reason) != "" {
		detail = append(detail, decision.Reason)
	}
	return ExecutionResponse{
		Content:           decision.Message,
		Detail:            detail,
		Status:            protocol.ResultStatusDegraded,
		DegradationReason: decision.Reason,
	}
}

func ExecutionResponseFromResult(result *protocol.TaskResult, detail []string, traceID string) ExecutionResponse {
	response := ExecutionResponse{
		Detail:  detail,
		TraceID: traceID,
		Status:  protocol.ResultStatusSucceeded,
	}
	if result == nil {
		return response
	}
	response.Content = result.Summary
	if result.Status != "" {
		response.Status = result.Status
	}
	response.DegradationReason = strings.TrimSpace(result.DegradationReason)
	return response
}

func degradationMessage(ctx context.Context) string {
	message := degradationConfigString(ctx, "degradation.message")
	if message == "" {
		return defaultDegradationMessage
	}
	return message
}

func isTruthyFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on", "yes", "enabled":
		return true
	default:
		return false
	}
}
