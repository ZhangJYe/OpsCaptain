package rabbitmq

import (
	"SuperBizAgent/utility/common"
	"context"
	"strings"
	"time"
)

// ResolveRabbitMQString resolves a config value that may be an env reference.
// If raw is an env var reference, it resolves it. If raw is empty or an
// unresolvable env reference, it returns fallback.
func ResolveRabbitMQString(raw, fallback string) string {
	if resolved, ok := common.ResolveOptionalEnv(raw); ok {
		return strings.TrimSpace(resolved)
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || common.IsEnvReference(trimmed) {
		return fallback
	}
	return trimmed
}

// SleepReconnect sleeps for the given delay, returning false if ctx is cancelled.
func SleepReconnect(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		delay = 5 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
