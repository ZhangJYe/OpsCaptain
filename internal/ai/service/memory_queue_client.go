package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/metrics"

	"github.com/gogf/gf/v2/frame/g"
	amqp "github.com/rabbitmq/amqp091-go"
)

type memoryExtractionEvent struct {
	EventID     string `json:"event_id"`
	SessionID   string `json:"session_id"`
	UserID      string `json:"user_id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
	Query       string `json:"query"`
	Summary     string `json:"summary"`
	RequestedAt int64  `json:"requested_at"`
	Attempt     int    `json:"attempt"`
}

type rabbitMQMemoryClient struct {
	cfg       rabbitMQMemoryConfig
	client    QueueClient
	completed Deduper
}

func newRabbitMQMemoryClient(cfg rabbitMQMemoryConfig) (*rabbitMQMemoryClient, error) {
	dedupTTL := cfg.MemoryExtractDedupTTL
	if dedupTTL <= 0 {
		dedupTTL = defaultMemoryExtractDedupTTL
	}
	dedupMax := cfg.MemoryExtractDedupMaxEntries
	if dedupMax <= 0 {
		dedupMax = defaultMemoryExtractDedupMaxEntries
	}

	client, err := NewQueueClientFunc(queueTopology(cfg), "[memory]", func(err error) {
		metrics.ObserveMemoryExtraction("rabbitmq", "reconnect_failed")
		g.Log().Warningf(context.Background(), "[memory] rabbitmq reconnect failed: %v", err)
	})
	if err != nil {
		return nil, err
	}

	return &rabbitMQMemoryClient{
		cfg:       cfg,
		client:    client,
		completed: buildMemoryDeduper(cfg, dedupTTL, dedupMax),
	}, nil
}

// buildMemoryDeduper 跨实例 Redis SETNX + 本实例 TTLSet 双层去重。
// 多实例 consumer 同时拉到同一条 memory 抽取事件时，谁先 Claim 谁干活，
// 失败方 ack 跳过，避免对同一条对话重复跑 LLM 抽取。
func buildMemoryDeduper(cfg rabbitMQMemoryConfig, ttl time.Duration, max int) Deduper {
	local := NewTTLSetFunc(ttl, max)
	redis := g.Redis()
	if redis == nil {
		g.Log().Warningf(context.Background(), "[memory] redis unavailable, dedup falls back to in-memory (multi-instance may double-extract)")
		return local
	}
	prefix := "opscaption:dedup:memory_extract:"
	return NewCompositeDeduperFunc(NewRedisDeduperFunc(redis, prefix, ttl), local)
}

func queueTopology(cfg rabbitMQMemoryConfig) QueueClientConfig {
	return QueueClientConfig{
		URL:               cfg.URL,
		Exchange:          cfg.Exchange,
		Queue:             cfg.MemoryExtractQueue,
		RoutingKey:        cfg.MemoryExtractRoutingKey,
		RetryQueue:        cfg.MemoryExtractRetryQueue,
		RetryRoutingKey:   cfg.MemoryExtractRetryRoutingKey,
		DLQ:               cfg.MemoryExtractDLQ,
		DLQRoutingKey:     cfg.MemoryExtractDLQRoutingKey,
		RetryDelay:        cfg.MemoryExtractRetryDelay,
		Prefetch:          cfg.MemoryExtractPrefetch,
		ConsumerEnabled:   cfg.MemoryExtractConsumerEnabled,
		ConnectionTimeout: time.Duration(defaultMemoryExtractConnectionTimeoutSec) * time.Second,
	}
}

func (c *rabbitMQMemoryClient) startConsumer() error {
	if c.client == nil {
		return fmt.Errorf("rabbitmq client not initialized")
	}
	return c.client.StartConsumer(context.Background(), func(delivery amqp.Delivery) {
		c.handleDelivery(delivery)
	}, c.cfg.MemoryExtractReconnectDelay)
}

func (c *rabbitMQMemoryClient) handleDelivery(delivery amqp.Delivery) {
	event, err := decodeMemoryExtractionEvent(delivery.Body)
	if err != nil {
		metrics.ObserveMemoryExtraction("rabbitmq", "consume_failed")
		if publishErr := publishMemoryRaw(c, context.Background(), delivery.Body, c.cfg.MemoryExtractDLQRoutingKey, amqp.Table{
			"error": "decode_failed",
		}); publishErr != nil {
			g.Log().Errorf(context.Background(), "[memory] publish decode-failed event to dlq failed: %v", publishErr)
			c.nackRequeue(delivery)
			return
		}
		c.ack(delivery)
		return
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = buildMemoryExtractionEventID(event.SessionID, event.Query, event.Summary)
	}
	if !c.completed.Claim(context.Background(), event.EventID) {
		metrics.ObserveMemoryExtraction("rabbitmq", "deduped")
		c.ack(delivery)
		return
	}

	if err := consumeMemoryEvent(c, event); err != nil {
		if event.Attempt < c.cfg.MemoryExtractMaxRetries {
			event.Attempt++
			if publishErr := publishMemoryEvent(c, context.Background(), event, c.cfg.MemoryExtractRetryRoutingKey); publishErr == nil {
				c.completed.Release(context.Background(), event.EventID)
				metrics.ObserveMemoryExtraction("rabbitmq", "retried")
				c.ack(delivery)
				return
			} else {
				metrics.ObserveMemoryExtraction("rabbitmq", "consume_failed")
				g.Log().Errorf(context.Background(), "[memory] publish retry event failed: %v", publishErr)
				c.completed.Claim(context.Background(), event.EventID)
				if dlqErr := publishMemoryEvent(c, context.Background(), event, c.cfg.MemoryExtractDLQRoutingKey); dlqErr == nil {
					metrics.ObserveMemoryExtraction("rabbitmq", "dlq")
					c.ack(delivery)
					return
				} else {
					metrics.ObserveMemoryExtraction("rabbitmq", "consume_failed")
					g.Log().Errorf(context.Background(), "[memory] publish retry-failed event to dlq failed: %v", dlqErr)
				}
			}
			c.nackRequeue(delivery)
			return
		}
		if dlqErr := publishMemoryEvent(c, context.Background(), event, c.cfg.MemoryExtractDLQRoutingKey); dlqErr == nil {
			metrics.ObserveMemoryExtraction("rabbitmq", "dlq")
			c.ack(delivery)
			return
		} else {
			metrics.ObserveMemoryExtraction("rabbitmq", "consume_failed")
			g.Log().Errorf(context.Background(), "[memory] publish max-retry event to dlq failed: %v", dlqErr)
		}
		c.nackRequeue(delivery)
		return
	}

	// Claim 已在入口标记
	metrics.ObserveMemoryExtraction("rabbitmq", "consumed")
	c.ack(delivery)
}

func (c *rabbitMQMemoryClient) ack(delivery amqp.Delivery) {
	if err := ackMemoryDelivery(delivery); err != nil {
		g.Log().Warningf(context.Background(), "[memory] delivery ack failed: %v", err)
	}
}

func (c *rabbitMQMemoryClient) nackRequeue(delivery amqp.Delivery) {
	if err := nackMemoryDelivery(delivery, true); err != nil {
		g.Log().Warningf(context.Background(), "[memory] delivery nack failed: %v", err)
	}
}

func (c *rabbitMQMemoryClient) consumeEvent(event memoryExtractionEvent) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("memory extract panic: %v", recovered)
		}
	}()

	extractCtx, cancel := context.WithTimeout(context.Background(), c.cfg.MemoryExtractTimeout)
	defer cancel()

	report := processMemoryEventFunc(extractCtx, memory.MemoryEvent{
		SessionID: event.SessionID,
		UserID:    event.UserID,
		ProjectID: event.ProjectID,
		Query:     event.Query,
		Answer:    event.Summary,
		TraceID:   event.TraceID,
	})
	if extractCtx.Err() != nil {
		return extractCtx.Err()
	}
	if report != nil && len(report.Dropped) > 0 {
		g.Log().Debugf(context.Background(), "[memory] dropped %d memory candidates for session %s", len(report.Dropped), event.SessionID)
	}
	return nil
}

func (c *rabbitMQMemoryClient) publishEvent(ctx context.Context, event memoryExtractionEvent, routingKey string) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	headers := amqp.Table{
		"event_id": event.EventID,
		"attempt":  int32(event.Attempt),
	}
	return c.publishRaw(ctx, body, routingKey, headers)
}

func (c *rabbitMQMemoryClient) publishRaw(ctx context.Context, body []byte, routingKey string, headers amqp.Table) error {
	if c.client == nil {
		return fmt.Errorf("rabbitmq client not initialized")
	}
	publishCtx := ctx
	if publishCtx == nil {
		publishCtx = context.Background()
	}
	if c.cfg.MemoryExtractPublishTimeout > 0 {
		timeoutCtx, cancel := context.WithTimeout(publishCtx, c.cfg.MemoryExtractPublishTimeout)
		defer cancel()
		publishCtx = timeoutCtx
	}
	return c.client.Publish(publishCtx, body, routingKey, headers)
}

func (c *rabbitMQMemoryClient) Close(_ context.Context) error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

func decodeMemoryExtractionEvent(body []byte) (memoryExtractionEvent, error) {
	var event memoryExtractionEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return memoryExtractionEvent{}, err
	}
	if strings.TrimSpace(event.SessionID) == "" || strings.TrimSpace(event.Query) == "" || strings.TrimSpace(event.Summary) == "" {
		return memoryExtractionEvent{}, fmt.Errorf("invalid memory extraction event")
	}
	return event, nil
}

func newMemoryExtractionEvent(ctx context.Context, sessionID, query, summary string, attempt int) memoryExtractionEvent {
	requestedAt := time.Now().UnixMilli()
	traceID := ""
	if value, ok := ctx.Value(consts.CtxKeyTraceID).(string); ok {
		traceID = strings.TrimSpace(value)
	}
	return memoryExtractionEvent{
		EventID:     buildMemoryExtractionEventID(sessionID, query, summary),
		SessionID:   sessionID,
		UserID:      memoryUserID(ctx),
		ProjectID:   memoryProjectID(ctx),
		TraceID:     traceID,
		Query:       query,
		Summary:     summary,
		RequestedAt: requestedAt,
		Attempt:     attempt,
	}
}

func buildMemoryExtractionEventID(sessionID, query, summary string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(sessionID))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(query))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(summary))
	return hex.EncodeToString(hasher.Sum(nil))
}