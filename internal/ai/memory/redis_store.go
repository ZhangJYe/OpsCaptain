package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

const redisSaveEntriesScript = `
local n = tonumber(ARGV[1])
for i = 1, n do
  redis.call("SET", KEYS[i], ARGV[i + 1])
end
for i = 1, n do
  redis.call("SADD", KEYS[n + 1], ARGV[n + 1 + i])
end
return n
`

const redisDeleteEntriesScript = `
local n = tonumber(ARGV[1])
for i = 1, n do
  redis.call("DEL", KEYS[i])
end
for i = 1, n do
  redis.call("SREM", KEYS[n + 1], ARGV[i + 1])
end
return n
`

type redisMemoryStore struct {
	redis  *gredis.Redis
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

	keys := make([]interface{}, 0, len(entries)+1)
	args := make([]interface{}, 0, len(entries)*2+1)
	args = append(args, strconv.Itoa(len(entries)))
	for _, entry := range entries {
		body, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal entry %s: %w", entry.ID, err)
		}
		keys = append(keys, s.entryKey(entry.ID))
		args = append(args, string(body))
	}
	keys = append(keys, s.idsKey())
	for _, entry := range entries {
		args = append(args, entry.ID)
	}
	cmdArgs := make([]interface{}, 0, len(keys)+len(args)+2)
	cmdArgs = append(cmdArgs, redisSaveEntriesScript, len(keys))
	cmdArgs = append(cmdArgs, keys...)
	cmdArgs = append(cmdArgs, args...)
	if _, err := s.redis.Do(ctx, "EVAL", cmdArgs...); err != nil {
		return fmt.Errorf("save entries transaction failed: %w", err)
	}

	return nil
}

func (s *redisMemoryStore) DeleteEntries(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	keys := make([]interface{}, 0, len(ids)+1)
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, strconv.Itoa(len(ids)))
	for _, id := range ids {
		keys = append(keys, s.entryKey(id))
		args = append(args, id)
	}
	keys = append(keys, s.idsKey())
	cmdArgs := make([]interface{}, 0, len(keys)+len(args)+2)
	cmdArgs = append(cmdArgs, redisDeleteEntriesScript, len(keys))
	cmdArgs = append(cmdArgs, keys...)
	cmdArgs = append(cmdArgs, args...)
	if _, err := s.redis.Do(ctx, "EVAL", cmdArgs...); err != nil {
		return fmt.Errorf("delete entries transaction failed: %w", err)
	}

	return nil
}
