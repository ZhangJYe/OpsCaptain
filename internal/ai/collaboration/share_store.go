package collaboration

import (
	"fmt"
	"sync"
	"time"
)

type ShareStore struct {
	mu    sync.RWMutex
	links map[string]*ShareLink
}

func NewShareStore() *ShareStore {
	return &ShareStore{
		links: make(map[string]*ShareLink),
	}
}

func (s *ShareStore) Create(sessionID, createdBy string, ttlHours int) (*ShareLink, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if ttlHours <= 0 {
		ttlHours = 24
	}

	now := time.Now().UnixMilli()
	link := &ShareLink{
		ID:        fmt.Sprintf("share_%d", now),
		SessionID: sessionID,
		CreatedBy: createdBy,
		CreatedAt: now,
		ExpiresAt: time.Now().Add(time.Duration(ttlHours) * time.Hour).UnixMilli(),
	}

	s.mu.Lock()
	s.links[link.ID] = link
	s.mu.Unlock()

	return link, nil
}

func (s *ShareStore) Get(id string) (*ShareLink, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, ok := s.links[id]
	if !ok {
		return nil, false
	}
	if time.Now().UnixMilli() > link.ExpiresAt {
		return nil, false
	}
	return link, true
}

func (s *ShareStore) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.links[id]; !ok {
		return fmt.Errorf("share link not found: %s", id)
	}
	delete(s.links, id)
	return nil
}
