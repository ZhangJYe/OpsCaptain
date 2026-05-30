package rabbitmq

import (
	"context"
	"testing"
	"time"
)

func TestTTLSetMarkAndHas(t *testing.T) {
	s := NewTTLSet(time.Minute, 100)
	if s.Has("key") {
		t.Fatal("expected Has=false for missing key")
	}
	s.Mark("key")
	if !s.Has("key") {
		t.Fatal("expected Has=true after Mark")
	}
}

func TestTTLSetExpiry(t *testing.T) {
	s := NewTTLSet(50*time.Millisecond, 100)
	s.Mark("key")
	if !s.Has("key") {
		t.Fatal("expected Has=true immediately after Mark")
	}
	time.Sleep(60 * time.Millisecond)
	if s.Has("key") {
		t.Fatal("expected Has=false after TTL expiry")
	}
}

func TestTTLSetMaxEntries(t *testing.T) {
	s := NewTTLSet(time.Minute, 3)
	s.Mark("a")
	s.Mark("b")
	s.Mark("c")
	if !s.Has("a") || !s.Has("b") || !s.Has("c") {
		t.Fatal("expected all 3 entries present")
	}
	// Adding a 4th should evict one existing entry
	s.Mark("d")
	if !s.Has("d") {
		t.Fatal("expected newly marked key 'd' to be present")
	}
	count := 0
	for _, k := range []string{"a", "b", "c", "d"} {
		if s.Has(k) {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("expected exactly 3 entries after overflow, got %d", count)
	}
}

func TestTTLSetEmptyKeyIgnored(t *testing.T) {
	s := NewTTLSet(time.Minute, 100)
	s.Mark("")
	s.Mark("   ")
	if s.Has("") {
		t.Fatal("empty key should not be stored")
	}
}

func TestResolveRabbitMQString(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		fallback string
		want     string
	}{
		{"empty raw uses fallback", "", "amqp://localhost", "amqp://localhost"},
		{"whitespace raw uses fallback", "  ", "amqp://localhost", "amqp://localhost"},
		{"plain value returned", "amqp://host:5672", "fallback", "amqp://host:5672"},
		{"plain value trimmed", "  amqp://host:5672  ", "fallback", "amqp://host:5672"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveRabbitMQString(tc.raw, tc.fallback)
			if got != tc.want {
				t.Errorf("ResolveRabbitMQString(%q, %q) = %q, want %q", tc.raw, tc.fallback, got, tc.want)
			}
		})
	}
}

func TestSleepReconnectRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if SleepReconnect(ctx, time.Hour) {
		t.Fatal("expected false when context is already cancelled")
	}
}

func TestSleepReconnectReturnsTrueOnDelay(t *testing.T) {
	ctx := context.Background()
	if !SleepReconnect(ctx, time.Millisecond) {
		t.Fatal("expected true after delay elapses")
	}
}
