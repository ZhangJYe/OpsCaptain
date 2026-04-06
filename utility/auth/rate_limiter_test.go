package auth

import (
	"testing"
	"time"
)

func TestInMemoryRateLimiterAllowsWithinCapacity(t *testing.T) {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     5,
		capacity: 5,
		window:   time.Minute,
	}

	for i := 0; i < 5; i++ {
		if !rl.Allow("user-1") {
			t.Fatalf("expected allow on request %d", i+1)
		}
	}

	if rl.Allow("user-1") {
		t.Fatal("expected rate limit to reject 6th request")
	}
}

func TestInMemoryRateLimiterIsolatesClients(t *testing.T) {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     2,
		capacity: 2,
		window:   time.Minute,
	}

	rl.Allow("user-a")
	rl.Allow("user-a")

	if rl.Allow("user-a") {
		t.Fatal("expected user-a to be rate limited")
	}

	if !rl.Allow("user-b") {
		t.Fatal("expected user-b to still have capacity")
	}
}

func TestInMemoryRateLimiterRefillsOverTime(t *testing.T) {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     60,
		capacity: 1,
		window:   time.Minute,
	}

	if !rl.Allow("user-1") {
		t.Fatal("expected first request to be allowed")
	}

	if rl.Allow("user-1") {
		t.Fatal("expected second request to be rejected immediately")
	}

	time.Sleep(1100 * time.Millisecond)

	if !rl.Allow("user-1") {
		t.Fatal("expected request to be allowed after refill")
	}
}

func TestRemainingTokensReportsCorrectly(t *testing.T) {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     10,
		capacity: 10,
		window:   time.Minute,
	}

	if remaining := rl.RemainingTokens("unknown-user"); remaining != 10 {
		t.Fatalf("expected 10 remaining for unknown user, got %d", remaining)
	}

	rl.Allow("user-1")
	rl.Allow("user-1")
	rl.Allow("user-1")

	remaining := rl.RemainingTokens("user-1")
	if remaining != 7 {
		t.Fatalf("expected 7 remaining, got %d", remaining)
	}
}
