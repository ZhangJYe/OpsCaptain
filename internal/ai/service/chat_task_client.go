package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/utility/metrics"

	"github.com/gogf/gf/v2/frame/g"
	amqp "github.com/rabbitmq/amqp091-go"
)

func newRabbitMQChatTaskClient(cfg rabbitMQChatTaskConfig) (*rabbitMQChatTaskClient, error) {
	client, err := NewQueueClientFunc(chatTaskTopology(cfg), "[chat_task]", func(err error) {
		metrics.ObserveChatTask("reconnect_failed")
		g.Log().Warningf(context.Background(), "[chat_task] rabbitmq reconnect failed: %v", err)
	})
	if err != nil {
		return nil, err
	}
	return &rabbitMQChatTaskClient{
		cfg:       cfg,
		client:    client,
		completed: buildChatTaskDeduper(cfg),
	}, nil
}

// buildChatTaskDeduper 组合跨实例 Redis 去重 + 本实例热缓存。
// 多实例下：A、B 同时拉到同一条 delivery → 谁先 Claim 谁处理；
// 失败方直接 ack 跳过，避免重复执行 LLM。Redis 不可用时退化到本实例 TTLSet
// （多实例可能漏防，但至少不会因为 deduper 失败而 nack 雪崩）。
func buildChatTaskDeduper(cfg rabbitMQChatTaskConfig) Deduper {
	local := NewTTLSetFunc(10*time.Minute, 20000)
	redis := g.Redis()
	if redis == nil {
		g.Log().Warningf(context.Background(), "[chat_task] redis unavailable, dedup falls back to in-memory (multi-instance may double-execute)")
		return local
	}
	prefix := strings.TrimSpace(cfg.RedisKeyPrefix)
	if prefix == "" {
		prefix = "chat_task:"
	}
	prefix = "opscaption:dedup:" + prefix
	redisDed := NewRedisDeduperFunc(redis, prefix, 10*time.Minute)
	return NewCompositeDeduperFunc(redisDed, local)
}

func chatTaskTopology(cfg rabbitMQChatTaskConfig) QueueClientConfig {
	return QueueClientConfig{
		URL:               cfg.URL,
		Exchange:          cfg.Exchange,
		Queue:             cfg.Queue,
		RoutingKey:        cfg.RoutingKey,
		RetryQueue:        cfg.RetryQueue,
		RetryRoutingKey:   cfg.RetryRoutingKey,
		DLQ:               cfg.DLQ,
		DLQRoutingKey:     cfg.DLQRoutingKey,
		RetryDelay:        cfg.RetryDelay,
		Prefetch:          cfg.Prefetch,
		ConsumerEnabled:   cfg.ConsumerEnabled,
		ConnectionTimeout: cfg.ConnectionTimeout,
	}
}

func (c *rabbitMQChatTaskClient) startConsumer() error {
	if c.client == nil {
		return fmt.Errorf("rabbitmq client not initialized")
	}
	return c.client.StartConsumer(context.Background(), func(delivery amqp.Delivery) {
		c.handleDelivery(delivery)
	}, c.cfg.ReconnectDelay)
}

func (c *rabbitMQChatTaskClient) handleDelivery(delivery amqp.Delivery) {
	event, err := decodeChatTaskEvent(delivery.Body)
	if err != nil {
		metrics.ObserveChatTask("consume_failed")
		if publishErr := c.publishRaw(context.Background(), delivery.Body, c.cfg.DLQRoutingKey, amqp.Table{
			"error": "decode_failed",
		}); publishErr != nil {
			g.Log().Errorf(context.Background(), "[chat_task] publish decode-failed event to dlq failed: %v", publishErr)
			c.nackRequeue(delivery)
			return
		}
		c.ack(delivery)
		return
	}
	if !c.completed.Claim(context.Background(), event.TaskID) {
		metrics.ObserveChatTask("deduped")
		c.ack(delivery)
		return
	}

	if err := c.processEvent(event); err != nil {
		if event.Attempt < c.cfg.MaxRetries {
			event.Attempt++
			publishErr := c.publishEvent(context.Background(), event, c.cfg.RetryRoutingKey)
			if publishErr == nil {
				c.completed.Release(context.Background(), event.TaskID)
				metrics.ObserveChatTask("retried")
				c.ack(delivery)
				return
			}
			metrics.ObserveChatTask("consume_failed")
			g.Log().Errorf(context.Background(), "[chat_task] publish retry event failed: %v", publishErr)
		}

		if dlqErr := c.publishEvent(context.Background(), event, c.cfg.DLQRoutingKey); dlqErr == nil {
			metrics.ObserveChatTask("dlq")
			c.ack(delivery)
			return
		}
		metrics.ObserveChatTask("consume_failed")
		c.nackRequeue(delivery)
		return
	}

	metrics.ObserveChatTask("consumed")
	c.ack(delivery)
}

func (c *rabbitMQChatTaskClient) processEvent(event chatTaskEvent) error {
	exec := getChatTaskExecutor()
	if exec == nil {
		return fmt.Errorf("chat task executor is not registered")
	}

	now := time.Now().Unix()
	record, err := loadChatTaskRecord(context.Background(), c.cfg, event.TaskID)
	if err != nil {
		record = &ChatTaskRecord{
			ID:        event.TaskID,
			SessionID: event.SessionID,
			Query:     event.Query,
			CreatedAt: now,
		}
	}
	record.Status = ChatTaskStatusRunning
	record.StartedAt = now
	record.UpdatedAt = now
	record.Error = ""
	if err := saveChatTaskRecord(context.Background(), c.cfg, record); err != nil {
		return err
	}
	metrics.ObserveChatTask("running")

	execCtx := context.Background()
	cancel := func() {}
	if c.cfg.ExecuteTimeout > 0 {
		execCtx, cancel = context.WithTimeout(execCtx, c.cfg.ExecuteTimeout)
	}
	defer cancel()

	result, err := exec(execCtx, event.SessionID, event.Query)
	record.UpdatedAt = time.Now().Unix()
	record.FinishedAt = record.UpdatedAt
	if err != nil {
		record.Status = ChatTaskStatusFailed
		record.Error = err.Error()
		saveErr := saveChatTaskRecord(context.Background(), c.cfg, record)
		if saveErr != nil {
			return saveErr
		}
		metrics.ObserveChatTask("failed")
		return err
	}

	record.Status = ChatTaskStatusSucceeded
	record.Answer = result.Answer
	record.Detail = append([]string{}, result.Detail...)
	record.TraceID = result.TraceID
	record.Mode = result.Mode
	record.Degraded = result.Degraded
	record.DegradationReason = result.DegradationReason
	record.Error = ""
	if err := saveChatTaskRecord(context.Background(), c.cfg, record); err != nil {
		return err
	}
	metrics.ObserveChatTask("succeeded")
	return nil
}

func (c *rabbitMQChatTaskClient) publishEvent(ctx context.Context, event chatTaskEvent, routingKey string) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	headers := amqp.Table{
		"task_id": event.TaskID,
		"attempt": int32(event.Attempt),
	}
	return c.publishRaw(ctx, body, routingKey, headers)
}

func (c *rabbitMQChatTaskClient) publishRaw(ctx context.Context, body []byte, routingKey string, headers amqp.Table) error {
	if c.client == nil {
		return fmt.Errorf("rabbitmq client not initialized")
	}
	publishCtx := ctx
	if publishCtx == nil {
		publishCtx = context.Background()
	}
	if c.cfg.PublishTimeout > 0 {
		timeoutCtx, cancel := context.WithTimeout(publishCtx, c.cfg.PublishTimeout)
		defer cancel()
		publishCtx = timeoutCtx
	}
	return c.client.Publish(publishCtx, body, routingKey, headers)
}

func (c *rabbitMQChatTaskClient) ack(delivery amqp.Delivery) {
	if err := ackChatTaskDelivery(delivery); err != nil {
		g.Log().Warningf(context.Background(), "[chat_task] delivery ack failed: %v", err)
	}
}

func (c *rabbitMQChatTaskClient) nackRequeue(delivery amqp.Delivery) {
	if err := nackChatTaskDelivery(delivery, true); err != nil {
		g.Log().Warningf(context.Background(), "[chat_task] delivery nack failed: %v", err)
	}
}

func (c *rabbitMQChatTaskClient) Close(_ context.Context) error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

func decodeChatTaskEvent(body []byte) (chatTaskEvent, error) {
	var event chatTaskEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return chatTaskEvent{}, err
	}
	if strings.TrimSpace(event.TaskID) == "" || strings.TrimSpace(event.SessionID) == "" || strings.TrimSpace(event.Query) == "" {
		return chatTaskEvent{}, fmt.Errorf("invalid chat task event")
	}
	return event, nil
}
