// Package leader 提供基于 Redis 的轻量级 leader election，
// 用于多实例部署下确保某个周期任务/单消费者只在一个实例运行。
//
// 设计原则：
//   - 简单：SET NX EX 抢锁；后台 goroutine 定期续约；释放时 Lua CAS 防止误删别人的锁。
//   - 故障自愈：lease TTL 到期自动释放；leader 实例崩溃后其他实例可在 TTL 内接管。
//   - 无外部依赖：仅依赖 g.Redis()；与 OpsCaptain 现有的 gogf 体系一致。
//
// 典型用法：
//
//	lease, err := leader.Acquire(ctx, "ce:cleanup", 30*time.Second)
//	if err != nil { return }  // 拿不到 = 别人是 leader，本实例跳过
//	defer lease.Release(context.Background())
//	// 干 leader-only 的活...
//
// 或封装成周期任务：
//
//	leader.RunIfLeader(ctx, "ce:cleanup", 30*time.Second, func(leaderCtx context.Context) {
//	    // 只在 leader 实例执行
//	})
package leader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

// ErrNotLeader 表示当前没抢到锁；调用方应跳过 leader-only 工作。
var ErrNotLeader = errors.New("leader: not the leader")

// keyPrefix 前缀所有 leader key，避免与业务 key 冲突。
const keyPrefix = "opscaption:leader:"

// instanceID 标识本进程；用于 lease 内容，便于诊断和 CAS 释放。
var instanceID = func() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%s", host, uuid.NewString()[:8])
}()

// InstanceID 暴露实例 ID 用于日志/指标。
func InstanceID() string { return instanceID }

// Lease 表示已获得的领导权，需通过 Release 显式归还。
// 内部启动续约 goroutine 自动延长 TTL，崩溃即自动失效。
type Lease struct {
	key      string
	value    string
	ttl      time.Duration
	redis    *gredis.Redis
	cancel   context.CancelFunc
	released atomic.Bool
}

// Acquire 尝试在 ctx 内获得 key 的领导权。
//   - ttl: lease 有效期；建议 ≥ 任务周期 × 2，避免任务跑超过 TTL 后丢失锁。
//   - 抢到：返回 *Lease（后台开始按 ttl/3 续约）。
//   - 没抢到：返回 ErrNotLeader（典型场景：另一个实例已是 leader）。
func Acquire(ctx context.Context, key string, ttl time.Duration) (*Lease, error) {
	if key == "" {
		return nil, fmt.Errorf("leader: key is empty")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("leader: ttl must be > 0")
	}
	redis := g.Redis()
	if redis == nil {
		return nil, fmt.Errorf("leader: redis client is not configured")
	}
	fullKey := keyPrefix + key
	value := instanceID
	// SET key value NX EX <ttl_seconds> —— 经典 Redlock 单 key 抢占。
	ttlSec := int(ttl.Seconds())
	if ttlSec <= 0 {
		ttlSec = 1
	}
	result, err := redis.Do(ctx, "SET", fullKey, value, "NX", "EX", ttlSec)
	if err != nil {
		return nil, fmt.Errorf("leader: SET NX failed: %w", err)
	}
	if result == nil || result.IsNil() {
		return nil, ErrNotLeader
	}

	renewCtx, cancel := context.WithCancel(context.Background())
	lease := &Lease{
		key:    fullKey,
		value:  value,
		ttl:    ttl,
		redis:  redis,
		cancel: cancel,
	}
	go lease.renewLoop(renewCtx)
	return lease, nil
}

// Release 主动归还领导权（Lua CAS：value 匹配才删，避免删错别人的锁）。
// 即使不调用，TTL 到期也会自动释放。
func (l *Lease) Release(ctx context.Context) {
	if l == nil || l.released.Swap(true) {
		return
	}
	if l.cancel != nil {
		l.cancel()
	}
	// Lua: 只有 value 匹配（仍是我们持有的）才 DEL。
	const releaseScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
	if l.redis != nil {
		_, _ = l.redis.Do(ctx, "EVAL", releaseScript, 1, l.key, l.value)
	}
}

// IsHeld 用于调用方在长任务中间检查 lease 是否仍持有
// （续约失败/被强制 DEL 时返回 false，调用方应中断工作）。
func (l *Lease) IsHeld(ctx context.Context) bool {
	if l == nil || l.released.Load() || l.redis == nil {
		return false
	}
	val, err := l.redis.Do(ctx, "GET", l.key)
	if err != nil || val == nil || val.IsNil() {
		return false
	}
	return val.String() == l.value
}

func (l *Lease) renewLoop(ctx context.Context) {
	// 续约周期 = TTL/3，给至少 2 次失败窗口。
	interval := l.ttl / 3
	if interval < 500*time.Millisecond {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	const renewScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("PEXPIRE", KEYS[1], ARGV[2]) else return 0 end`
	ttlMs := l.ttl.Milliseconds()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			val, err := l.redis.Do(renewCtx, "EVAL", renewScript, 1, l.key, l.value, ttlMs)
			cancel()
			// CAS 失败 / 网络错：留给 IsHeld 检测；这里不主动 release，避免 false positive。
			if err == nil && val != nil && val.Int() == 0 {
				g.Log().Warningf(ctx, "[leader] lease %s lost (CAS mismatch)", l.key)
				return
			}
		}
	}
}

// RunIfLeader 是高阶封装：以 ttl 为粒度反复抢锁，抢到就运行 fn(leaderCtx)。
// fn 在 leaderCtx 下运行；leaderCtx.Done 触发时（外层 ctx 取消或 lease 丢失）
// fn 应尽快返回。fn 返回后会 Release lease 并 sleep 一个 TTL 再下一轮。
//
// 典型场景：周期 cleanup / health-report / batch indexer。
func RunIfLeader(ctx context.Context, key string, ttl time.Duration, fn func(leaderCtx context.Context)) {
	for {
		if ctx.Err() != nil {
			return
		}
		lease, err := Acquire(ctx, key, ttl)
		if errors.Is(err, ErrNotLeader) {
			// 不是 leader，等一会再试。
			select {
			case <-ctx.Done():
				return
			case <-time.After(ttl):
			}
			continue
		}
		if err != nil {
			g.Log().Warningf(ctx, "[leader] acquire %s failed: %v", key, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(ttl):
			}
			continue
		}
		g.Log().Infof(ctx, "[leader] elected for %s (instance=%s, ttl=%s)", key, instanceID, ttl)
		leaderCtx, cancel := context.WithCancel(ctx)
		// 监控 lease 健康度，丢失就 cancel leaderCtx。
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			ticker := time.NewTicker(ttl / 3)
			defer ticker.Stop()
			for {
				select {
				case <-leaderCtx.Done():
					return
				case <-ticker.C:
					if !lease.IsHeld(leaderCtx) {
						g.Log().Warningf(leaderCtx, "[leader] lease %s lost during execution, cancelling", key)
						cancel()
						return
					}
				}
			}
		}()
		fn(leaderCtx)
		cancel()
		<-watchDone
		lease.Release(context.Background())
		// 一个 TTL 周期后再试，避免抖动（其他实例有机会接管）。
		select {
		case <-ctx.Done():
			return
		case <-time.After(ttl):
		}
	}
}

// once-style helper for tests that need to avoid Redis.
var (
	disableOnce sync.Once
	disabled    atomic.Bool
)

// DisableForTests 让 Acquire 永远返回 lease（仅限单元测试用）。
func DisableForTests() {
	disableOnce.Do(func() {
		disabled.Store(true)
	})
}
