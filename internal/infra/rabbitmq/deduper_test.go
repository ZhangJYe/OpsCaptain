package rabbitmq

import (
	"context"
	"testing"
	"time"
)

func TestTTLSet_Claim(t *testing.T) {
	s := NewTTLSet(time.Minute, 100)
	if !s.Claim(context.Background(), "k1") {
		t.Fatal("first claim should succeed")
	}
	if s.Claim(context.Background(), "k1") {
		t.Fatal("second claim of same key should fail")
	}
	if !s.Claim(context.Background(), "k2") {
		t.Fatal("different key should succeed")
	}
}

func TestTTLSet_Claim_AfterExpiry(t *testing.T) {
	s := NewTTLSet(50*time.Millisecond, 100)
	if !s.Claim(context.Background(), "k1") {
		t.Fatal("first claim should succeed")
	}
	time.Sleep(70 * time.Millisecond)
	if !s.Claim(context.Background(), "k1") {
		t.Fatal("after expiry, claim should succeed again")
	}
}

func TestTTLSet_Claim_Eviction(t *testing.T) {
	s := NewTTLSet(time.Minute, 2)
	s.Claim(context.Background(), "a")
	s.Claim(context.Background(), "b")
	s.Claim(context.Background(), "c")
	// 应当移除最早的 "a"
	if !s.Claim(context.Background(), "a") {
		t.Fatal("after eviction 'a' should be claimable again")
	}
}

func TestRedisDeduper_NilSafe(t *testing.T) {
	var d *RedisDeduper
	if !d.Claim(context.Background(), "k1") {
		t.Fatal("nil RedisDeduper should default to allow")
	}
}

func TestRedisDeduper_EmptyKey(t *testing.T) {
	d := &RedisDeduper{}
	if !d.Claim(context.Background(), "") {
		t.Fatal("empty key should be allowed")
	}
}

func TestCompositeDeduper_AllAllow(t *testing.T) {
	d := NewCompositeDeduper(NewTTLSet(time.Minute, 10), NewTTLSet(time.Minute, 10))
	if !d.Claim(context.Background(), "k") {
		t.Fatal("first composite claim should succeed")
	}
	if d.Claim(context.Background(), "k") {
		t.Fatal("second composite claim should fail (first part rejects)")
	}
}

func TestCompositeDeduper_NilParts(t *testing.T) {
	d := NewCompositeDeduper(nil, NewTTLSet(time.Minute, 10), nil)
	if !d.Claim(context.Background(), "k") {
		t.Fatal("composite with nil parts should still work")
	}
}

func TestDescribeDeduper(t *testing.T) {
	if got := DescribeDeduper(nil); got != "<nil>" {
		t.Fatalf("expected <nil>, got %s", got)
	}
	if got := DescribeDeduper(NewTTLSet(time.Minute, 10)); got == "" {
		t.Fatal("expected non-empty description for TTLSet")
	}
	if got := DescribeDeduper(NewCompositeDeduper(NewTTLSet(time.Minute, 10))); got == "" {
		t.Fatal("expected non-empty description for Composite")
	}
}
