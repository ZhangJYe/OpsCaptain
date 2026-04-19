package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestBuildMemoryExtractionEventIDDeterministic(t *testing.T) {
	id1 := buildMemoryExtractionEventID("s", "q", "a", 12345)
	id2 := buildMemoryExtractionEventID("s", "q", "a", 12345)
	id3 := buildMemoryExtractionEventID("s", "q", "a", 12346)

	if id1 == "" {
		t.Fatal("expected non-empty event id")
	}
	if id1 != id2 {
		t.Fatalf("expected same input to produce same id, got %q and %q", id1, id2)
	}
	if id1 == id3 {
		t.Fatalf("expected different requested_at to produce different id, got %q", id1)
	}
}

func TestTTLSetMarkAndHas(t *testing.T) {
	set := newTTLSet(40*time.Millisecond, 2)
	if set.Has("k1") {
		t.Fatal("expected key to be absent before mark")
	}
	set.Mark("k1")
	if !set.Has("k1") {
		t.Fatal("expected key to be present after mark")
	}
	time.Sleep(60 * time.Millisecond)
	if set.Has("k1") {
		t.Fatal("expected key to expire")
	}
}

func TestDecodeMemoryExtractionEventValidation(t *testing.T) {
	event, err := decodeMemoryExtractionEvent([]byte(`{"event_id":"x","session_id":"s","query":"q","summary":"a","requested_at":1,"attempt":0}`))
	if err != nil {
		t.Fatalf("decode should succeed: %v", err)
	}
	if event.SessionID != "s" {
		t.Fatalf("unexpected session id: %s", event.SessionID)
	}

	if _, err := decodeMemoryExtractionEvent([]byte(`{"session_id":"","query":"q","summary":"a"}`)); err == nil {
		t.Fatal("expected validation error when session_id is empty")
	}
}

func TestHandleDeliveryConsumeFailureAckWhenRetryPublished(t *testing.T) {
	oldConsume := consumeMemoryEvent
	oldPublishEvent := publishMemoryEvent
	oldPublishRaw := publishMemoryRaw
	oldAck := ackMemoryDelivery
	oldNack := nackMemoryDelivery
	defer func() {
		consumeMemoryEvent = oldConsume
		publishMemoryEvent = oldPublishEvent
		publishMemoryRaw = oldPublishRaw
		ackMemoryDelivery = oldAck
		nackMemoryDelivery = oldNack
	}()

	var ackCount int
	var nackCount int
	ackMemoryDelivery = func(amqp.Delivery) error {
		ackCount++
		return nil
	}
	nackMemoryDelivery = func(amqp.Delivery, bool) error {
		nackCount++
		return nil
	}
	consumeMemoryEvent = func(*rabbitMQMemoryClient, memoryExtractionEvent) error {
		return errors.New("extract failed")
	}
	publishMemoryEvent = func(_ *rabbitMQMemoryClient, _ context.Context, _ memoryExtractionEvent, routingKey string) error {
		if routingKey == "retry" {
			return nil
		}
		return errors.New("unexpected routing key")
	}
	publishMemoryRaw = func(_ *rabbitMQMemoryClient, _ context.Context, _ []byte, _ string, _ amqp.Table) error {
		return nil
	}

	client := &rabbitMQMemoryClient{
		cfg: rabbitMQMemoryConfig{
			MemoryExtractRetryRoutingKey: "retry",
			MemoryExtractDLQRoutingKey:   "dlq",
			MemoryExtractMaxRetries:      3,
		},
		completed: newTTLSet(time.Minute, 100),
	}
	client.handleDelivery(amqp.Delivery{Body: []byte(`{"session_id":"s","query":"q","summary":"a","attempt":0}`)})

	if ackCount != 1 {
		t.Fatalf("expected ack once, got %d", ackCount)
	}
	if nackCount != 0 {
		t.Fatalf("expected no nack, got %d", nackCount)
	}
}

func TestHandleDeliveryConsumeFailureNackWhenRetryAndDLQFailed(t *testing.T) {
	oldConsume := consumeMemoryEvent
	oldPublishEvent := publishMemoryEvent
	oldPublishRaw := publishMemoryRaw
	oldAck := ackMemoryDelivery
	oldNack := nackMemoryDelivery
	defer func() {
		consumeMemoryEvent = oldConsume
		publishMemoryEvent = oldPublishEvent
		publishMemoryRaw = oldPublishRaw
		ackMemoryDelivery = oldAck
		nackMemoryDelivery = oldNack
	}()

	var ackCount int
	var nackCount int
	ackMemoryDelivery = func(amqp.Delivery) error {
		ackCount++
		return nil
	}
	nackMemoryDelivery = func(amqp.Delivery, bool) error {
		nackCount++
		return nil
	}
	consumeMemoryEvent = func(*rabbitMQMemoryClient, memoryExtractionEvent) error {
		return errors.New("extract failed")
	}
	publishMemoryEvent = func(_ *rabbitMQMemoryClient, _ context.Context, _ memoryExtractionEvent, _ string) error {
		return errors.New("publish failed")
	}
	publishMemoryRaw = func(_ *rabbitMQMemoryClient, _ context.Context, _ []byte, _ string, _ amqp.Table) error {
		return nil
	}

	client := &rabbitMQMemoryClient{
		cfg: rabbitMQMemoryConfig{
			MemoryExtractRetryRoutingKey: "retry",
			MemoryExtractDLQRoutingKey:   "dlq",
			MemoryExtractMaxRetries:      3,
		},
		completed: newTTLSet(time.Minute, 100),
	}
	client.handleDelivery(amqp.Delivery{Body: []byte(`{"session_id":"s","query":"q","summary":"a","attempt":0}`)})

	if ackCount != 0 {
		t.Fatalf("expected no ack, got %d", ackCount)
	}
	if nackCount != 1 {
		t.Fatalf("expected nack once, got %d", nackCount)
	}
}

func TestHandleDeliveryDecodeFailureNackWhenDLQFailed(t *testing.T) {
	oldConsume := consumeMemoryEvent
	oldPublishEvent := publishMemoryEvent
	oldPublishRaw := publishMemoryRaw
	oldAck := ackMemoryDelivery
	oldNack := nackMemoryDelivery
	defer func() {
		consumeMemoryEvent = oldConsume
		publishMemoryEvent = oldPublishEvent
		publishMemoryRaw = oldPublishRaw
		ackMemoryDelivery = oldAck
		nackMemoryDelivery = oldNack
	}()

	var ackCount int
	var nackCount int
	ackMemoryDelivery = func(amqp.Delivery) error {
		ackCount++
		return nil
	}
	nackMemoryDelivery = func(amqp.Delivery, bool) error {
		nackCount++
		return nil
	}
	publishMemoryRaw = func(_ *rabbitMQMemoryClient, _ context.Context, _ []byte, _ string, _ amqp.Table) error {
		return errors.New("dlq publish failed")
	}
	publishMemoryEvent = func(_ *rabbitMQMemoryClient, _ context.Context, _ memoryExtractionEvent, _ string) error {
		return nil
	}
	consumeMemoryEvent = func(*rabbitMQMemoryClient, memoryExtractionEvent) error {
		return nil
	}

	client := &rabbitMQMemoryClient{
		cfg: rabbitMQMemoryConfig{
			MemoryExtractRetryRoutingKey: "retry",
			MemoryExtractDLQRoutingKey:   "dlq",
			MemoryExtractMaxRetries:      3,
		},
		completed: newTTLSet(time.Minute, 100),
	}
	client.handleDelivery(amqp.Delivery{Body: []byte("bad-json")})

	if ackCount != 0 {
		t.Fatalf("expected no ack, got %d", ackCount)
	}
	if nackCount != 1 {
		t.Fatalf("expected nack once, got %d", nackCount)
	}
}

func TestStartMemoryQueueInitLoopRetriesUntilConnected(t *testing.T) {
	oldFactory := newMemoryQueueClient
	defer func() {
		newMemoryQueueClient = oldFactory
		_ = stopMemoryQueueInitLoop(context.Background())
		closeAndSwapMemoryQueueClient(nil)
	}()
	_ = stopMemoryQueueInitLoop(context.Background())
	closeAndSwapMemoryQueueClient(nil)

	var attempts int32
	newMemoryQueueClient = func(cfg rabbitMQMemoryConfig) (*rabbitMQMemoryClient, error) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			return nil, errors.New("rabbitmq unavailable")
		}
		return &rabbitMQMemoryClient{
			cfg:       cfg,
			completed: newTTLSet(time.Minute, 32),
		}, nil
	}

	startMemoryQueueInitLoop(rabbitMQMemoryConfig{
		Enabled:                      true,
		MemoryExtractConsumerEnabled: false,
		MemoryExtractReconnectDelay:  5 * time.Millisecond,
		MemoryExtractDedupTTL:        time.Minute,
		MemoryExtractDedupMaxEntries: 32,
	})

	deadline := time.After(500 * time.Millisecond)
	for getMemoryQueueClient() == nil {
		select {
		case <-deadline:
			t.Fatalf("expected memory queue client to be initialized, attempts=%d", atomic.LoadInt32(&attempts))
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if got := atomic.LoadInt32(&attempts); got < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", got)
	}
}

func TestStopMemoryQueueInitLoopStopsRetry(t *testing.T) {
	oldFactory := newMemoryQueueClient
	defer func() {
		newMemoryQueueClient = oldFactory
		_ = stopMemoryQueueInitLoop(context.Background())
		closeAndSwapMemoryQueueClient(nil)
	}()
	_ = stopMemoryQueueInitLoop(context.Background())
	closeAndSwapMemoryQueueClient(nil)

	var attempts int32
	newMemoryQueueClient = func(rabbitMQMemoryConfig) (*rabbitMQMemoryClient, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("always fail")
	}

	startMemoryQueueInitLoop(rabbitMQMemoryConfig{
		Enabled:                      true,
		MemoryExtractConsumerEnabled: false,
		MemoryExtractReconnectDelay:  10 * time.Millisecond,
	})

	time.Sleep(30 * time.Millisecond)
	if err := stopMemoryQueueInitLoop(context.Background()); err != nil {
		t.Fatalf("expected stop loop success, got %v", err)
	}
	before := atomic.LoadInt32(&attempts)
	time.Sleep(30 * time.Millisecond)
	after := atomic.LoadInt32(&attempts)
	if after != before {
		t.Fatalf("expected no more attempts after stop, before=%d after=%d", before, after)
	}
}
