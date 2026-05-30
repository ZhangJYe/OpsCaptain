package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/infra/rabbitmq"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/metrics"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	defaultChatTaskKeyPrefix            = "opscaptionai:chat_task"
	defaultChatTaskTTL                  = 24 * time.Hour
	defaultChatTaskExecuteTimeout       = 120 * time.Second
	defaultChatTaskRoutingKey           = "chat.task.request"
	defaultChatTaskRetryRoutingKey      = "chat.task.retry"
	defaultChatTaskDLQRoutingKey        = "chat.task.dlq"
	defaultChatTaskQueue                = "opscaption.chat.task"
	defaultChatTaskPrefetch             = 4
	defaultChatTaskMaxRetries           = 3
	defaultChatTaskRetryDelay           = 2 * time.Second
	defaultChatTaskPublishTimeout       = 2 * time.Second
	defaultChatTaskReconnectDelay       = 2 * time.Second
	defaultChatTaskConsumerEnabled      = true
	defaultChatTaskConnectionTimeoutSec = 5
)

type ChatTaskStatus string

const (
	ChatTaskStatusQueued    ChatTaskStatus = "queued"
	ChatTaskStatusRunning   ChatTaskStatus = "running"
	ChatTaskStatusSucceeded ChatTaskStatus = "succeeded"
	ChatTaskStatusFailed    ChatTaskStatus = "failed"
)

type ChatTaskRecord struct {
	ID                string         `json:"id"`
	SessionID         string         `json:"session_id"`
	Query             string         `json:"query"`
	Status            ChatTaskStatus `json:"status"`
	Answer            string         `json:"answer,omitempty"`
	Detail            []string       `json:"detail,omitempty"`
	TraceID           string         `json:"trace_id,omitempty"`
	Mode              string         `json:"mode,omitempty"`
	Degraded          bool           `json:"degraded,omitempty"`
	DegradationReason string         `json:"degradation_reason,omitempty"`
	Error             string         `json:"error,omitempty"`
	CreatedAt         int64          `json:"created_at"`
	UpdatedAt         int64          `json:"updated_at"`
	StartedAt         int64          `json:"started_at,omitempty"`
	FinishedAt        int64          `json:"finished_at,omitempty"`
}

type ChatTaskExecutionResult struct {
	Answer            string
	Detail            []string
	TraceID           string
	Mode              string
	Degraded          bool
	DegradationReason string
}

type chatTaskEvent struct {
	TaskID      string `json:"task_id"`
	SessionID   string `json:"session_id"`
	Query       string `json:"query"`
	RequestedAt int64  `json:"requested_at"`
	Attempt     int    `json:"attempt"`
}

type rabbitMQChatTaskConfig struct {
	Enabled           bool
	URL               string
	Exchange          string
	RoutingKey        string
	RetryRoutingKey   string
	DLQRoutingKey     string
	Queue             string
	RetryQueue        string
	DLQ               string
	Prefetch          int
	MaxRetries        int
	RetryDelay        time.Duration
	ReconnectDelay    time.Duration
	PublishTimeout    time.Duration
	ExecuteTimeout    time.Duration
	ConsumerEnabled   bool
	TaskTTL           time.Duration
	RedisKeyPrefix    string
	ConnectionTimeout time.Duration
}

type rabbitMQChatTaskClient struct {
	cfg       rabbitMQChatTaskConfig
	client    *rabbitmq.Client
	completed *rabbitmq.TTLSet
}

var (
	chatTaskQueueMu      sync.RWMutex
	chatTaskQueueClient  *rabbitMQChatTaskClient
	chatTaskInitMu       sync.Mutex
	chatTaskInitStop     context.CancelFunc
	chatTaskInitDone     chan struct{}
	newChatTaskClient    = newRabbitMQChatTaskClient
	chatTaskExecutorMu   sync.RWMutex
	chatTaskExecutorFunc func(context.Context, string, string) (ChatTaskExecutionResult, error)

	ackChatTaskDelivery = func(delivery amqp.Delivery) error {
		return delivery.Ack(false)
	}
	nackChatTaskDelivery = func(delivery amqp.Delivery, requeue bool) error {
		return delivery.Nack(false, requeue)
	}
	chatTaskRedisConfigured = func() bool {
		v, err := g.Cfg().Get(context.Background(), "redis.default.address")
		if err != nil {
			return false
		}
		_, ok := common.ResolveOptionalEnv(v.String())
		return ok
	}
)

func RegisterChatTaskExecutor(fn func(context.Context, string, string) (ChatTaskExecutionResult, error)) {
	chatTaskExecutorMu.Lock()
	chatTaskExecutorFunc = fn
	chatTaskExecutorMu.Unlock()
}

func StartChatTaskPipeline(ctx context.Context) (func(context.Context) error, error) {
	cfg := loadRabbitMQChatTaskConfig(ctx)
	if err := validateRabbitMQChatTaskConfig(cfg); err != nil {
		return func(context.Context) error { return nil }, err
	}
	if !cfg.Enabled {
		_ = stopChatTaskInitLoop(context.Background())
		closeAndSwapChatTaskClient(nil)
		return func(context.Context) error { return nil }, nil
	}
	startChatTaskInitLoop(cfg)
	return func(stopCtx context.Context) error {
		err := stopChatTaskInitLoop(stopCtx)
		closeAndSwapChatTaskClient(nil)
		return err
	}, nil
}

func ValidateChatTaskPipelineConfig(ctx context.Context) error {
	return validateRabbitMQChatTaskConfig(loadRabbitMQChatTaskConfig(ctx))
}

func SubmitChatTask(ctx context.Context, sessionID, query string) (*ChatTaskRecord, error) {
	cfg := loadRabbitMQChatTaskConfig(ctx)
	if err := validateRabbitMQChatTaskConfig(cfg); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("chat async queue is not enabled")
	}

	now := time.Now().Unix()
	record := &ChatTaskRecord{
		ID:        uuid.NewString(),
		SessionID: strings.TrimSpace(sessionID),
		Query:     strings.TrimSpace(query),
		Status:    ChatTaskStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if record.SessionID == "" {
		return nil, fmt.Errorf("session id is empty")
	}
	if record.Query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	if err := saveChatTaskRecord(ctx, cfg, record); err != nil {
		return nil, err
	}

	client := getChatTaskQueueClient()
	if client == nil {
		record.Status = ChatTaskStatusFailed
		record.Error = "chat task queue is not ready"
		record.UpdatedAt = time.Now().Unix()
		_ = saveChatTaskRecord(ctx, cfg, record)
		return nil, fmt.Errorf("chat task queue is not ready")
	}

	event := chatTaskEvent{
		TaskID:      record.ID,
		SessionID:   record.SessionID,
		Query:       record.Query,
		RequestedAt: time.Now().UnixMilli(),
		Attempt:     0,
	}
	if err := client.publishEvent(ctx, event, client.cfg.RoutingKey); err != nil {
		record.Status = ChatTaskStatusFailed
		record.Error = err.Error()
		record.UpdatedAt = time.Now().Unix()
		_ = saveChatTaskRecord(ctx, cfg, record)
		return nil, err
	}

	metrics.ObserveChatTask("submitted")
	return record, nil
}

func GetChatTask(ctx context.Context, taskID string) (*ChatTaskRecord, error) {
	cfg := loadRabbitMQChatTaskConfig(ctx)
	return loadChatTaskRecord(ctx, cfg, taskID)
}

func startChatTaskInitLoop(cfg rabbitMQChatTaskConfig) {
	delay := cfg.ReconnectDelay
	if delay <= 0 {
		delay = defaultChatTaskReconnectDelay
	}

	initCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	chatTaskInitMu.Lock()
	previousCancel := chatTaskInitStop
	previousDone := chatTaskInitDone
	chatTaskInitStop = cancel
	chatTaskInitDone = done
	chatTaskInitMu.Unlock()

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

			client, err := newChatTaskClient(cfg)
			if err == nil {
				if cfg.ConsumerEnabled {
					if err = client.startConsumer(); err != nil {
						_ = client.Close(context.Background())
					}
				}
				if err == nil {
					closeAndSwapChatTaskClient(client)
					metrics.ObserveChatTask("bootstrap_connected")
					g.Log().Info(context.Background(), "[chat_task] rabbitmq chat task pipeline connected")
					return
				}
				g.Log().Warningf(context.Background(), "[chat_task] start consumer failed: %v", err)
			} else {
				g.Log().Warningf(context.Background(), "[chat_task] init failed: %v", err)
			}

			metrics.ObserveChatTask("bootstrap_failed")
			if !rabbitmq.SleepReconnect(initCtx, delay) {
				return
			}
		}
	}()
}

func stopChatTaskInitLoop(ctx context.Context) error {
	chatTaskInitMu.Lock()
	cancel := chatTaskInitStop
	done := chatTaskInitDone
	chatTaskInitStop = nil
	chatTaskInitDone = nil
	chatTaskInitMu.Unlock()

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

func getChatTaskQueueClient() *rabbitMQChatTaskClient {
	chatTaskQueueMu.RLock()
	defer chatTaskQueueMu.RUnlock()
	return chatTaskQueueClient
}

func closeAndSwapChatTaskClient(next *rabbitMQChatTaskClient) {
	chatTaskQueueMu.Lock()
	previous := chatTaskQueueClient
	chatTaskQueueClient = next
	chatTaskQueueMu.Unlock()

	if previous != nil && previous != next {
		_ = previous.Close(context.Background())
	}
}

func getChatTaskExecutor() func(context.Context, string, string) (ChatTaskExecutionResult, error) {
	chatTaskExecutorMu.RLock()
	defer chatTaskExecutorMu.RUnlock()
	return chatTaskExecutorFunc
}

func newRabbitMQChatTaskClient(cfg rabbitMQChatTaskConfig) (*rabbitMQChatTaskClient, error) {
	client := rabbitmq.NewClient(chatTaskTopology(cfg), "[chat_task]")
	client.OnReconnectFailed = func(err error) {
		metrics.ObserveChatTask("reconnect_failed")
		g.Log().Warningf(context.Background(), "[chat_task] rabbitmq reconnect failed: %v", err)
	}
	if err := client.Connect(); err != nil {
		return nil, err
	}
	return &rabbitMQChatTaskClient{
		cfg:       cfg,
		client:    client,
		completed: rabbitmq.NewTTLSet(10*time.Minute, 20000),
	}, nil
}

func chatTaskTopology(cfg rabbitMQChatTaskConfig) rabbitmq.TopologyConfig {
	return rabbitmq.TopologyConfig{
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
	if c.completed.Has(event.TaskID) {
		metrics.ObserveChatTask("deduped")
		c.ack(delivery)
		return
	}

	if err := c.processEvent(event); err != nil {
		if event.Attempt < c.cfg.MaxRetries {
			event.Attempt++
			publishErr := c.publishEvent(context.Background(), event, c.cfg.RetryRoutingKey)
			if publishErr == nil {
				metrics.ObserveChatTask("retried")
				c.ack(delivery)
				return
			}
			metrics.ObserveChatTask("consume_failed")
			g.Log().Errorf(context.Background(), "[chat_task] publish retry event failed: %v", publishErr)
		}

		if dlqErr := c.publishEvent(context.Background(), event, c.cfg.DLQRoutingKey); dlqErr == nil {
			metrics.ObserveChatTask("dlq")
			c.completed.Mark(event.TaskID)
			c.ack(delivery)
			return
		}
		metrics.ObserveChatTask("consume_failed")
		c.nackRequeue(delivery)
		return
	}

	c.completed.Mark(event.TaskID)
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
		cfg.RedisKeyPrefix = rabbitmq.ResolveRabbitMQString(v.String(), cfg.RedisKeyPrefix)
	}

	if v, err := g.Cfg().Get(ctx, "rabbitmq.url"); err == nil {
		cfg.URL = rabbitmq.ResolveRabbitMQString(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.exchange"); err == nil {
		cfg.Exchange = rabbitmq.ResolveRabbitMQString(v.String(), cfg.Exchange)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_routing_key"); err == nil {
		cfg.RoutingKey = rabbitmq.ResolveRabbitMQString(v.String(), cfg.RoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_retry_routing_key"); err == nil {
		cfg.RetryRoutingKey = rabbitmq.ResolveRabbitMQString(v.String(), cfg.RetryRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_dlq_routing_key"); err == nil {
		cfg.DLQRoutingKey = rabbitmq.ResolveRabbitMQString(v.String(), cfg.DLQRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_queue"); err == nil {
		cfg.Queue = rabbitmq.ResolveRabbitMQString(v.String(), cfg.Queue)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_retry_queue"); err == nil {
		cfg.RetryQueue = rabbitmq.ResolveRabbitMQString(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.chat_task_dlq"); err == nil {
		cfg.DLQ = rabbitmq.ResolveRabbitMQString(v.String(), "")
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

func saveChatTaskRecord(ctx context.Context, cfg rabbitMQChatTaskConfig, record *ChatTaskRecord) error {
	if record == nil {
		return fmt.Errorf("chat task record is nil")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	ttl := int(cfg.TaskTTL.Seconds())
	if ttl <= 0 {
		ttl = int(defaultChatTaskTTL.Seconds())
	}
	_, err = g.Redis().Do(ctx, "SETEX", chatTaskRecordKey(cfg.RedisKeyPrefix, record.ID), ttl, string(payload))
	return err
}

func loadChatTaskRecord(ctx context.Context, cfg rabbitMQChatTaskConfig, taskID string) (*ChatTaskRecord, error) {
	key := chatTaskRecordKey(cfg.RedisKeyPrefix, taskID)
	val, err := g.Redis().Do(ctx, "GET", key)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(val.String())
	if raw == "" {
		return nil, fmt.Errorf("chat task %s not found", strings.TrimSpace(taskID))
	}
	var record ChatTaskRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func chatTaskRecordKey(prefix, taskID string) string {
	p := strings.TrimSpace(prefix)
	if p == "" {
		p = defaultChatTaskKeyPrefix
	}
	return fmt.Sprintf("%s:task:%s", p, strings.TrimSpace(taskID))
}
