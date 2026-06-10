package clusterbus

import (
	"context"
	"testing"
	"time"
)

// 端到端 publish/subscribe 测试依赖 Redis；放到 integration 标签。
// 这里只验证参数 / nil 安全。

func TestNew_DefaultPrefix(t *testing.T) {
	bus := New("")
	if bus.prefix != "opscaption" {
		t.Fatalf("expected default prefix opscaption, got %s", bus.prefix)
	}
}

func TestPublish_Validate(t *testing.T) {
	var bus *Bus
	if err := bus.Publish(context.Background(), "ch", []byte("x")); err == nil {
		t.Fatal("expected error on nil bus")
	}
	bus = &Bus{}
	if err := bus.Publish(context.Background(), "", []byte("x")); err == nil {
		t.Fatal("expected error on empty channel")
	}
}

func TestSubscribe_Validate(t *testing.T) {
	var bus *Bus
	if _, err := bus.Subscribe(context.Background(), "ch", func(context.Context, []byte) {}); err == nil {
		t.Fatal("expected error on nil bus")
	}
	bus = &Bus{}
	if _, err := bus.Subscribe(context.Background(), "", func(context.Context, []byte) {}); err == nil {
		t.Fatal("expected error on empty channel")
	}
	if _, err := bus.Subscribe(context.Background(), "ch", nil); err == nil {
		t.Fatal("expected error on nil handler")
	}
}

func TestSubscribe_ClosedBus(t *testing.T) {
	bus := &Bus{closed: true}
	if _, err := bus.Subscribe(context.Background(), "ch", func(context.Context, []byte) {}); err == nil {
		t.Fatal("expected error on closed bus")
	}
}

func TestClose_NilSafe(t *testing.T) {
	var bus *Bus
	bus.Close() // 不应 panic
}

func TestClose_Idempotent(t *testing.T) {
	bus := &Bus{}
	bus.Close()
	bus.Close() // 再次调用不应 panic
}

func TestWaitOrCancel_RespectCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !waitOrCancel(ctx, time.Second) {
		t.Fatal("waitOrCancel should return true when ctx cancelled")
	}
}

func TestSafeInvoke_RecoverPanic(t *testing.T) {
	// handler panic 不应让测试崩溃
	safeInvoke(context.Background(), func(context.Context, []byte) {
		panic("boom")
	}, []byte("x"))
}
