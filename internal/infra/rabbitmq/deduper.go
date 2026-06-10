package rabbitmq

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
)

// Deduper 是消费幂等去重接口。
// 同一进程内（TTLSet）和跨实例（RedisDeduper）共享语义：
//
//	Claim 原子返回 true 表示「这个 key 此前未见过、现在已记下」；
//	返回 false 表示别人（可能是其他实例）已 claim 过，调用方应跳过处理。
type Deduper interface {
	// Claim 原子检查+标记。成功 claim 返回 true。
	Claim(ctx context.Context, key string) bool
}

// claim 兼容旧 TTLSet 调用方的 Has+Mark 风格。
// 调用方应优先使用 Deduper.Claim 以避免 TOCTOU。
func (s *TTLSet) Claim(_ context.Context, key string) bool {
	if strings.TrimSpace(key) == "" {
		return true
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	if expireAt, ok := s.items[key]; ok && now.Before(expireAt) {
		return false
	}
	if s.maxEntries > 0 && len(s.items) >= s.maxEntries {
		s.evictOldestLocked()
	}
	s.items[key] = now.Add(s.ttl)
	return true
}

// RedisDeduper 用 Redis SET NX EX 做跨实例幂等去重。
// 适合多实例 RabbitMQ 消费者：每条 delivery 在处理前先 Claim，
// 保证同一个 message_id / task_id 全集群只处理一次。
type RedisDeduper struct {
	redis  *gredis.Redis
	prefix string
	ttl    time.Duration
}

// NewRedisDeduper 创建跨实例去重器。prefix 用于隔离不同业务（如 "dedup:chat_task:"），
// ttl 应 >= 业务最长处理时间，防止处理中 lease 到期后被另一实例 reclaim。
func NewRedisDeduper(redis *gredis.Redis, prefix string, ttl time.Duration) *RedisDeduper {
	if prefix == "" {
		prefix = "opscaption:dedup:"
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &RedisDeduper{redis: redis, prefix: prefix, ttl: ttl}
}

// Claim 在 Redis 上原子地占位。失败（已有人占用 / Redis 错误）返回 false。
// Redis 错误时返回 false 是「故障安全」选择：宁可漏一条也不让重复消费。
func (d *RedisDeduper) Claim(ctx context.Context, key string) bool {
	if d == nil || d.redis == nil {
		// 没 Redis 则默认放行，调用方应用 in-memory 兜底
		return true
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return true
	}
	fullKey := d.prefix + key
	ttlSec := int(d.ttl.Seconds())
	if ttlSec <= 0 {
		ttlSec = 1
	}
	val, err := d.redis.Do(ctx, "SET", fullKey, "1", "NX", "EX", ttlSec)
	if err != nil {
		return false
	}
	return !(val == nil || val.IsNil())
}

// CompositeDeduper 组合多个 Deduper：所有都 Claim 通过才算成功。
// 用法：第一个是 Redis（跨实例），第二个是 TTLSet（同实例快速路径）。
// 调用方应把更便宜的放前面以最大化短路收益（但本实现保持参数顺序）。
type CompositeDeduper struct {
	parts []Deduper
}

func NewCompositeDeduper(parts ...Deduper) *CompositeDeduper {
	return &CompositeDeduper{parts: parts}
}

func (c *CompositeDeduper) Claim(ctx context.Context, key string) bool {
	if c == nil || len(c.parts) == 0 {
		return true
	}
	for _, p := range c.parts {
		if p == nil {
			continue
		}
		if !p.Claim(ctx, key) {
			return false
		}
	}
	return true
}

// evictOldestLocked 取最早过期的项移除，避免 map 无界增长。
// 调用方需持有 s.mu。
func (s *TTLSet) evictOldestLocked() {
	var oldestKey string
	var oldestAt time.Time
	first := true
	for k, t := range s.items {
		if first || t.Before(oldestAt) {
			oldestKey = k
			oldestAt = t
			first = false
		}
	}
	if oldestKey != "" {
		delete(s.items, oldestKey)
	}
}

// describeDeduper 用于日志诊断
func describeDeduper(d Deduper) string {
	if d == nil {
		return "<nil>"
	}
	switch v := d.(type) {
	case *TTLSet:
		return fmt.Sprintf("TTLSet(ttl=%s,max=%d)", v.ttl, v.maxEntries)
	case *RedisDeduper:
		return fmt.Sprintf("RedisDeduper(prefix=%s,ttl=%s)", v.prefix, v.ttl)
	case *CompositeDeduper:
		parts := make([]string, 0, len(v.parts))
		for _, p := range v.parts {
			parts = append(parts, describeDeduper(p))
		}
		return "Composite[" + strings.Join(parts, ",") + "]"
	default:
		return fmt.Sprintf("%T", d)
	}
}

// DescribeDeduper 暴露给调用方做启动日志
func DescribeDeduper(d Deduper) string { return describeDeduper(d) }
