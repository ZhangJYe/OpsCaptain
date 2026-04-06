package auth

import (
	"context"
	"fmt"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

type RedisRateLimiter struct {
	rate     int
	window   time.Duration
	redisCfg *gredis.Config
}

func NewRedisRateLimiter(rate int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		rate:   rate,
		window: window,
	}
}

var slidingWindowScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

local count = redis.call('ZCARD', key)
if count < limit then
    redis.call('ZADD', key, now, now .. '-' .. math.random(1, 1000000))
    redis.call('PEXPIRE', key, window)
    return 1
end

return 0
`

func (rl *RedisRateLimiter) Allow(ctx context.Context, clientID string) (bool, error) {
	redis := g.Redis()
	key := fmt.Sprintf("ratelimit:%s", clientID)
	nowMs := time.Now().UnixMilli()
	windowMs := rl.window.Milliseconds()

	result, err := redis.Do(ctx, "EVAL", slidingWindowScript, 1, key, rl.rate, windowMs, nowMs)
	if err != nil {
		return false, fmt.Errorf("redis rate limit eval failed: %w", err)
	}

	return result.Int() == 1, nil
}

func (rl *RedisRateLimiter) Remaining(ctx context.Context, clientID string) (int, error) {
	redis := g.Redis()
	key := fmt.Sprintf("ratelimit:%s", clientID)
	nowMs := time.Now().UnixMilli()
	windowMs := rl.window.Milliseconds()

	_, err := redis.Do(ctx, "ZREMRANGEBYSCORE", key, 0, nowMs-windowMs)
	if err != nil {
		return 0, err
	}

	count, err := redis.Do(ctx, "ZCARD", key)
	if err != nil {
		return 0, err
	}

	remaining := rl.rate - count.Int()
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

func IsRedisAvailable(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "redis.default.address")
	if err != nil || v.String() == "" {
		return false
	}
	defer func() { recover() }()
	result, err := g.Redis().Do(ctx, "PING")
	if err != nil {
		g.Log().Debugf(ctx, "redis ping failed, falling back to in-memory rate limiter: %v", err)
		return false
	}
	return result.String() == "PONG"
}
