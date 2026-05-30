package rabbitmq

import (
	"strings"
	"sync"
	"time"
)

// TTLSet is a thread-safe set that automatically expires entries after a TTL
// and evicts the oldest entry when maxEntries is reached.
type TTLSet struct {
	mu         sync.Mutex
	items      map[string]time.Time
	ttl        time.Duration
	maxEntries int
}

// NewTTLSet creates a TTLSet. ttl and maxEntries must be positive.
func NewTTLSet(ttl time.Duration, maxEntries int) *TTLSet {
	return &TTLSet{
		items:      make(map[string]time.Time),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

// Has returns true if key exists and has not expired.
func (s *TTLSet) Has(key string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	expireAt, ok := s.items[key]
	if !ok {
		return false
	}
	if now.After(expireAt) {
		delete(s.items, key)
		return false
	}
	return true
}

// Mark adds key to the set with the configured TTL. Empty keys are ignored.
func (s *TTLSet) Mark(key string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	if len(s.items) >= s.maxEntries {
		for existing := range s.items {
			delete(s.items, existing)
			break
		}
	}
	s.items[key] = now.Add(s.ttl)
}

func (s *TTLSet) pruneExpiredLocked(now time.Time) {
	for key, expireAt := range s.items {
		if now.After(expireAt) {
			delete(s.items, key)
		}
	}
}
