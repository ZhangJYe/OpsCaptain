package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultRabbitMQExchange                  = "opscaption.events"
	defaultMemoryExtractRoutingKey           = "memory.extract.request"
	defaultMemoryExtractRetryRoutingKey      = "memory.extract.retry"
	defaultMemoryExtractDLQRoutingKey        = "memory.extract.dlq"
	defaultMemoryExtractQueue                = "opscaption.memory.extract"
	defaultMemoryExtractPrefetch             = 8
	defaultMemoryExtractMaxRetries           = 3
	defaultMemoryExtractRetryDelay           = 2 * time.Second
	defaultMemoryExtractPublishTimeout       = 2 * time.Second
	defaultMemoryExtractDedupTTL             = 10 * time.Minute
	defaultMemoryExtractDedupMaxEntries      = 20000
	defaultMemoryExtractConsumerEnabled      = true
	defaultMemoryExtractConnectionTimeoutSec = 5
	defaultMemoryExtractReconnectDelay       = 2 * time.Second
)

type rabbitMQMemoryConfig struct {
	Enabled                      bool
	URL                          string
	Exchange                     string
	MemoryExtractRoutingKey      string
	MemoryExtractRetryRoutingKey string
	MemoryExtractDLQRoutingKey   string
	MemoryExtractQueue           string
	MemoryExtractRetryQueue      string
	MemoryExtractDLQ             string
	MemoryExtractPrefetch        int
	MemoryExtractMaxRetries      int
	MemoryExtractRetryDelay      time.Duration
	MemoryExtractPublishTimeout  time.Duration
	MemoryExtractTimeout         time.Duration
	MemoryExtractDedupTTL        time.Duration
	MemoryExtractDedupMaxEntries int
	MemoryExtractConsumerEnabled bool
	MemoryExtractReconnectDelay  time.Duration
}

func loadRabbitMQMemoryConfig(ctx context.Context) rabbitMQMemoryConfig {
	cfg := rabbitMQMemoryConfig{
		Exchange:                     defaultRabbitMQExchange,
		MemoryExtractRoutingKey:      defaultMemoryExtractRoutingKey,
		MemoryExtractRetryRoutingKey: defaultMemoryExtractRetryRoutingKey,
		MemoryExtractDLQRoutingKey:   defaultMemoryExtractDLQRoutingKey,
		MemoryExtractQueue:           defaultMemoryExtractQueue,
		MemoryExtractPrefetch:        defaultMemoryExtractPrefetch,
		MemoryExtractMaxRetries:      defaultMemoryExtractMaxRetries,
		MemoryExtractRetryDelay:      defaultMemoryExtractRetryDelay,
		MemoryExtractPublishTimeout:  defaultMemoryExtractPublishTimeout,
		MemoryExtractTimeout:         memoryExtractionTimeout(ctx),
		MemoryExtractDedupTTL:        defaultMemoryExtractDedupTTL,
		MemoryExtractDedupMaxEntries: defaultMemoryExtractDedupMaxEntries,
		MemoryExtractConsumerEnabled: defaultMemoryExtractConsumerEnabled,
		MemoryExtractReconnectDelay:  defaultMemoryExtractReconnectDelay,
	}

	if v, err := g.Cfg().Get(ctx, "rabbitmq.enabled"); err == nil {
		cfg.Enabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.url"); err == nil {
		cfg.URL = ResolveQueueStringFunc(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.exchange"); err == nil {
		cfg.Exchange = ResolveQueueStringFunc(v.String(), cfg.Exchange)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_routing_key"); err == nil {
		cfg.MemoryExtractRoutingKey = ResolveQueueStringFunc(v.String(), cfg.MemoryExtractRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_retry_routing_key"); err == nil {
		cfg.MemoryExtractRetryRoutingKey = ResolveQueueStringFunc(v.String(), cfg.MemoryExtractRetryRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_dlq_routing_key"); err == nil {
		cfg.MemoryExtractDLQRoutingKey = ResolveQueueStringFunc(v.String(), cfg.MemoryExtractDLQRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_queue"); err == nil {
		cfg.MemoryExtractQueue = ResolveQueueStringFunc(v.String(), cfg.MemoryExtractQueue)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_retry_queue"); err == nil {
		cfg.MemoryExtractRetryQueue = ResolveQueueStringFunc(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_dlq"); err == nil {
		cfg.MemoryExtractDLQ = ResolveQueueStringFunc(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.prefetch"); err == nil && v.Int() > 0 {
		cfg.MemoryExtractPrefetch = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.max_retries"); err == nil && v.Int() >= 0 {
		cfg.MemoryExtractMaxRetries = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.retry_delay_ms"); err == nil && v.Int64() > 0 {
		cfg.MemoryExtractRetryDelay = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.publish_timeout_ms"); err == nil && v.Int64() > 0 {
		cfg.MemoryExtractPublishTimeout = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.reconnect_delay_ms"); err == nil && v.Int64() > 0 {
		cfg.MemoryExtractReconnectDelay = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_consumer_enabled"); err == nil {
		cfg.MemoryExtractConsumerEnabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_dedup_ttl_ms"); err == nil && v.Int64() > 0 {
		cfg.MemoryExtractDedupTTL = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_dedup_max_entries"); err == nil && v.Int() > 0 {
		cfg.MemoryExtractDedupMaxEntries = v.Int()
	}

	if strings.TrimSpace(cfg.MemoryExtractRetryQueue) == "" {
		cfg.MemoryExtractRetryQueue = cfg.MemoryExtractQueue + ".retry"
	}
	if strings.TrimSpace(cfg.MemoryExtractDLQ) == "" {
		cfg.MemoryExtractDLQ = cfg.MemoryExtractQueue + ".dlq"
	}
	return cfg
}

func ValidateMemoryExtractionPipelineConfig(ctx context.Context) error {
	return validateRabbitMQMemoryConfig(loadRabbitMQMemoryConfig(ctx))
}

func validateRabbitMQMemoryConfig(cfg rabbitMQMemoryConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("rabbitmq.enabled=true but rabbitmq.url is empty")
	}
	if cfg.MemoryExtractRetryDelay < 0 {
		return fmt.Errorf("rabbitmq.retry_delay_ms must be >= 0")
	}
	if cfg.MemoryExtractReconnectDelay < 0 {
		return fmt.Errorf("rabbitmq.reconnect_delay_ms must be >= 0")
	}
	if cfg.MemoryExtractPublishTimeout < 0 {
		return fmt.Errorf("rabbitmq.publish_timeout_ms must be >= 0")
	}
	if cfg.MemoryExtractMaxRetries < 0 {
		return fmt.Errorf("rabbitmq.max_retries must be >= 0")
	}
	if cfg.MemoryExtractPrefetch <= 0 {
		return fmt.Errorf("rabbitmq.prefetch must be > 0")
	}
	return nil
}