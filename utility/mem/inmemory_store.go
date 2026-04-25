package mem

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/schema"
)

type InMemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]*serializedSession
}

func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]*serializedSession),
	}
}

func (s *InMemorySessionStore) LoadMessages(ctx context.Context, sessionID string) ([]*schema.Message, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, "", nil
	}
	return deserializeMessages(sess.Messages), sess.Summary, nil
}

func (s *InMemorySessionStore) SaveMessages(ctx context.Context, sessionID string, messages []*schema.Message, summary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = &serializedSession{
		Messages: serializeMessages(messages),
		Summary:  summary,
	}
	return nil
}

func (s *InMemorySessionStore) Touch(ctx context.Context, sessionID string) error {
	return nil
}

func (s *InMemorySessionStore) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

func (s *InMemorySessionStore) Exists(ctx context.Context, sessionID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sessions[sessionID]
	return ok, nil
}

type InMemoryLongTermStore struct {
	mu      sync.Mutex
	entries map[string]*MemoryEntry
	index   map[string][]string
}

func NewInMemoryLongTermStore() *InMemoryLongTermStore {
	return &InMemoryLongTermStore{
		entries: make(map[string]*MemoryEntry),
		index:   make(map[string][]string),
	}
}

func (s *InMemoryLongTermStore) StoreEntry(ctx context.Context, entry *MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entry.ID] = entry
	ids := s.index[entry.SessionID]
	for _, id := range ids {
		if id == entry.ID {
			return nil
		}
	}
	s.index[entry.SessionID] = append(ids, entry.ID)
	return nil
}

func (s *InMemoryLongTermStore) LoadEntries(ctx context.Context, sessionID string) ([]*MemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.index[sessionID]
	result := make([]*MemoryEntry, 0, len(ids))
	for _, id := range ids {
		if e, ok := s.entries[id]; ok {
			result = append(result, e)
		}
	}
	return result, nil
}

func (s *InMemoryLongTermStore) UpdateEntry(ctx context.Context, entry *MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entry.ID] = entry
	return nil
}

func (s *InMemoryLongTermStore) DeleteEntry(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok {
		return nil
	}
	delete(s.entries, id)
	ids := s.index[entry.SessionID]
	for i, sid := range ids {
		if sid == id {
			s.index[entry.SessionID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	return nil
}

func (s *InMemoryLongTermStore) CountBySession(ctx context.Context, sessionID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.index[sessionID]), nil
}

func (s *InMemoryLongTermStore) CountAll(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries), nil
}
