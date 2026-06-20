package feedback

import (
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// StoreInterface defines the feedback store contract.
type StoreInterface interface {
	Submit(entry *FeedbackEntry) error
	GetBySession(sessionID string) []FeedbackEntry
	Stats() FeedbackStats
}

type Store struct {
	mu      sync.RWMutex
	entries map[string]*FeedbackEntry
	stats   FeedbackStats
}

func NewStore() *Store {
	return &Store{
		entries: make(map[string]*FeedbackEntry),
	}
}

func (s *Store) Submit(entry *FeedbackEntry) error {
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = fmt.Sprintf("fb_%d", time.Now().UnixNano())
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().UnixMilli()
	}

	s.entries[entry.ID] = entry
	s.recalcStats()

	g.Log().Debugf(nil, "[feedback] submitted: id=%s session=%s rating=%s", entry.ID, entry.SessionID, entry.Rating)
	return nil
}

func (s *Store) GetBySession(sessionID string) []FeedbackEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []FeedbackEntry
	for _, e := range s.entries {
		if e.SessionID == sessionID {
			result = append(result, *e)
		}
	}
	return result
}

func (s *Store) Stats() FeedbackStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func (s *Store) recalcStats() {
	var stats FeedbackStats
	for _, e := range s.entries {
		stats.Total++
		switch e.Rating {
		case RatingHelpful:
			stats.Helpful++
		case RatingNotHelpful:
			stats.NotHelpful++
		}
	}
	if stats.Total > 0 {
		stats.Score = float64(stats.Helpful) / float64(stats.Total)
	}
	s.stats = stats
}
