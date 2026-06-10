package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

// ChangeEventStore 是变更事件的主事实存储接口。
// 结构化查询优先，RAG 仅做语义补充。
type ChangeEventStore interface {
	ReserveDedupeKey(ctx context.Context, key string, eventID string) (bool, string, error)
	ReleaseDedupeKey(ctx context.Context, key string) error
	Save(ctx context.Context, event *protocol.ChangeEvent) error
	GetByID(ctx context.Context, eventID string) (*protocol.ChangeEvent, error)
	ExistsByDedupeKey(ctx context.Context, key string) (bool, error)
	Query(ctx context.Context, filter protocol.ChangeEventFilter) ([]*protocol.ChangeEvent, error)
	Delete(ctx context.Context, eventID string) error
	Cleanup(ctx context.Context, before time.Time) (int, error)
}

// ringBuffer 保留最近 N 条变更事件在内存中，用于排障时毫秒级关联。
type ringBuffer struct {
	mu     sync.RWMutex
	events []*protocol.ChangeEvent
	size   int
	head   int
	count  int
}

func newRingBuffer(size int) *ringBuffer {
	if size <= 0 {
		size = 200
	}
	return &ringBuffer{
		events: make([]*protocol.ChangeEvent, size),
		size:   size,
	}
}

func (rb *ringBuffer) Add(event *protocol.ChangeEvent) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.events[rb.head] = event
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
}

func (rb *ringBuffer) RecentByService(service string, since time.Time, limit int) []*protocol.ChangeEvent {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	var result []*protocol.ChangeEvent
	for i := 0; i < rb.count; i++ {
		idx := (rb.head - 1 - i + rb.size) % rb.size
		e := rb.events[idx]
		if e == nil {
			break
		}
		if e.StartedAt.Before(since) {
			continue
		}
		if service != "" && !strings.EqualFold(e.Service, service) {
			continue
		}
		result = append(result, e)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func (rb *ringBuffer) RecentAll(since time.Time, limit int) []*protocol.ChangeEvent {
	return rb.RecentByService("", since, limit)
}

// RedisChangeEventStore 使用 Redis 存储变更事件。
// 数据结构（每事件独立 key，便于 TTL 精确生效）：
//
//	String: ce:data:{event_id}     → JSON ChangeEvent (EX = retention_hours)
//	ZSet:   ce:index:time          → (event_id, started_at.Unix())
//	ZSet:   ce:svc:{service}       → (event_id, started_at.Unix())   ←  替代旧的 SET
//	String: ce:dedup:{dedupe_key}  → event_id (EX = retention_hours)
//
// 设计要点：
//   - data 用独立 key + EX：每事件按各自 retention 自动过期，不会因 hash 整体 TTL
//     而被「后写事件刷新」连带保留旧字段（旧 schema 的 ce:data 哈希就有这个坑）。
//   - svc 索引用 ZSet（score=started_at.Unix()）：
//     1) ZREVRANGEBYSCORE 精确取「最近 N 条」而非旧 SRANDMEMBER 的随机采样；
//     2) Cleanup / max_events trim 时可同步从 svc 索引剔除 ID。
type RedisChangeEventStore struct {
	redis          *gredis.Redis
	prefix         string
	retentionHours int
	maxEvents      int
}

// MemoryChangeEventStore is an in-process store for local development and tests.
type MemoryChangeEventStore struct {
	mu      sync.RWMutex
	events  map[string]*protocol.ChangeEvent
	dedupes map[string]string
}

func NewMemoryChangeEventStore() *MemoryChangeEventStore {
	return &MemoryChangeEventStore{
		events:  make(map[string]*protocol.ChangeEvent),
		dedupes: make(map[string]string),
	}
}

func (s *MemoryChangeEventStore) ReserveDedupeKey(_ context.Context, key string, eventID string) (bool, string, error) {
	if key == "" {
		return true, "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID := s.dedupes[key]; existingID != "" {
		return false, existingID, nil
	}
	s.dedupes[key] = eventID
	return true, "", nil
}

func (s *MemoryChangeEventStore) ReleaseDedupeKey(_ context.Context, key string) error {
	if key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.dedupes, key)
	return nil
}

func (s *MemoryChangeEventStore) Save(_ context.Context, event *protocol.ChangeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[event.EventID] = event
	if event.DedupeKey != "" {
		s.dedupes[event.DedupeKey] = event.EventID
	}
	return nil
}

func (s *MemoryChangeEventStore) GetByID(_ context.Context, eventID string) (*protocol.ChangeEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.events[eventID], nil
}

func (s *MemoryChangeEventStore) ExistsByDedupeKey(_ context.Context, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dedupes[key] != "", nil
}

func (s *MemoryChangeEventStore) Query(_ context.Context, filter protocol.ChangeEventFilter) ([]*protocol.ChangeEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := make([]*protocol.ChangeEvent, 0, len(s.events))
	for _, event := range s.events {
		if matchServices(event, filter.Services) && matchFilter(event, filter) {
			events = append(events, event)
		}
	}
	sortChangeEvents(events)
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func (s *MemoryChangeEventStore) Delete(_ context.Context, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event := s.events[eventID]; event != nil && event.DedupeKey != "" {
		delete(s.dedupes, event.DedupeKey)
	}
	delete(s.events, eventID)
	return nil
}

func (s *MemoryChangeEventStore) Cleanup(_ context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for id, event := range s.events {
		if event.StartedAt.Before(before) {
			if event.DedupeKey != "" {
				delete(s.dedupes, event.DedupeKey)
			}
			delete(s.events, id)
			count++
		}
	}
	return count, nil
}

func NewRedisChangeEventStore(redis *gredis.Redis, prefix string, retentionHours int, maxEvents ...int) *RedisChangeEventStore {
	if prefix == "" {
		prefix = "opscaptionai:ce:"
	}
	if retentionHours <= 0 {
		retentionHours = 720 // 30 days
	}
	maxEventCount := 10000
	if len(maxEvents) > 0 && maxEvents[0] > 0 {
		maxEventCount = maxEvents[0]
	}
	return &RedisChangeEventStore{
		redis:          redis,
		prefix:         prefix,
		retentionHours: retentionHours,
		maxEvents:      maxEventCount,
	}
}

func (s *RedisChangeEventStore) ReserveDedupeKey(ctx context.Context, key string, eventID string) (bool, string, error) {
	if key == "" {
		return true, "", nil
	}
	ttl := s.retentionHours * 3600
	dedupKey := s.prefix + "dedup:" + key
	val, err := s.redis.Do(ctx, "SET", dedupKey, eventID, "NX", "EX", ttl)
	if err != nil {
		return false, "", fmt.Errorf("reserve dedupe key: %w", err)
	}
	if val.IsNil() || val.String() == "" {
		existing, getErr := s.redis.Do(ctx, "GET", dedupKey)
		if getErr != nil {
			return false, "", fmt.Errorf("get dedupe event id: %w", getErr)
		}
		return false, existing.String(), nil
	}
	return true, "", nil
}

func (s *RedisChangeEventStore) ReleaseDedupeKey(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := s.redis.Do(ctx, "DEL", s.prefix+"dedup:"+key)
	return err
}

func (s *RedisChangeEventStore) Save(ctx context.Context, event *protocol.ChangeEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal change event: %w", err)
	}
	ttl := s.retentionHours * 3600
	score := event.StartedAt.Unix()

	// 1. data: 独立 key + EX，让每条事件按自身 retention 自然过期。
	dataKey := s.prefix + "data:" + event.EventID
	if _, err := s.redis.Do(ctx, "SET", dataKey, string(data), "EX", ttl); err != nil {
		return fmt.Errorf("save event data: %w", err)
	}

	// 2. 时间索引（全局），用于按时间窗口查询和 cleanup。
	if _, err := s.redis.Do(ctx, "ZADD", s.prefix+"index:time", score, event.EventID); err != nil {
		return fmt.Errorf("add to time index: %w", err)
	}

	// 3. 服务索引（每服务一个 ZSet），按 started_at 排序，
	//    便于 ZREVRANGEBYSCORE 取最近 N 条（非随机）。
	if event.Service != "" {
		svcKey := s.prefix + "svc:" + strings.ToLower(event.Service)
		if _, err := s.redis.Do(ctx, "ZADD", svcKey, score, event.EventID); err != nil {
			return fmt.Errorf("add to service index: %w", err)
		}
		// ZSet 整体 TTL：随着新事件写入持续刷新；最坏情况下
		// 索引外活 retention，最终被 Cleanup 兜底清理。
		if _, err := s.redis.Do(ctx, "EXPIRE", svcKey, ttl); err != nil {
			g.Log().Warningf(ctx, "[change_event] EXPIRE on %s failed: %v", svcKey, err)
		}
	}

	// 4. Dedupe key。
	if event.DedupeKey != "" {
		dedupKey := s.prefix + "dedup:" + event.DedupeKey
		if _, err := s.redis.Do(ctx, "SET", dedupKey, event.EventID, "EX", ttl); err != nil {
			return fmt.Errorf("set dedupe key: %w", err)
		}
	}

	// 5. max_events 上限：用 ZREMRANGEBYRANK 原子修剪时间索引。
	//    被淘汰条目的 data key 会在 retention TTL 自然过期；
	//    svc 索引的孤儿条目由 Cleanup() 用 ZREMRANGEBYSCORE 一次性清理。
	//    采用原子单命令避免 ZRANGE+Delete 在并发 Save 下的 TOCTOU。
	if s.maxEvents > 0 {
		_, _ = s.redis.Do(ctx, "ZREMRANGEBYRANK", s.prefix+"index:time", 0, -(s.maxEvents + 1))
	}

	return nil
}

func (s *RedisChangeEventStore) GetByID(ctx context.Context, eventID string) (*protocol.ChangeEvent, error) {
	val, err := s.redis.Do(ctx, "GET", s.prefix+"data:"+eventID)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	if val.IsNil() || val.String() == "" {
		return nil, nil
	}
	var event protocol.ChangeEvent
	if err := json.Unmarshal([]byte(val.String()), &event); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}
	return &event, nil
}

func (s *RedisChangeEventStore) ExistsByDedupeKey(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	val, err := s.redis.Do(ctx, "EXISTS", s.prefix+"dedup:"+key)
	if err != nil {
		return false, fmt.Errorf("check dedupe key: %w", err)
	}
	return val.Int64() > 0, nil
}

func (s *RedisChangeEventStore) Query(ctx context.Context, filter protocol.ChangeEventFilter) ([]*protocol.ChangeEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	// 时间窗口由 filter 决定，转换为 ZSet score。
	minScore := "-inf"
	maxScore := "+inf"
	if filter.Since != nil {
		minScore = strconv.FormatInt(filter.Since.Unix(), 10)
	}
	if filter.Until != nil {
		maxScore = strconv.FormatInt(filter.Until.Unix(), 10)
	}

	// 多取一些（limit*4）作为筛选 buffer，确保 risk/cluster/type 过滤后仍够用。
	fetchLimit := limit * 4
	if fetchLimit < limit {
		fetchLimit = limit
	}

	var eventIDs []string
	if len(filter.Services) > 0 {
		// 按服务查询：每服务的 ZSet 按 started_at 排序，取最近 N 条，
		// 替代旧 SRANDMEMBER 的随机采样。
		seen := make(map[string]bool)
		for _, svc := range filter.Services {
			svcKey := s.prefix + "svc:" + strings.ToLower(svc)
			val, err := s.redis.Do(ctx, "ZREVRANGEBYSCORE", svcKey, maxScore, minScore, "LIMIT", 0, fetchLimit)
			if err != nil {
				continue
			}
			for _, id := range val.Strings() {
				if !seen[id] {
					seen[id] = true
					eventIDs = append(eventIDs, id)
				}
			}
		}
	} else {
		val, err := s.redis.Do(ctx, "ZREVRANGEBYSCORE", s.prefix+"index:time", maxScore, minScore, "LIMIT", 0, fetchLimit)
		if err != nil {
			return nil, fmt.Errorf("query by time: %w", err)
		}
		eventIDs = val.Strings()
	}

	// 加载事件并按完整 filter 二次过滤（env/cluster/event_type/risk_level）。
	results := make([]*protocol.ChangeEvent, 0, len(eventIDs))
	for _, id := range eventIDs {
		event, err := s.GetByID(ctx, id)
		if err != nil || event == nil {
			continue
		}
		if !matchServices(event, filter.Services) {
			continue
		}
		if !matchFilter(event, filter) {
			continue
		}
		results = append(results, event)
	}
	sortChangeEvents(results)
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (s *RedisChangeEventStore) Delete(ctx context.Context, eventID string) error {
	// 先获取事件以清理索引（即使 data 已过期，也尝试清理 svc 索引）。
	event, _ := s.GetByID(ctx, eventID)
	if event != nil {
		if event.Service != "" {
			s.redis.Do(ctx, "ZREM", s.prefix+"svc:"+strings.ToLower(event.Service), eventID)
		}
		if event.DedupeKey != "" {
			s.redis.Do(ctx, "DEL", s.prefix+"dedup:"+event.DedupeKey)
		}
	}
	s.redis.Do(ctx, "ZREM", s.prefix+"index:time", eventID)
	_, err := s.redis.Do(ctx, "DEL", s.prefix+"data:"+eventID)
	return err
}

func (s *RedisChangeEventStore) Cleanup(ctx context.Context, before time.Time) (int, error) {
	score := strconv.FormatInt(before.Unix(), 10)
	val, err := s.redis.Do(ctx, "ZRANGEBYSCORE", s.prefix+"index:time", "-inf", score)
	if err != nil {
		return 0, err
	}
	ids := val.Strings()
	count := 0
	for _, id := range ids {
		if err := s.Delete(ctx, id); err == nil {
			count++
		}
	}

	// 额外清理：所有 svc:* ZSet 中分值 ≤ cutoff 的孤儿条目
	// （其 data key 已被本次或之前的 Delete/TTL 清理掉）。
	// 用 SCAN 迭代避免阻塞 Redis；ZREMRANGEBYSCORE 是 O(log N + M)。
	cursor := uint64(0)
	pattern := s.prefix + "svc:*"
	for {
		val, scanErr := s.redis.Do(ctx, "SCAN", cursor, "MATCH", pattern, "COUNT", 100)
		if scanErr != nil {
			break
		}
		parts := val.Slice()
		if len(parts) < 2 {
			break
		}
		nextCursor, _ := strconv.ParseUint(fmt.Sprintf("%v", parts[0]), 10, 64)
		keys, _ := parts[1].([]any)
		for _, k := range keys {
			svcKey, _ := k.(string)
			if svcKey == "" {
				continue
			}
			_, _ = s.redis.Do(ctx, "ZREMRANGEBYSCORE", svcKey, "-inf", score)
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}

	return count, nil
}

func matchFilter(event *protocol.ChangeEvent, filter protocol.ChangeEventFilter) bool {
	if filter.Env != "" && !strings.EqualFold(event.Env, filter.Env) {
		return false
	}
	if filter.Cluster != "" && !strings.EqualFold(event.Cluster, filter.Cluster) {
		return false
	}
	if filter.EventType != "" && event.EventType != filter.EventType {
		return false
	}
	if filter.Since != nil && event.StartedAt.Before(*filter.Since) {
		return false
	}
	if filter.Until != nil && event.StartedAt.After(*filter.Until) {
		return false
	}
	if filter.RiskLevel != "" {
		if !meetsRiskLevel(event.RiskLevel, filter.RiskLevel) {
			return false
		}
	}
	return true
}

func matchServices(event *protocol.ChangeEvent, services []string) bool {
	if len(services) == 0 {
		return true
	}
	for _, svc := range services {
		if strings.EqualFold(event.Service, svc) {
			return true
		}
	}
	return false
}

func sortChangeEvents(events []*protocol.ChangeEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].StartedAt.After(events[j].StartedAt)
	})
}
