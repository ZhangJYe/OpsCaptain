package rabbitmq

import (
	"fmt"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const defaultConnectionTimeout = 10 * time.Second

// TopologyConfig describes the AMQP topology for a single queue with retry and DLQ.
type TopologyConfig struct {
	URL               string
	Exchange          string
	Queue             string
	RoutingKey        string
	RetryQueue        string
	RetryRoutingKey   string
	DLQ               string
	DLQRoutingKey     string
	RetryDelay        time.Duration
	Prefetch          int
	ConsumerEnabled   bool
	ConnectionTimeout time.Duration
}

// DeclareTopology declares exchange, main queue, retry queue, and DLQ on ch.
func DeclareTopology(ch *amqp.Channel, topo TopologyConfig) error {
	if err := ch.ExchangeDeclare(topo.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange failed: %w", err)
	}

	if _, err := ch.QueueDeclare(topo.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue failed: %w", err)
	}
	if err := ch.QueueBind(topo.Queue, topo.RoutingKey, topo.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue failed: %w", err)
	}

	retryArgs := amqp.Table{
		"x-dead-letter-exchange":    topo.Exchange,
		"x-dead-letter-routing-key": topo.RoutingKey,
		"x-message-ttl":             int32(topo.RetryDelay.Milliseconds()),
	}
	if _, err := ch.QueueDeclare(topo.RetryQueue, true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("declare retry queue failed: %w", err)
	}
	if err := ch.QueueBind(topo.RetryQueue, topo.RetryRoutingKey, topo.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind retry queue failed: %w", err)
	}

	if _, err := ch.QueueDeclare(topo.DLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dlq failed: %w", err)
	}
	if err := ch.QueueBind(topo.DLQ, topo.DLQRoutingKey, topo.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind dlq failed: %w", err)
	}

	return nil
}

// OpenChannels opens an AMQP connection, declares topology, and returns
// the connection, publish channel, and (if ConsumerEnabled) consume channel.
func OpenChannels(topo TopologyConfig) (*amqp.Connection, *amqp.Channel, *amqp.Channel, error) {
	if strings.TrimSpace(topo.URL) == "" {
		return nil, nil, nil, fmt.Errorf("rabbitmq url is empty")
	}

	connTimeout := topo.ConnectionTimeout
	if connTimeout <= 0 {
		connTimeout = defaultConnectionTimeout
	}

	conn, err := amqp.DialConfig(topo.URL, amqp.Config{
		Dial: amqp.DefaultDial(connTimeout),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect rabbitmq failed: %w", err)
	}

	publishCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, nil, fmt.Errorf("open publish channel failed: %w", err)
	}

	if err := DeclareTopology(publishCh, topo); err != nil {
		_ = publishCh.Close()
		_ = conn.Close()
		return nil, nil, nil, err
	}

	var consumeCh *amqp.Channel
	if topo.ConsumerEnabled {
		consumeCh, err = conn.Channel()
		if err != nil {
			_ = publishCh.Close()
			_ = conn.Close()
			return nil, nil, nil, fmt.Errorf("open consume channel failed: %w", err)
		}
		if err := consumeCh.Qos(topo.Prefetch, 0, false); err != nil {
			_ = consumeCh.Close()
			_ = publishCh.Close()
			_ = conn.Close()
			return nil, nil, nil, fmt.Errorf("set consume qos failed: %w", err)
		}
	}

	return conn, publishCh, consumeCh, nil
}

// CloseChannel safely closes an AMQP channel, ignoring already-closed errors.
func CloseChannel(ch *amqp.Channel) error {
	if ch == nil {
		return nil
	}
	if err := ch.Close(); err != nil && !IsAlreadyClosed(err) {
		return err
	}
	return nil
}

// CloseConnection safely closes an AMQP connection, ignoring already-closed errors.
func CloseConnection(conn *amqp.Connection) error {
	if conn == nil {
		return nil
	}
	if err := conn.Close(); err != nil && !IsAlreadyClosed(err) {
		return err
	}
	return nil
}

// IsAlreadyClosed returns true if err is amqp.ErrClosed.
func IsAlreadyClosed(err error) bool {
	return err == amqp.ErrClosed
}
