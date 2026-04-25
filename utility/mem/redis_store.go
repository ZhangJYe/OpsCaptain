package mem

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	redisSessionKeyPrefix = "opscaption:mem:session:"
	redisLTMKeyPrefix     = "opscaption:mem:ltm:"
	redisLTMIndexPrefix   = "opscaption:mem:ltm_idx:"
	redisSessionTTL       = 2 * time.Hour
	redisLTMTTL           = 7 * 24 * time.Hour
)

type RedisSessionStore struct{}

func NewRedisSessionStore() *RedisSessionStore {
	return &RedisSessionStore{}
}

func (r *RedisSessionStore) LoadMessages(ctx context.Context, sessionID string) ([]*schema.Message, string, error) {
	key := redisSessionKeyPrefix + sessionID
	raw, err := g.Redis().Do(ctx, "GET", key)
	if err != nil {
		return nil, "", err
	}
	if raw.IsNil() || raw.String() == "" {
		return nil, "", nil
	}
	var s serializedSession
	if err := json.Unmarshal([]byte(raw.String()), &s); err != nil {
		return nil, "", fmt.Errorf("unmarshal session %s: %w", sessionID, err)
	}
	return deserializeMessages(s.Messages), s.Summary, nil
}

func (r *RedisSessionStore) SaveMessages(ctx context.Context, sessionID string, messages []*schema.Message, summary string) error {
	s := serializedSession{
		Messages: serializeMessages(messages),
		Summary:  summary,
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	key := redisSessionKeyPrefix + sessionID
	_, err = g.Redis().Do(ctx, "SET", key, string(data), "EX", int(redisSessionTTL.Seconds()))
	return err
}

func (r *RedisSessionStore) Touch(ctx context.Context, sessionID string) error {
	key := redisSessionKeyPrefix + sessionID
	_, err := g.Redis().Do(ctx, "EXPIRE", key, int(redisSessionTTL.Seconds()))
	return err
}

func (r *RedisSessionStore) Delete(ctx context.Context, sessionID string) error {
	key := redisSessionKeyPrefix + sessionID
	_, err := g.Redis().Do(ctx, "DEL", key)
	return err
}

func (r *RedisSessionStore) Exists(ctx context.Context, sessionID string) (bool, error) {
	key := redisSessionKeyPrefix + sessionID
	v, err := g.Redis().Do(ctx, "EXISTS", key)
	if err != nil {
		return false, err
	}
	return v.Int() > 0, nil
}

type RedisLongTermStore struct{}

func NewRedisLongTermStore() *RedisLongTermStore {
	return &RedisLongTermStore{}
}

func (r *RedisLongTermStore) StoreEntry(ctx context.Context, entry *MemoryEntry) error {
	data, err := json.Marshal(serializeEntry(entry))
	if err != nil {
		return err
	}
	key := redisLTMKeyPrefix + entry.ID
	_, err = g.Redis().Do(ctx, "SET", key, string(data), "EX", int(redisLTMTTL.Seconds()))
	if err != nil {
		return err
	}
	idxKey := redisLTMIndexPrefix + entry.SessionID
	_, err = g.Redis().Do(ctx, "SADD", idxKey, entry.ID)
	if err != nil {
		return err
	}
	_, _ = g.Redis().Do(ctx, "EXPIRE", idxKey, int(redisLTMTTL.Seconds()))
	return nil
}

func (r *RedisLongTermStore) LoadEntries(ctx context.Context, sessionID string) ([]*MemoryEntry, error) {
	idxKey := redisLTMIndexPrefix + sessionID
	raw, err := g.Redis().Do(ctx, "SMEMBERS", idxKey)
	if err != nil {
		return nil, err
	}
	ids := raw.Strings()
	if len(ids) == 0 {
		return nil, nil
	}

	entries := make([]*MemoryEntry, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		entryRaw, err := g.Redis().Do(ctx, "GET", redisLTMKeyPrefix+id)
		if err != nil || entryRaw.IsNil() || entryRaw.String() == "" {
			_, _ = g.Redis().Do(ctx, "SREM", idxKey, id)
			continue
		}
		var s serializedMemoryEntry
		if err := json.Unmarshal([]byte(entryRaw.String()), &s); err != nil {
			continue
		}
		entries = append(entries, deserializeEntry(s))
	}
	return entries, nil
}

func (r *RedisLongTermStore) UpdateEntry(ctx context.Context, entry *MemoryEntry) error {
	return r.StoreEntry(ctx, entry)
}

func (r *RedisLongTermStore) DeleteEntry(ctx context.Context, id string) error {
	key := redisLTMKeyPrefix + id
	raw, err := g.Redis().Do(ctx, "GET", key)
	if err == nil && !raw.IsNil() && raw.String() != "" {
		var s serializedMemoryEntry
		if err := json.Unmarshal([]byte(raw.String()), &s); err == nil {
			idxKey := redisLTMIndexPrefix + s.SessionID
			_, _ = g.Redis().Do(ctx, "SREM", idxKey, id)
		}
	}
	_, err = g.Redis().Do(ctx, "DEL", key)
	return err
}

func (r *RedisLongTermStore) CountBySession(ctx context.Context, sessionID string) (int, error) {
	idxKey := redisLTMIndexPrefix + sessionID
	v, err := g.Redis().Do(ctx, "SCARD", idxKey)
	if err != nil {
		return 0, err
	}
	return v.Int(), nil
}

func (r *RedisLongTermStore) CountAll(ctx context.Context) (int, error) {
	return 0, nil
}
