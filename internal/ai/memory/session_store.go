package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

// sessionStore 是短期对话存储抽象。
// inMemorySessionStore（默认）按进程持有；redisSessionStore（多实例部署）
// 把每个 session 序列化进 Redis Hash field，跨实例共享。
type sessionStore interface {
	// Load 取出 session（可能为 nil 表示首次）。
	Load(ctx context.Context, id string) (*SimpleMemory, error)
	// Save 写回 session（TTL 由 store 决定）。
	Save(ctx context.Context, mem *SimpleMemory) error
	// Delete 移除 session（如登出、Reset 触发）。
	Delete(ctx context.Context, id string) error
}

// sessionWireFormat 是 SimpleMemory 在 store 内的稳定 wire 格式。
// 不直接用 SimpleMemory.JSON 是为了控制版本兼容性（schema.Message 已自带 JSON tag，可以直接复用）。
type sessionWireFormat struct {
	ID            string            `json:"id"`
	Messages      []*schema.Message `json:"messages"`
	Summary       string            `json:"summary"`
	MaxWindowSize int               `json:"max_window_size"`
	TurnCount     int               `json:"turn_count"`
	CreatedAt     int64             `json:"created_at_unix"`
}

func toWire(mem *SimpleMemory) sessionWireFormat {
	mem.mu.Lock()
	defer mem.mu.Unlock()
	return sessionWireFormat{
		ID:            mem.ID,
		Messages:      append([]*schema.Message(nil), mem.Messages...),
		Summary:       mem.Summary,
		MaxWindowSize: mem.MaxWindowSize,
		TurnCount:     mem.turnCount,
		CreatedAt:     mem.createdAt.Unix(),
	}
}

func fromWire(w sessionWireFormat) *SimpleMemory {
	created := time.Unix(w.CreatedAt, 0)
	if w.CreatedAt == 0 {
		created = time.Now()
	}
	mem := &SimpleMemory{
		ID:            w.ID,
		Messages:      append([]*schema.Message(nil), w.Messages...),
		Summary:       w.Summary,
		MaxWindowSize: w.MaxWindowSize,
		turnCount:     w.TurnCount,
		createdAt:     created,
	}
	if mem.MaxWindowSize <= 0 {
		mem.MaxWindowSize = defaultMaxWindowSize
	}
	return mem
}

// === Redis backend ===

// redisSessionStore 用 Redis String 存每个 session（key = prefix:id）。
// 选 String 不选 Hash field：String 自带 EX，session-level TTL 精确；
// Hash 整体 TTL 会被新 session 写入连带刷新，造成"老 session 永远不过期"。
type redisSessionStore struct {
	redis  *gredis.Redis
	prefix string
	ttl    time.Duration
}

func newRedisSessionStore(redis *gredis.Redis, prefix string, ttl time.Duration) *redisSessionStore {
	if prefix == "" {
		prefix = "opscaption:session:"
	}
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	if ttl <= 0 {
		ttl = sessionTTL
	}
	return &redisSessionStore{redis: redis, prefix: prefix, ttl: ttl}
}

func (s *redisSessionStore) key(id string) string { return s.prefix + id }

func (s *redisSessionStore) Load(ctx context.Context, id string) (*SimpleMemory, error) {
	if id == "" {
		return nil, errors.New("empty session id")
	}
	val, err := s.redis.Do(ctx, "GET", s.key(id))
	if err != nil {
		return nil, fmt.Errorf("redis get session: %w", err)
	}
	if val == nil || val.IsNil() {
		return nil, nil
	}
	var w sessionWireFormat
	if err := json.Unmarshal([]byte(val.String()), &w); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return fromWire(w), nil
}

func (s *redisSessionStore) Save(ctx context.Context, mem *SimpleMemory) error {
	if mem == nil || mem.ID == "" {
		return nil
	}
	data, err := json.Marshal(toWire(mem))
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	ttlSec := int(s.ttl.Seconds())
	if ttlSec <= 0 {
		ttlSec = int(sessionTTL.Seconds())
	}
	if _, err := s.redis.Do(ctx, "SET", s.key(mem.ID), string(data), "EX", ttlSec); err != nil {
		return fmt.Errorf("redis set session: %w", err)
	}
	return nil
}

func (s *redisSessionStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := s.redis.Do(ctx, "DEL", s.key(id))
	return err
}

// === 全局 store ===

// defaultSessionStore 是当前进程使用的 store。
// 默认 nil → GetSimpleMemory 退化到 in-process sessionMap（向后兼容）；
// main.go 在多实例模式下通过 SetSessionStore 注入 Redis 实现。
var defaultSessionStore sessionStore

// SetSessionStore 由 main 在启动时注入 store。传 nil 恢复 in-process 模式。
// 注意：调用后 sessionMap 仍保留（充当 L1 热缓存），但新数据写穿到 store。
func SetSessionStore(store sessionStore) {
	defaultSessionStore = store
}

// EnableRedisSessionStore 是给 main 用的便捷函数：
// 如果 redis 可用则切到 Redis backend，否则保持 in-process。
func EnableRedisSessionStore(prefix string, ttl time.Duration) {
	defer func() { _ = recover() }()
	redis := g.Redis()
	if redis == nil {
		return
	}
	SetSessionStore(newRedisSessionStore(redis, prefix, ttl))
}

// loadFromStore 是 GetSimpleMemory 的「先查 store」入口。
// 返回 nil 表示 store 没有或 store 未配置，应当走本地分支创建新 session。
func loadFromStore(id string) *SimpleMemory {
	if defaultSessionStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	mem, err := defaultSessionStore.Load(ctx, id)
	if err != nil {
		g.Log().Warningf(ctx, "[memory] load session %s from store failed: %v", id, err)
		return nil
	}
	return mem
}

// saveToStore 在 SimpleMemory 发生变更后异步写回 store。
// 失败只 log，不影响主流程（store 是「最终一致」语义）。
func saveToStore(mem *SimpleMemory) {
	if defaultSessionStore == nil || mem == nil || mem.ID == "" {
		return
	}
	// 同步写：保证下一次跨实例 Load 立刻拿到最新；
	// 失败只 log，不阻断主流程。
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := defaultSessionStore.Save(ctx, mem); err != nil {
		g.Log().Warningf(ctx, "[memory] save session %s to store failed: %v", mem.ID, err)
	}
}

func deleteFromStore(id string) {
	if defaultSessionStore == nil || id == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := defaultSessionStore.Delete(ctx, id); err != nil {
		g.Log().Warningf(ctx, "[memory] delete session %s from store failed: %v", id, err)
	}
}
