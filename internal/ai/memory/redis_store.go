package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

type redisMemoryStore struct {
	redis *gredis.Redis
	prefix string
}

// NewRedisMemoryStore 创建 Redis 持久化后端
// key 格式: {prefix}:memory:entry:{id}, {prefix}:memory:ids
// prefix 来自 memory.project_id 配置，默认 "opscaptionai"
func NewRedisMemoryStore(redis *gredis.Redis) LongTermMemoryStore {
	prefix := "opscaptionai"
	if v, err := g.Cfg().Get(context.Background(), "memory.project_id"); err == nil {
		if p := strings.TrimSpace(v.String()); p != "" {
			prefix = p
		}
	}
	return &redisMemoryStore{redis: redis, prefix: prefix}
}

func (s *redisMemoryStore) entryKey(id string) string {
	return fmt.Sprintf("%s:memory:entry:%s", s.prefix, id)
}

func (s *redisMemoryStore) idsKey() string {
	return fmt.Sprintf("%s:memory:ids", s.prefix)
}

func (s *redisMemoryStore) LoadAll(ctx context.Context) ([]*MemoryEntry, error) {
	ids, err := s.redis.Do(ctx, "SMEMBERS", s.idsKey())
	if err != nil {
		return nil, fmt.Errorf("smembers failed: %w", err)
	}

	idList := ids.Strings()
	if len(idList) == 0 {
		return nil, nil
	}

	entries := make([]*MemoryEntry, 0, len(idList))
	for _, id := range idList {
		val, err := s.redis.Do(ctx, "GET", s.entryKey(id))
		if err != nil {
			g.Log().Warningf(ctx, "[ltm-redis] GET entry %s failed: %v", id, err)
			continue
		}
		if val.IsEmpty() {
			g.Log().Warningf(ctx, "[ltm-redis] entry %s missing from Redis, skipping", id)
			continue
		}
		var entry MemoryEntry
		if err := json.Unmarshal(val.Bytes(), &entry); err != nil {
			g.Log().Warningf(ctx, "[ltm-redis] unmarshal entry %s failed: %v", id, err)
			continue
		}
		entries = append(entries, &entry)
	}

	g.Log().Infof(ctx, "[ltm-redis] loaded %d/%d memories from Redis", len(entries), len(idList))
	return entries, nil
}

func (s *redisMemoryStore) SaveEntries(ctx context.Context, entries []*MemoryEntry) error {
	if len(entries) == 0 {
		return nil
	}

	ids := make([]interface{}, 0, len(entries))
	for _, entry := range entries {
		body, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal entry %s: %w", entry.ID, err)
		}
		if _, err := s.redis.Do(ctx, "SET", s.entryKey(entry.ID), body); err != nil {
			return fmt.Errorf("set entry %s: %w", entry.ID, err)
		}
		ids = append(ids, entry.ID)
	}

	saddArgs := make([]interface{}, 0, len(ids)+1)
	saddArgs = append(saddArgs, s.idsKey())
	saddArgs = append(saddArgs, ids...)
	if _, err := s.redis.Do(ctx, "SADD", saddArgs...); err != nil {
		return fmt.Errorf("sadd ids: %w", err)
	}

	return nil
}

func (s *redisMemoryStore) DeleteEntries(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	delArgs := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		delArgs = append(delArgs, s.entryKey(id))
	}
	if _, err := s.redis.Do(ctx, "DEL", delArgs...); err != nil {
		return fmt.Errorf("del entries: %w", err)
	}

	sremArgs := make([]interface{}, 0, len(ids)+1)
	sremArgs = append(sremArgs, s.idsKey())
	for _, id := range ids {
		sremArgs = append(sremArgs, id)
	}
	if _, err := s.redis.Do(ctx, "SREM", sremArgs...); err != nil {
		return fmt.Errorf("srem ids: %w", err)
	}

	return nil
}
