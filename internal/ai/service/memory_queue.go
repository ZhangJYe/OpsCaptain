package service

import (
	"context"
	"sync"
	"time"

	"SuperBizAgent/utility/metrics"

	"github.com/gogf/gf/v2/frame/g"
	amqp "github.com/rabbitmq/amqp091-go"
)

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
			if !SleepReconnectFunc(initCtx, delay) {
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