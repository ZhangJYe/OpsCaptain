package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/internal/infra/rabbitmq"
	"SuperBizAgent/utility/metrics"

	"github.com/gogf/gf/v2/frame/g"
	amqp "github.com/rabbitmq/amqp091-go"
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

type rabbitMQMemoryClient struct {
	cfg       rabbitMQMemoryConfig
	client    *rabbitmq.Client
	completed rabbitmq.Deduper
}

var (
	memoryQueueMu        sync.RWMutex
	memoryQueueClient    *rabbitMQMemoryClient
	memoryQueueInitMu    sync.Mutex
	memoryQueueInitStop  context.CancelFunc
	memoryQueueInitDone  chan struct{}
	newMemoryQueueClient = newRabbitMQMemoryClient
	consumeMemoryEvent   = func(c *rabbitMQMemoryClient, event memoryExtractionEvent) error {
		return c.consumeEvent(event)
	}
	publishMemoryEvent = func(c *rabbitMQMemoryClient, ctx context.Context, event memoryExtractionEvent, routingKey string) error {
		return c.publishEvent(ctx, event, routingKey)
	}
	publishMemoryRaw = func(c *rabbitMQMemoryClient, ctx context.Context, body []byte, routingKey string, headers amqp.Table) error {
		return c.publishRaw(ctx, body, routingKey, headers)
	}
	ackMemoryDelivery = func(delivery amqp.Delivery) error {
		return delivery.Ack(false)
	}
	nackMemoryDelivery = func(delivery amqp.Delivery, requeue bool) error {
		return delivery.Nack(false, requeue)
	}
)

func StartMemoryExtractionPipeline(ctx context.Context) (func(context.Context) error, error) {
	cfg := loadRabbitMQMemoryConfig(ctx)
	if err := validateRabbitMQMemoryConfig(cfg); err != nil {
		return func(context.Context) error { return nil }, err
	}
	if !cfg.Enabled {
		_ = stopMemoryQueueInitLoop(context.Background())
		closeAndSwapMemoryQueueClient(nil)
		return func(context.Context) error { return nil }, nil
	}
	startMemoryQueueInitLoop(cfg)

	return func(stopCtx context.Context) error {
		err := stopMemoryQueueInitLoop(stopCtx)
		closeAndSwapMemoryQueueClient(nil)
		return err
	}, nil
}

func enqueueMemoryExtractionDefault(ctx context.Context, sessionID, query, summary string) (bool, error) {
	client := getMemoryQueueClient()
	if client == nil {
		return false, nil
	}
	event := newMemoryExtractionEvent(ctx, sessionID, query, summary, 0)
	if err := client.publishEvent(ctx, event, client.cfg.MemoryExtractRoutingKey); err != nil {
		metrics.ObserveMemoryExtraction("rabbitmq", "publish_failed")
		return false, err
	}
	metrics.ObserveMemoryExtraction("rabbitmq", "published")
	return true, nil
}

func getMemoryQueueClient() *rabbitMQMemoryClient {
	memoryQueueMu.RLock()
	defer memoryQueueMu.RUnlock()
	return memoryQueueClient
}

func startMemoryQueueInitLoop(cfg rabbitMQMemoryConfig) {
	delay := cfg.MemoryExtractReconnectDelay
	if delay <= 0 {
		delay = defaultMemoryExtractReconnectDelay
	}

	initCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	memoryQueueInitMu.Lock()
	previousCancel := memoryQueueInitStop
	previousDone := memoryQueueInitDone
	memoryQueueInitStop = cancel
	memoryQueueInitDone = done
	memoryQueueInitMu.Unlock()

	if previousCancel != nil {
		previousCancel()
		waitMemoryQueueInitStopped(previousDone, 2*time.Second)
	}

	go func() {
		defer close(done)
		for {
			select {
			case <-initCtx.Done():
				return
			default:
			}

			client, err := newMemoryQueueClient(cfg)
			if err == nil {
				if cfg.MemoryExtractConsumerEnabled {
					if err = client.startConsumer(); err != nil {
						_ = client.Close(context.Background())
					}
				}
				if err == nil {
					closeAndSwapMemoryQueueClient(client)
					metrics.ObserveMemoryExtraction("rabbitmq", "bootstrap_connected")
					g.Log().Info(context.Background(), "[memory] rabbitmq memory extraction pipeline connected")
					return
				}
				g.Log().Warningf(context.Background(), "[memory] rabbitmq memory extraction consumer start failed: %v", err)
			} else {
				g.Log().Warningf(context.Background(), "[memory] rabbitmq memory extraction init failed: %v", err)
			}

			metrics.ObserveMemoryExtraction("rabbitmq", "bootstrap_failed")
			if !rabbitmq.SleepReconnect(initCtx, delay) {
				return
			}
		}
	}()
}

func stopMemoryQueueInitLoop(ctx context.Context) error {
	memoryQueueInitMu.Lock()
	cancel := memoryQueueInitStop
	done := memoryQueueInitDone
	memoryQueueInitStop = nil
	memoryQueueInitDone = nil
	memoryQueueInitMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		waitCtx := ctx
		if waitCtx == nil {
			waitCtx = context.Background()
		}
		select {
		case <-done:
		case <-waitCtx.Done():
			return waitCtx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

func waitMemoryQueueInitStopped(done chan struct{}, timeout time.Duration) {
	if done == nil {
		return
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func closeAndSwapMemoryQueueClient(next *rabbitMQMemoryClient) {
	memoryQueueMu.Lock()
	previous := memoryQueueClient
	memoryQueueClient = next
	memoryQueueMu.Unlock()

	if previous != nil && previous != next {
		_ = previous.Close(context.Background())
	}
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

	client := rabbitmq.NewClient(memoryTopology(cfg), "[memory]")
	client.OnReconnectFailed = func(err error) {
		metrics.ObserveMemoryExtraction("rabbitmq", "reconnect_failed")
		g.Log().Warningf(context.Background(), "[memory] rabbitmq reconnect failed: %v", err)
	}
	if err := client.Connect(); err != nil {
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
func buildMemoryDeduper(cfg rabbitMQMemoryConfig, ttl time.Duration, max int) rabbitmq.Deduper {
	local := rabbitmq.NewTTLSet(ttl, max)
	redis := g.Redis()
	if redis == nil {
		g.Log().Warningf(context.Background(), "[memory] redis unavailable, dedup falls back to in-memory (multi-instance may double-extract)")
		return local
	}
	prefix := "opscaption:dedup:memory_extract:"
	return rabbitmq.NewCompositeDeduper(rabbitmq.NewRedisDeduper(redis, prefix, ttl), local)
}

func memoryTopology(cfg rabbitMQMemoryConfig) rabbitmq.TopologyConfig {
	return rabbitmq.TopologyConfig{
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
				metrics.ObserveMemoryExtraction("rabbitmq", "retried")
				c.ack(delivery)
				return
			} else {
				metrics.ObserveMemoryExtraction("rabbitmq", "consume_failed")
				g.Log().Errorf(context.Background(), "[memory] publish retry event failed: %v", publishErr)
				if dlqErr := publishMemoryEvent(c, context.Background(), event, c.cfg.MemoryExtractDLQRoutingKey); dlqErr == nil {
					metrics.ObserveMemoryExtraction("rabbitmq", "dlq")
					// Claim 已标记
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
			// Claim 已标记
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
		cfg.URL = rabbitmq.ResolveRabbitMQString(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.exchange"); err == nil {
		cfg.Exchange = rabbitmq.ResolveRabbitMQString(v.String(), cfg.Exchange)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_routing_key"); err == nil {
		cfg.MemoryExtractRoutingKey = rabbitmq.ResolveRabbitMQString(v.String(), cfg.MemoryExtractRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_retry_routing_key"); err == nil {
		cfg.MemoryExtractRetryRoutingKey = rabbitmq.ResolveRabbitMQString(v.String(), cfg.MemoryExtractRetryRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_dlq_routing_key"); err == nil {
		cfg.MemoryExtractDLQRoutingKey = rabbitmq.ResolveRabbitMQString(v.String(), cfg.MemoryExtractDLQRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_queue"); err == nil {
		cfg.MemoryExtractQueue = rabbitmq.ResolveRabbitMQString(v.String(), cfg.MemoryExtractQueue)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_retry_queue"); err == nil {
		cfg.MemoryExtractRetryQueue = rabbitmq.ResolveRabbitMQString(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_dlq"); err == nil {
		cfg.MemoryExtractDLQ = rabbitmq.ResolveRabbitMQString(v.String(), "")
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
