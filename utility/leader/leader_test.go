package leader

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 这些测试只校验不依赖 Redis 的代码路径（参数校验 / nil 处理）。
// 完整端到端测试在 docker-compose 起 Redis 后跑（test-integration 标签）。

func TestAcquire_ZeroTTL(t *testing.T) {
	_, err := Acquire(context.Background(), "test", 0)
	if err == nil {
		t.Fatal("expected error for ttl=0")
	}
}

func TestAcquire_EmptyKey(t *testing.T) {
	_, err := Acquire(context.Background(), "", time.Second)
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestRelease_NilSafe(t *testing.T) {
	var l *Lease
	l.Release(context.Background()) // 不应 panic
}

func TestIsHeld_NilSafe(t *testing.T) {
	var l *Lease
	if l.IsHeld(context.Background()) {
		t.Fatal("nil lease should not be held")
	}
}

func TestRunIfLeader_RespectCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		// 即使 Redis 不可达，RunIfLeader 看到 ctx.Done 应立刻返回。
		RunIfLeader(ctx, "test:ctx", time.Second, func(context.Context) {})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunIfLeader did not respect cancelled ctx")
	}
}

func TestInstanceID_Stable(t *testing.T) {
	first := InstanceID()
	second := InstanceID()
	if first != second {
		t.Fatalf("InstanceID should be stable across calls: %s vs %s", first, second)
	}
	if first == "" {
		t.Fatal("InstanceID should not be empty")
	}
}

func TestErrNotLeader_Sentinel(t *testing.T) {
	if !errors.Is(ErrNotLeader, ErrNotLeader) {
		t.Fatal("ErrNotLeader should match itself via errors.Is")
	}
}
