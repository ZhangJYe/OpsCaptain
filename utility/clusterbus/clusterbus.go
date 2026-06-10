// Package clusterbus 提供基于 Redis Pub/Sub 的跨实例事件广播。
//
// 用途：单一进程内的 channel/handler fan-out 在多实例下"事件源进程私有"的问题，
// 用 Pub/Sub 拓展为「任一实例 publish → 所有实例 subscriber 收到」。
//
// 设计取舍：
//   - 选 Pub/Sub 而不是 Streams：消息「即时广播、订阅者各自消费」语义；
//     不需要持久化、不需要 consumer group 协调。订阅者宕掉就丢失这段时间事件，
//     符合「实时通知」场景（变更哨兵、配置 hot reload 等）。
//   - 不做幂等 / 不做 ack：调用方自行处理（如 store.Save 已经走 dedup）。
//   - JSON 编码 payload：跨语言友好，体积可接受（变更事件 < 4KB）。
//
// 典型用法：
//
//	bus := clusterbus.New("ce")
//	bus.Subscribe(ctx, "events", func(ctx context.Context, payload []byte) {
//	    // 收到本实例或其他实例 publish 的事件
//	})
//	bus.Publish(ctx, "events", eventBytes)
package clusterbus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

// Bus 是一个 namespace 下的 Pub/Sub 客户端。
// 一个 Bus 对应一组同前缀的 channel；多个业务（变更事件、配置变更等）
// 各持一个 Bus，避免 channel 名冲突。
type Bus struct {
	prefix string
	redis  *gredis.Redis

	mu          sync.Mutex
	subscribers []*subscriber
	closed      bool
}

type subscriber struct {
	channel string
	cancel  context.CancelFunc
}

// MessageHandler 处理收到的消息。ctx 在 Bus.Close 时取消。
// 处理函数应轻量；耗时操作交给独立 goroutine，避免阻塞 Pub/Sub 接收循环。
type MessageHandler func(ctx context.Context, payload []byte)

// New 创建一个 Bus。prefix 用于隔离不同业务的 channel 命名空间。
// prefix 为空时使用 "opscaption"。
// 注意：redis 客户端延迟到 Publish/Subscribe 时通过 g.Redis() 获取，
// 避免单元测试在没有 Redis 配置时启动即 panic。
func New(prefix string) *Bus {
	if prefix == "" {
		prefix = "opscaption"
	}
	return &Bus{prefix: prefix}
}

// redisClient 懒加载 g.Redis()，避免在初始化期间 panic。
func (b *Bus) redisClient() *gredis.Redis {
	if b.redis != nil {
		return b.redis
	}
	defer func() { _ = recover() }()
	b.redis = g.Redis()
	return b.redis
}

// Publish 把 payload 广播到 channel。所有 Subscribe 到该 channel 的实例都会收到。
// 注意：包含本实例自己的订阅（如果有）。调用方如需"只广播给其他实例"，
// 自行在 payload 里加 source instance ID 在 handler 里过滤。
func (b *Bus) Publish(ctx context.Context, channel string, payload []byte) error {
	if b == nil {
		return errors.New("clusterbus: nil bus")
	}
	redis := b.redisClient()
	if redis == nil {
		return errors.New("clusterbus: redis not configured")
	}
	if channel == "" {
		return errors.New("clusterbus: empty channel")
	}
	fullChannel := b.prefix + ":" + channel
	if _, err := redis.Do(ctx, "PUBLISH", fullChannel, string(payload)); err != nil {
		return fmt.Errorf("clusterbus publish: %w", err)
	}
	return nil
}

// Subscribe 注册 handler 到 channel。返回 cancel 函数解除订阅。
// 收到消息会在独立 goroutine 调 handler，handler 之间串行（按 Redis 推送顺序）。
//
// 内部启动一个常驻 goroutine 跑 ReceiveMessage 循环；
// 连接错误会自动重连（指数退避，最大 30s）。
func (b *Bus) Subscribe(ctx context.Context, channel string, handler MessageHandler) (context.CancelFunc, error) {
	if b == nil {
		return nil, errors.New("clusterbus: nil bus")
	}
	if b.redisClient() == nil {
		return nil, errors.New("clusterbus: redis not configured")
	}
	if channel == "" {
		return nil, errors.New("clusterbus: empty channel")
	}
	if handler == nil {
		return nil, errors.New("clusterbus: nil handler")
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errors.New("clusterbus: bus closed")
	}
	subCtx, cancel := context.WithCancel(ctx)
	sub := &subscriber{channel: channel, cancel: cancel}
	b.subscribers = append(b.subscribers, sub)
	b.mu.Unlock()

	fullChannel := b.prefix + ":" + channel
	go b.runSubscribeLoop(subCtx, fullChannel, handler)

	return cancel, nil
}

// runSubscribeLoop 在断开 / 错误时自动重连，直到 ctx.Done。
func (b *Bus) runSubscribeLoop(ctx context.Context, fullChannel string, handler MessageHandler) {
	backoff := 100 * time.Millisecond
	const maxBackoff = 30 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		conn, _, err := b.redis.GroupPubSub().Subscribe(ctx, fullChannel)
		if err != nil {
			g.Log().Warningf(ctx, "[clusterbus] subscribe %s failed: %v (retry in %s)", fullChannel, err, backoff)
			if waitOrCancel(ctx, backoff) {
				return
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = 100 * time.Millisecond
		b.consume(ctx, conn, fullChannel, handler)
		_ = conn.Close(context.Background())
	}
}

func (b *Bus) consume(ctx context.Context, conn gredis.Conn, fullChannel string, handler MessageHandler) {
	for {
		if ctx.Err() != nil {
			return
		}
		msg, err := conn.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			g.Log().Warningf(ctx, "[clusterbus] receive on %s failed: %v (reconnecting)", fullChannel, err)
			return
		}
		if msg == nil {
			continue
		}
		// handler 在 ctx 下运行；不开新 goroutine 是为了保证按 Redis 顺序处理。
		// 若 handler 耗时，调用方在内部自行 fan-out 到 goroutine。
		safeInvoke(ctx, handler, []byte(msg.Payload))
	}
}

func safeInvoke(ctx context.Context, handler MessageHandler, payload []byte) {
	defer func() {
		if r := recover(); r != nil {
			g.Log().Errorf(ctx, "[clusterbus] handler panic recovered: %v", r)
		}
	}()
	handler(ctx, payload)
}

func waitOrCancel(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}

// Close 取消所有订阅并阻止后续 Subscribe。Publish 仍可调用（直到 redis 关闭）。
func (b *Bus) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, s := range b.subscribers {
		s.cancel()
	}
	b.subscribers = nil
}
