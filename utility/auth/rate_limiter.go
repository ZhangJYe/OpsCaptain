package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type RateLimiter struct {
	buckets  map[string]*tokenBucket
	mu       sync.Mutex
	rate     int
	capacity int
	window   time.Duration
}

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

var (
	globalLimiter     *RateLimiter
	globalLimiterOnce sync.Once
	redisLimiter      *RedisRateLimiter
	useRedis          bool
	limiterInitOnce   sync.Once
)

func initLimiterBackend() {
	limiterInitOnce.Do(func() {
		ctx := context.Background()
		if IsRedisAvailable(ctx) {
			rate := 20
			v, err := g.Cfg().Get(ctx, "auth.rate_limit_per_minute")
			if err == nil && v.Int() > 0 {
				rate = v.Int()
			}
			redisLimiter = NewRedisRateLimiter(rate, time.Minute)
			useRedis = true
			g.Log().Info(ctx, "rate limiter: using Redis sliding window")
		} else {
			useRedis = false
			g.Log().Info(ctx, "rate limiter: using in-memory token bucket")
		}
	})
}

func GetRateLimiter() *RateLimiter {
	globalLimiterOnce.Do(func() {
		rate := 20
		capacity := 30

		v, err := g.Cfg().Get(context.Background(), "auth.rate_limit_per_minute")
		if err == nil && v.Int() > 0 {
			rate = v.Int()
		}
		v, err = g.Cfg().Get(context.Background(), "auth.rate_limit_burst")
		if err == nil && v.Int() > 0 {
			capacity = v.Int()
		}

		globalLimiter = &RateLimiter{
			buckets:  make(map[string]*tokenBucket),
			rate:     rate,
			capacity: capacity,
			window:   time.Minute,
		}

		go globalLimiter.cleanup()
	})
	return globalLimiter
}

func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.buckets[clientID]
	if !exists {
		bucket = &tokenBucket{
			tokens:     float64(rl.capacity),
			lastRefill: time.Now(),
		}
		rl.buckets[clientID] = bucket
	}

	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	refill := elapsed * float64(rl.rate) / rl.window.Seconds()
	bucket.tokens += refill
	if bucket.tokens > float64(rl.capacity) {
		bucket.tokens = float64(rl.capacity)
	}
	bucket.lastRefill = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true
	}
	return false
}

func (rl *RateLimiter) RemainingTokens(clientID string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.buckets[clientID]
	if !exists {
		return rl.capacity
	}
	return int(bucket.tokens)
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for id, bucket := range rl.buckets {
			if now.Sub(bucket.lastRefill) > 10*time.Minute {
				delete(rl.buckets, id)
			}
		}
		rl.mu.Unlock()
	}
}

func CheckRateLimit(clientID string) error {
	initLimiterBackend()

	if useRedis {
		ctx := context.Background()
		allowed, err := redisLimiter.Allow(ctx, clientID)
		if err != nil {
			g.Log().Warningf(ctx, "redis rate limit error, falling back to in-memory: %v", err)
			return checkInMemoryRateLimit(clientID)
		}
		if !allowed {
			return fmt.Errorf("rate limit exceeded, please try again later")
		}
		return nil
	}

	return checkInMemoryRateLimit(clientID)
}

func checkInMemoryRateLimit(clientID string) error {
	limiter := GetRateLimiter()
	if !limiter.Allow(clientID) {
		return fmt.Errorf("rate limit exceeded, please try again later")
	}
	return nil
}

func RemainingTokens(clientID string) int {
	initLimiterBackend()

	if useRedis {
		remaining, err := redisLimiter.Remaining(context.Background(), clientID)
		if err == nil {
			return remaining
		}
	}

	return GetRateLimiter().RemainingTokens(clientID)
}
