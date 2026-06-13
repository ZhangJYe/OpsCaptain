package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func loadRabbitMQChatTaskConfig(ctx context.Context) rabbitMQChatTaskConfig {
	cfg := rabbitMQChatTaskConfig{
		Exchange:          defaultRabbitMQExchange,
		RoutingKey:        defaultChatTaskRoutingKey,
		RetryRoutingKey:   defaultChatTaskRetryRoutingKey,
		DLQRoutingKey:     defaultChatTaskDLQRoutingKey,
		Queue:             defaultChatTaskQueue,
		Prefetch:          defaultChatTaskPrefetch,
		MaxRetries:        defaultChatTaskMaxRetries,
		RetryDelay:        defaultChatTaskRetryDelay,
		ReconnectDelay:    defaultChatTaskReconnectDelay,
		PublishTimeout:    defaultChatTaskPublishTimeout,
		ExecuteTimeout:    defaultChatTaskExecuteTimeout,
		ConsumerEnabled:   defaultChatTaskConsumerEnabled,
		TaskTTL:           defaultChatTaskTTL,
		RedisKeyPrefix:    defaultChatTaskKeyPrefix,
		ConnectionTimeout: time.Duration(defaultChatTaskConnectionTimeoutSec) * time.Second,
	}

	if v, err := g.Cfg().Get(ctx, "chat_async.enabled"); err == nil {
		cfg.Enabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "chat_async.task_ttl_seconds"); err == nil && v.Int64() > 0 {
		cfg.TaskTTL = time.Duration(v.Int64()) * time.Second
	}
	if v, err := g.Cfg().Get(ctx, "chat_async.execute_timeout_ms"); err == nil && v.Int64() > 0 {
		cfg.ExecuteTimeout = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "chat_async.redis_key_prefix"); err == nil {
		cfg.RedisKeyPrefix = ResolveQueueStringFunc(v.String(), cfg.RedisKeyPrefix)
	}

	if v, err := g.Cfg().Get(ctx, "rabbitmq.url"); err == nil {
		cfg.URL = ResolveQueueStringFunc(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.exchange"); err == nil {
		cfg.Exchange = ResolveQueueStringFunc(v.String(), cfg.Exchange)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_routing_key"); err == nil {
		cfg.RoutingKey = ResolveQueueStringFunc(v.String(), cfg.RoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_retry_routing_key"); err == nil {
		cfg.RetryRoutingKey = ResolveQueueStringFunc(v.String(), cfg.RetryRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_dlq_routing_key"); err == nil {
		cfg.DLQRoutingKey = ResolveQueueStringFunc(v.String(), cfg.DLQRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_queue"); err == nil {
		cfg.Queue = ResolveQueueStringFunc(v.String(), cfg.Queue)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_retry_queue"); err == nil {
		cfg.RetryQueue = ResolveQueueStringFunc(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_dlq"); err == nil {
		cfg.DLQ = ResolveQueueStringFunc(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_prefetch"); err == nil && v.Int() > 0 {
		cfg.Prefetch = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_max_retries"); err == nil && v.Int() >= 0 {
		cfg.MaxRetries = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_retry_delay_ms"); err == nil && v.Int64() >= 0 {
		cfg.RetryDelay = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_consumer_enabled"); err == nil {
		cfg.ConsumerEnabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.reconnect_delay_ms"); err == nil && v.Int64() > 0 {
		cfg.ReconnectDelay = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.publish_timeout_ms"); err == nil && v.Int64() > 0 {
		cfg.PublishTimeout = time.Duration(v.Int64()) * time.Millisecond
	}

	if strings.TrimSpace(cfg.RetryQueue) == "" {
		cfg.RetryQueue = cfg.Queue + ".retry"
	}
	if strings.TrimSpace(cfg.DLQ) == "" {
		cfg.DLQ = cfg.Queue + ".dlq"
	}
	return cfg
}

func validateRabbitMQChatTaskConfig(cfg rabbitMQChatTaskConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("chat_async.enabled=true but rabbitmq.url is empty")
	}
	if cfg.Prefetch <= 0 {
		return fmt.Errorf("rabbitmq.chat_task_prefetch must be > 0")
	}
	if cfg.MaxRetries < 0 {
		return fmt.Errorf("rabbitmq.chat_task_max_retries must be >= 0")
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("rabbitmq.chat_task_retry_delay_ms must be >= 0")
	}
	if cfg.PublishTimeout < 0 {
		return fmt.Errorf("rabbitmq.publish_timeout_ms must be >= 0")
	}
	if cfg.ReconnectDelay < 0 {
		return fmt.Errorf("rabbitmq.reconnect_delay_ms must be >= 0")
	}
	if cfg.TaskTTL <= 0 {
		return fmt.Errorf("chat_async.task_ttl_seconds must be > 0")
	}
	if !chatTaskRedisConfigured() {
		return fmt.Errorf("chat_async.enabled=true requires redis.default.address")
	}
	return nil
}
