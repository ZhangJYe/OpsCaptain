package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const redisKeyPrefix = "opscaption:feedback:"
const redisStatsKey = "opscaption:feedback:stats"

// RedisStore persists feedback entries in Redis.
type RedisStore struct{}

// NewRedisStore creates a Redis-backed feedback store.
func NewRedisStore() *RedisStore {
	return &RedisStore{}
}

func (s *RedisStore) Submit(entry *FeedbackEntry) error {
	if entry == nil {
		return fmt.Errorf("feedback entry is nil")
	}
	if entry.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if entry.Query == "" {
		return fmt.Errorf("query is required")
	}
	if entry.Rating != RatingHelpful && entry.Rating != RatingNotHelpful {
		return fmt.Errorf("rating must be helpful or not_helpful")
	}

	ctx := context.Background()
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("fb_%d", time.Now().UnixNano())
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().UnixMilli()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal feedback: %w", err)
	}

	key := redisKeyPrefix + entry.ID
	if _, err := g.Redis().Do(ctx, "SETEX", key, 30*24*3600, string(data)); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}

	// Update stats
	if err := s.updateStats(ctx, entry); err != nil {
		g.Log().Warningf(ctx, "feedback: failed to update stats: %v", err)
	}

	g.Log().Debugf(ctx, "[feedback] submitted (redis): id=%s session=%s rating=%s", entry.ID, entry.SessionID, entry.Rating)
	return nil
}

func (s *RedisStore) GetBySession(sessionID string) []FeedbackEntry {
	ctx := context.Background()
	result, err := g.Redis().Do(ctx, "KEYS", redisKeyPrefix+"*")
	if err != nil {
		return nil
	}

	keys := result.Strings()
	if len(keys) == 0 {
		return nil
	}

	var entries []FeedbackEntry
	for _, key := range keys {
		val, err := g.Redis().Do(ctx, "GET", key)
		if err != nil || val == nil || val.IsEmpty() {
			continue
		}
		var entry FeedbackEntry
		if err := json.Unmarshal([]byte(val.String()), &entry); err != nil {
			continue
		}
		if entry.SessionID == sessionID {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (s *RedisStore) Stats() FeedbackStats {
	ctx := context.Background()
	val, err := g.Redis().Do(ctx, "GET", redisStatsKey)
	if err != nil || val == nil || val.IsEmpty() {
		return FeedbackStats{}
	}
	var stats FeedbackStats
	if err := json.Unmarshal([]byte(val.String()), &stats); err != nil {
		return FeedbackStats{}
	}
	return stats
}

func (s *RedisStore) updateStats(ctx context.Context, entry *FeedbackEntry) error {
	stats := s.Stats()
	stats.Total++
	switch entry.Rating {
	case RatingHelpful:
		stats.Helpful++
	case RatingNotHelpful:
		stats.NotHelpful++
	}
	if stats.Total > 0 {
		stats.Score = float64(stats.Helpful) / float64(stats.Total)
	}

	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	_, err = g.Redis().Do(ctx, "SETEX", redisStatsKey, 30*24*3600, string(data))
	return err
}
