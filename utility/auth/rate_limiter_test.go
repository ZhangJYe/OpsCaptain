package auth

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_BasicAllow(t *testing.T) {
	globalLimiterOnce = sync.Once{}
	globalLimiter = nil

	limiter := GetRateLimiter()

	for i := 0; i < 25; i++ {
		if !limiter.Allow("test-client") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_ExceedLimit(t *testing.T) {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     5,
		capacity: 5,
		window:   time.Minute,
	}

	for i := 0; i < 5; i++ {
		if !rl.Allow("burst-client") {
			t.Fatalf("request %d should be allowed within capacity", i+1)
		}
	}

	if rl.Allow("burst-client") {
		t.Fatal("request should be rejected after exceeding capacity")
	}
}

func TestRateLimiter_DifferentClients(t *testing.T) {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     5,
		capacity: 5,
		window:   time.Minute,
	}

	for i := 0; i < 5; i++ {
		rl.Allow("client-A")
	}

	if !rl.Allow("client-B") {
		t.Fatal("client-B should not be affected by client-A's rate limit")
	}
}

func TestRateLimiter_RemainingTokens(t *testing.T) {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     10,
		capacity: 10,
		window:   time.Minute,
	}

	remaining := rl.RemainingTokens("new-client")
	if remaining != 10 {
		t.Fatalf("expected 10 remaining for new client, got %d", remaining)
	}

	rl.Allow("new-client")
	remaining = rl.RemainingTokens("new-client")
	if remaining != 9 {
		t.Fatalf("expected 9 remaining after 1 request, got %d", remaining)
	}
}

func TestCheckRateLimit(t *testing.T) {
	globalLimiterOnce = sync.Once{}
	globalLimiter = nil

	err := CheckRateLimit("check-test")
	if err != nil {
		t.Fatalf("first request should be allowed: %v", err)
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     100,
		capacity: 100,
		window:   time.Minute,
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow("concurrent-client")
		}()
	}
	wg.Wait()
}
