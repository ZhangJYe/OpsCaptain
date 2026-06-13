package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
	client    QueueClient
	completed Deduper
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
			if !SleepReconnectFunc(initCtx, delay) {
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
