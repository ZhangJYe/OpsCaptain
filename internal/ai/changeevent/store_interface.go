package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
	"sort"
	"strings"
	"sync"
	"time"
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