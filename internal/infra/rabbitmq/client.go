package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Client manages an AMQP connection with automatic reconnection,
// topology declaration, and publish/consume operations.
type Client struct {
	cfg       TopologyConfig
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

	logPrefix string

	// OnReconnectFailed is called when a reconnect attempt fails.
	// Useful for logging/metrics from the consumer loop where the caller
	// cannot observe the error directly.
	OnReconnectFailed func(err error)
}

// NewClient creates a new Client. Call Connect to establish the initial connection.
func NewClient(topo TopologyConfig, logPrefix string) *Client {
	return &Client{
		cfg:       topo,
		logPrefix: logPrefix,
	}
}

// Connect establishes the initial AMQP connection.
func (c *Client) Connect() error {
	conn, publishCh, consumeCh, err := OpenChannels(c.cfg)
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

// Publish sends a message to the exchange with the given routing key.
// If the first attempt fails (and the error is not a context cancellation),
// it reconnects and retries once.
func (c *Client) Publish(ctx context.Context, body []byte, routingKey string, headers amqp.Table) error {
	if c.isClosed() {
		return fmt.Errorf("%s rabbitmq client closed", c.logPrefix)
	}

	c.publishMu.Lock()
	defer c.publishMu.Unlock()

	if c.isClosed() {
		return fmt.Errorf("%s rabbitmq client closed", c.logPrefix)
	}

	if err := c.publishWithCurrentChannel(ctx, body, routingKey, headers); err == nil {
		return nil
	} else {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if reconnectErr := c.reconnect(ctx); reconnectErr != nil {
			return fmt.Errorf("publish failed: %v; reconnect failed: %w", err, reconnectErr)
		}
		if retryErr := c.publishWithCurrentChannel(ctx, body, routingKey, headers); retryErr != nil {
			return fmt.Errorf("publish failed after reconnect: %w", retryErr)
		}
	}
	return nil
}

// StartConsumer starts the consume loop in a goroutine. handler is called for
// each delivery. reconnectDelay controls the backoff between reconnection attempts.
// The handler is responsible for ack/nack.
func (c *Client) StartConsumer(ctx context.Context, handler func(amqp.Delivery), reconnectDelay time.Duration) error {
	if !c.cfg.ConsumerEnabled {
		return nil
	}

	c.stateMu.Lock()
	if c.consumeDone != nil {
		c.stateMu.Unlock()
		return nil
	}
	c.consumeCtx, c.consumeCancel = context.WithCancel(ctx)
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

			deliveries, err := c.openConsumerDeliveries(consumeCtx)
			if err != nil {
				if c.OnReconnectFailed != nil {
					c.OnReconnectFailed(err)
				}
				if !SleepReconnect(consumeCtx, reconnectDelay) {
					return
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
						if !SleepReconnect(consumeCtx, reconnectDelay) {
							return
						}
						if reconErr := c.reconnect(consumeCtx); reconErr != nil && c.OnReconnectFailed != nil {
							c.OnReconnectFailed(reconErr)
						}
						break consumeLoop
					}
					handler(delivery)
				}
			}
		}
	}()
	return nil
}

// Dedup returns nil — callers should manage their own dedup sets.
// This method exists only for interface compatibility during migration.
func (c *Client) Dedup() *TTLSet {
	return nil
}

// Close gracefully shuts down the client.
func (c *Client) Close() error {
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
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		select {
		case <-consumeDone:
		case <-waitCtx.Done():
			errs = append(errs, "consumer goroutine did not stop in time")
		}
	}

	c.reconnectMu.Lock()
	oldConn, oldPublishCh, oldConsumeCh := c.swapAMQPState(nil, nil, nil)
	c.reconnectMu.Unlock()

	if err := CloseChannel(oldConsumeCh); err != nil {
		errs = append(errs, err.Error())
	}
	if err := CloseChannel(oldPublishCh); err != nil {
		errs = append(errs, err.Error())
	}
	if err := CloseConnection(oldConn); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (c *Client) reconnect(ctx context.Context) error {
	if c.isClosed() {
		return fmt.Errorf("%s rabbitmq client closed", c.logPrefix)
	}

	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	if c.isClosed() {
		return fmt.Errorf("%s rabbitmq client closed", c.logPrefix)
	}

	topo := c.cfg
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ctx.Err()
		}
		if topo.ConnectionTimeout <= 0 || topo.ConnectionTimeout > remaining {
			topo.ConnectionTimeout = remaining
		}
	}

	conn, publishCh, consumeCh, err := OpenChannels(topo)
	if err != nil {
		return err
	}
	oldConn, oldPublishCh, oldConsumeCh := c.swapAMQPState(conn, publishCh, consumeCh)
	_ = CloseChannel(oldConsumeCh)
	_ = CloseChannel(oldPublishCh)
	_ = CloseConnection(oldConn)
	return nil
}

func (c *Client) publishWithCurrentChannel(ctx context.Context, body []byte, routingKey string, headers amqp.Table) error {
	publishCh := c.getPublishChannel()
	if publishCh == nil {
		return fmt.Errorf("%s publish channel unavailable", c.logPrefix)
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

func (c *Client) openConsumerDeliveries(ctx context.Context) (<-chan amqp.Delivery, error) {
	consumeCh := c.getConsumeChannel()
	if consumeCh == nil {
		if err := c.reconnect(ctx); err != nil {
			return nil, err
		}
		consumeCh = c.getConsumeChannel()
		if consumeCh == nil {
			return nil, fmt.Errorf("%s consume channel unavailable", c.logPrefix)
		}
	}

	deliveries, err := consumeCh.Consume(c.cfg.Queue, "", false, false, false, false, nil)
	if err == nil {
		return deliveries, nil
	}
	if reconnectErr := c.reconnect(ctx); reconnectErr != nil {
		return nil, fmt.Errorf("consume failed: %v; reconnect failed: %w", err, reconnectErr)
	}
	consumeCh = c.getConsumeChannel()
	if consumeCh == nil {
		return nil, fmt.Errorf("consume failed: %w; channel nil after reconnect", err)
	}
	return consumeCh.Consume(c.cfg.Queue, "", false, false, false, false, nil)
}

func (c *Client) swapAMQPState(
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

func (c *Client) getPublishChannel() *amqp.Channel {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.publishCh
}

func (c *Client) getConsumeChannel() *amqp.Channel {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.consumeCh
}

func (c *Client) isClosed() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.closed
}
