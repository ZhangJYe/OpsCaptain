package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/utility/common"
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
	conn      *amqp.Connection
	publishCh *amqp.Channel
	consumeCh *amqp.Channel

	stateMu     sync.RWMutex
	reconnectMu sync.Mutex
	publishMu   sync.Mutex

	consumeCtx    context.Context
	consumeCancel context.CancelFunc
	consumeDone   chan struct{}
	closed        bool

	completed *ttlSet
}

type ttlSet struct {
	mu         sync.Mutex
	items      map[string]time.Time
	ttl        time.Duration
	maxEntries int
}

var (
	memoryQueueMu      sync.RWMutex
	memoryQueueClient  *rabbitMQMemoryClient
	consumeMemoryEvent = func(c *rabbitMQMemoryClient, event memoryExtractionEvent) error {
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
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	client, err := newRabbitMQMemoryClient(cfg)
	if err != nil {
		return func(context.Context) error { return nil }, err
	}
	if cfg.MemoryExtractConsumerEnabled {
		if err := client.startConsumer(); err != nil {
			_ = client.Close(context.Background())
			return func(context.Context) error { return nil }, err
		}
	}

	memoryQueueMu.Lock()
	previous := memoryQueueClient
	memoryQueueClient = client
	memoryQueueMu.Unlock()

	if previous != nil {
		_ = previous.Close(context.Background())
	}

	return func(stopCtx context.Context) error {
		memoryQueueMu.Lock()
		if memoryQueueClient == client {
			memoryQueueClient = nil
		}
		memoryQueueMu.Unlock()
		return client.Close(stopCtx)
	}, nil
}

func enqueueMemoryExtractionDefault(ctx context.Context, sessionID, query, summary string) (bool, error) {
	client := getMemoryQueueClient()
	if client == nil {
		return false, nil
	}
	event := newMemoryExtractionEvent(sessionID, query, summary, 0)
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

func newRabbitMQMemoryClient(cfg rabbitMQMemoryConfig) (*rabbitMQMemoryClient, error) {
	client := &rabbitMQMemoryClient{
		cfg:       cfg,
		completed: newTTLSet(cfg.MemoryExtractDedupTTL, cfg.MemoryExtractDedupMaxEntries),
	}

	if err := client.connect(); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *rabbitMQMemoryClient) connect() error {
	conn, publishCh, consumeCh, err := openRabbitMQChannels(c.cfg)
	if err != nil {
		return err
	}

	c.stateMu.Lock()
	c.conn = conn
	c.publishCh = publishCh
	c.consumeCh = consumeCh
	c.stateMu.Unlock()
	return nil
}

func openRabbitMQChannels(cfg rabbitMQMemoryConfig) (*amqp.Connection, *amqp.Channel, *amqp.Channel, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, nil, nil, fmt.Errorf("rabbitmq url is empty")
	}
	conn, err := amqp.DialConfig(cfg.URL, amqp.Config{
		Dial: amqp.DefaultDial(time.Duration(defaultMemoryExtractConnectionTimeoutSec) * time.Second),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect rabbitmq failed: %w", err)
	}

	publishCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, nil, fmt.Errorf("open publish channel failed: %w", err)
	}

	if err := declareMemoryTopology(publishCh, cfg); err != nil {
		_ = publishCh.Close()
		_ = conn.Close()
		return nil, nil, nil, err
	}

	var consumeCh *amqp.Channel
	if cfg.MemoryExtractConsumerEnabled {
		consumeCh, err = conn.Channel()
		if err != nil {
			_ = publishCh.Close()
			_ = conn.Close()
			return nil, nil, nil, fmt.Errorf("open consume channel failed: %w", err)
		}
		if err := consumeCh.Qos(cfg.MemoryExtractPrefetch, 0, false); err != nil {
			_ = consumeCh.Close()
			_ = publishCh.Close()
			_ = conn.Close()
			return nil, nil, nil, fmt.Errorf("set consume qos failed: %w", err)
		}
	}

	return conn, publishCh, consumeCh, nil
}

func declareMemoryTopology(ch *amqp.Channel, cfg rabbitMQMemoryConfig) error {
	if err := ch.ExchangeDeclare(cfg.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange failed: %w", err)
	}

	if _, err := ch.QueueDeclare(cfg.MemoryExtractQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue failed: %w", err)
	}
	if err := ch.QueueBind(cfg.MemoryExtractQueue, cfg.MemoryExtractRoutingKey, cfg.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue failed: %w", err)
	}

	retryArgs := amqp.Table{
		"x-dead-letter-exchange":    cfg.Exchange,
		"x-dead-letter-routing-key": cfg.MemoryExtractRoutingKey,
		"x-message-ttl":             int32(cfg.MemoryExtractRetryDelay.Milliseconds()),
	}
	if _, err := ch.QueueDeclare(cfg.MemoryExtractRetryQueue, true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("declare retry queue failed: %w", err)
	}
	if err := ch.QueueBind(cfg.MemoryExtractRetryQueue, cfg.MemoryExtractRetryRoutingKey, cfg.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind retry queue failed: %w", err)
	}

	if _, err := ch.QueueDeclare(cfg.MemoryExtractDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dlq failed: %w", err)
	}
	if err := ch.QueueBind(cfg.MemoryExtractDLQ, cfg.MemoryExtractDLQRoutingKey, cfg.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind dlq failed: %w", err)
	}

	return nil
}

func (c *rabbitMQMemoryClient) startConsumer() error {
	if !c.cfg.MemoryExtractConsumerEnabled {
		return nil
	}

	c.stateMu.Lock()
	if c.consumeDone != nil {
		c.stateMu.Unlock()
		return nil
	}
	c.consumeCtx, c.consumeCancel = context.WithCancel(context.Background())
	c.consumeDone = make(chan struct{})
	consumeCtx := c.consumeCtx
	consumeDone := c.consumeDone
	c.stateMu.Unlock()

	go func() {
		defer close(consumeDone)
		for {
			select {
			case <-consumeCtx.Done():
				return
			default:
			}

			deliveries, err := c.openConsumerDeliveries()
			if err != nil {
				metrics.ObserveMemoryExtraction("rabbitmq", "consume_failed")
				g.Log().Warningf(context.Background(), "[memory] open consumer deliveries failed: %v", err)
				if !sleepMemoryReconnect(consumeCtx, c.cfg.MemoryExtractReconnectDelay) {
					return
				}
				if reconnectErr := c.reconnect(context.Background()); reconnectErr != nil {
					g.Log().Warningf(context.Background(), "[memory] reconnect after consume open failure failed: %v", reconnectErr)
				}
				continue
			}

		consumeLoop:
			for {
				select {
				case <-consumeCtx.Done():
					return
				case delivery, ok := <-deliveries:
					if !ok {
						metrics.ObserveMemoryExtraction("rabbitmq", "consume_channel_closed")
						if !sleepMemoryReconnect(consumeCtx, c.cfg.MemoryExtractReconnectDelay) {
							return
						}
						if reconnectErr := c.reconnect(context.Background()); reconnectErr != nil {
							g.Log().Warningf(context.Background(), "[memory] reconnect after consume channel closed failed: %v", reconnectErr)
						}
						break consumeLoop
					}
					c.handleDelivery(delivery)
				}
			}
		}
	}()
	return nil
}

func (c *rabbitMQMemoryClient) openConsumerDeliveries() (<-chan amqp.Delivery, error) {
	consumeCh := c.getConsumeChannel()
	if consumeCh == nil {
		if err := c.reconnect(context.Background()); err != nil {
			return nil, err
		}
		consumeCh = c.getConsumeChannel()
		if consumeCh == nil {
			return nil, fmt.Errorf("rabbitmq consume channel unavailable")
		}
	}

	deliveries, err := consumeCh.Consume(c.cfg.MemoryExtractQueue, "", false, false, false, false, nil)
	if err == nil {
		return deliveries, nil
	}
	if reconnectErr := c.reconnect(context.Background()); reconnectErr != nil {
		return nil, err
	}
	consumeCh = c.getConsumeChannel()
	if consumeCh == nil {
		return nil, err
	}
	return consumeCh.Consume(c.cfg.MemoryExtractQueue, "", false, false, false, false, nil)
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
		event.EventID = buildMemoryExtractionEventID(event.SessionID, event.Query, event.Summary, event.RequestedAt)
	}
	if c.completed.Has(event.EventID) {
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
					c.completed.Mark(event.EventID)
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
			c.completed.Mark(event.EventID)
			c.ack(delivery)
			return
		} else {
			metrics.ObserveMemoryExtraction("rabbitmq", "consume_failed")
			g.Log().Errorf(context.Background(), "[memory] publish max-retry event to dlq failed: %v", dlqErr)
		}
		c.nackRequeue(delivery)
		return
	}

	c.completed.Mark(event.EventID)
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

	report := extractMemoriesFunc(extractCtx, event.SessionID, event.Query, event.Summary)
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
	publishCtx := ctx
	if publishCtx == nil {
		publishCtx = context.Background()
	}
	if c.cfg.MemoryExtractPublishTimeout > 0 {
		timeoutCtx, cancel := context.WithTimeout(publishCtx, c.cfg.MemoryExtractPublishTimeout)
		defer cancel()
		publishCtx = timeoutCtx
	}

	c.publishMu.Lock()
	defer c.publishMu.Unlock()

	if c.isClosed() {
		return fmt.Errorf("rabbitmq client closed")
	}

	if err := c.publishWithCurrentChannel(publishCtx, body, routingKey, headers); err == nil {
		return nil
	} else {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if reconnectErr := c.reconnect(context.Background()); reconnectErr != nil {
			return fmt.Errorf("publish failed: %v; reconnect failed: %w", err, reconnectErr)
		}
		if retryErr := c.publishWithCurrentChannel(publishCtx, body, routingKey, headers); retryErr != nil {
			return fmt.Errorf("publish failed after reconnect: %w", retryErr)
		}
	}
	return nil
}

func (c *rabbitMQMemoryClient) Close(ctx context.Context) error {
	var errs []string

	c.stateMu.Lock()
	if c.closed {
		c.stateMu.Unlock()
		return nil
	}
	c.closed = true
	consumeCancel := c.consumeCancel
	consumeDone := c.consumeDone
	c.stateMu.Unlock()

	if consumeCancel != nil {
		consumeCancel()
	}

	if consumeDone != nil {
		waitCtx := ctx
		if waitCtx == nil {
			waitCtx = context.Background()
		}
		select {
		case <-consumeDone:
		case <-waitCtx.Done():
			errs = append(errs, waitCtx.Err().Error())
		case <-time.After(time.Second):
		}
	}

	c.reconnectMu.Lock()
	oldConn, oldPublishCh, oldConsumeCh := c.swapAMQPState(nil, nil, nil)
	c.reconnectMu.Unlock()

	if err := closeMemoryChannel(oldConsumeCh); err != nil {
		errs = append(errs, err.Error())
	}
	if err := closeMemoryChannel(oldPublishCh); err != nil {
		errs = append(errs, err.Error())
	}
	if err := closeMemoryConnection(oldConn); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (c *rabbitMQMemoryClient) publishWithCurrentChannel(ctx context.Context, body []byte, routingKey string, headers amqp.Table) error {
	publishCh := c.getPublishChannel()
	if publishCh == nil {
		return fmt.Errorf("rabbitmq publish channel unavailable")
	}
	return publishCh.PublishWithContext(
		ctx,
		c.cfg.Exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Headers:      headers,
			Body:         body,
		},
	)
}

func (c *rabbitMQMemoryClient) reconnect(ctx context.Context) error {
	_ = ctx
	if c.isClosed() {
		return fmt.Errorf("rabbitmq client closed")
	}

	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	if c.isClosed() {
		return fmt.Errorf("rabbitmq client closed")
	}

	conn, publishCh, consumeCh, err := openRabbitMQChannels(c.cfg)
	if err != nil {
		return err
	}
	oldConn, oldPublishCh, oldConsumeCh := c.swapAMQPState(conn, publishCh, consumeCh)
	_ = closeMemoryChannel(oldConsumeCh)
	_ = closeMemoryChannel(oldPublishCh)
	_ = closeMemoryConnection(oldConn)
	metrics.ObserveMemoryExtraction("rabbitmq", "reconnected")
	return nil
}

func (c *rabbitMQMemoryClient) swapAMQPState(
	conn *amqp.Connection,
	publishCh *amqp.Channel,
	consumeCh *amqp.Channel,
) (*amqp.Connection, *amqp.Channel, *amqp.Channel) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	oldConn := c.conn
	oldPublishCh := c.publishCh
	oldConsumeCh := c.consumeCh
	c.conn = conn
	c.publishCh = publishCh
	c.consumeCh = consumeCh
	return oldConn, oldPublishCh, oldConsumeCh
}

func (c *rabbitMQMemoryClient) getPublishChannel() *amqp.Channel {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.publishCh
}

func (c *rabbitMQMemoryClient) getConsumeChannel() *amqp.Channel {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.consumeCh
}

func (c *rabbitMQMemoryClient) isClosed() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.closed
}

func closeMemoryChannel(ch *amqp.Channel) error {
	if ch == nil {
		return nil
	}
	if err := ch.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
		return err
	}
	return nil
}

func closeMemoryConnection(conn *amqp.Connection) error {
	if conn == nil {
		return nil
	}
	if err := conn.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
		return err
	}
	return nil
}

func sleepMemoryReconnect(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		delay = defaultMemoryExtractReconnectDelay
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

func newMemoryExtractionEvent(sessionID, query, summary string, attempt int) memoryExtractionEvent {
	requestedAt := time.Now().UnixMilli()
	return memoryExtractionEvent{
		EventID:     buildMemoryExtractionEventID(sessionID, query, summary, requestedAt),
		SessionID:   sessionID,
		Query:       query,
		Summary:     summary,
		RequestedAt: requestedAt,
		Attempt:     attempt,
	}
}

func buildMemoryExtractionEventID(sessionID, query, summary string, requestedAt int64) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(sessionID))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(query))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(summary))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(fmt.Sprintf("%d", requestedAt)))
	return hex.EncodeToString(hasher.Sum(nil))
}

func newTTLSet(ttl time.Duration, maxEntries int) *ttlSet {
	if ttl <= 0 {
		ttl = defaultMemoryExtractDedupTTL
	}
	if maxEntries <= 0 {
		maxEntries = defaultMemoryExtractDedupMaxEntries
	}
	return &ttlSet{
		items:      make(map[string]time.Time),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

func (s *ttlSet) Has(key string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	expireAt, ok := s.items[key]
	if !ok {
		return false
	}
	if now.After(expireAt) {
		delete(s.items, key)
		return false
	}
	return true
}

func (s *ttlSet) Mark(key string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	if len(s.items) >= s.maxEntries {
		for existing := range s.items {
			delete(s.items, existing)
			break
		}
	}
	s.items[key] = now.Add(s.ttl)
}

func (s *ttlSet) pruneExpiredLocked(now time.Time) {
	for key, expireAt := range s.items {
		if now.After(expireAt) {
			delete(s.items, key)
		}
	}
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
		cfg.URL = resolveRabbitMQString(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.exchange"); err == nil {
		cfg.Exchange = resolveRabbitMQString(v.String(), cfg.Exchange)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_routing_key"); err == nil {
		cfg.MemoryExtractRoutingKey = resolveRabbitMQString(v.String(), cfg.MemoryExtractRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_retry_routing_key"); err == nil {
		cfg.MemoryExtractRetryRoutingKey = resolveRabbitMQString(v.String(), cfg.MemoryExtractRetryRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_dlq_routing_key"); err == nil {
		cfg.MemoryExtractDLQRoutingKey = resolveRabbitMQString(v.String(), cfg.MemoryExtractDLQRoutingKey)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_queue"); err == nil {
		cfg.MemoryExtractQueue = resolveRabbitMQString(v.String(), cfg.MemoryExtractQueue)
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_retry_queue"); err == nil {
		cfg.MemoryExtractRetryQueue = resolveRabbitMQString(v.String(), "")
	}
	if v, err := g.Cfg().Get(ctx, "rabbitmq.memory_extract_dlq"); err == nil {
		cfg.MemoryExtractDLQ = resolveRabbitMQString(v.String(), "")
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

func resolveRabbitMQString(raw, fallback string) string {
	if resolved, ok := common.ResolveOptionalEnv(raw); ok {
		return strings.TrimSpace(resolved)
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || common.IsEnvReference(trimmed) {
		return fallback
	}
	return trimmed
}
