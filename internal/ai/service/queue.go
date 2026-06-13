package service

import (
	"context"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Deduper abstracts idempotent claim operations for message consumption.
type Deduper interface {
	Claim(ctx context.Context, key string) bool
	Release(ctx context.Context, key string)
}

// QueueClient abstracts the RabbitMQ client operations needed by the service layer.
type QueueClient interface {
	Connect() error
	Publish(ctx context.Context, body []byte, routingKey string, headers amqp.Table) error
	StartConsumer(ctx context.Context, handler func(amqp.Delivery), reconnectDelay time.Duration) error
	Close() error
}

// QueueClientConfig describes the AMQP topology for a single queue with retry and DLQ.
type QueueClientConfig struct {
	URL               string
	Exchange          string
	Queue             string
	RoutingKey        string
	RetryQueue        string
	RetryRoutingKey   string
	DLQ               string
	DLQRoutingKey     string
	RetryDelay        time.Duration
	Prefetch          int
	ConsumerEnabled   bool
	ConnectionTimeout time.Duration
}

var (
	// NewQueueClientFunc creates a new RabbitMQ client. Injected at startup.
	NewQueueClientFunc func(cfg QueueClientConfig, logPrefix string, onReconnectFailed func(error)) (QueueClient, error)

	// ResolveQueueStringFunc resolves a config value that may be an env reference.
	ResolveQueueStringFunc = defaultResolveQueueString

	// SleepReconnectFunc sleeps for the given delay, returning false if ctx is cancelled.
	SleepReconnectFunc = defaultSleepReconnect

	// NewTTLSetFunc creates an in-memory TTL-based deduper.
	NewTTLSetFunc func(ttl time.Duration, maxEntries int) Deduper

	// NewRedisDeduperFunc creates a Redis-based cross-instance deduper.
	NewRedisDeduperFunc func(redis interface{}, prefix string, ttl time.Duration) Deduper

	// NewCompositeDeduperFunc creates a composite deduper that requires all parts to claim.
	NewCompositeDeduperFunc func(parts ...Deduper) Deduper
)

func defaultResolveQueueString(raw, fallback string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func defaultSleepReconnect(ctx context.Context, delay time.Duration) bool {
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
