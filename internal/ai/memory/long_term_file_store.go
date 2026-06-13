package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

type fileLongTermMemoryStore struct {
	path string
}

func NewFileLongTermMemoryStore(path string) LongTermMemoryStore {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &fileLongTermMemoryStore{path: path}
}

func (s *fileLongTermMemoryStore) LoadAll(ctx context.Context) ([]*MemoryEntry, error) {
	body, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	var entries []*MemoryEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}
	return entries, ctx.Err()
}

func (s *fileLongTermMemoryStore) SaveEntries(ctx context.Context, entries []*MemoryEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := s.LoadAll(ctx)
	if err != nil {
		return fmt.Errorf("load existing memory store before save: %w", err)
	}
	byID := make(map[string]*MemoryEntry, len(existing)+len(entries))
	for _, e := range existing {
		byID[e.ID] = e
	}
	for _, e := range entries {
		byID[e.ID] = e
	}
	merged := make([]*MemoryEntry, 0, len(byID))
	for _, e := range byID {
		merged = append(merged, e)
	}
	return s.writeAll(merged)
}

func (s *fileLongTermMemoryStore) DeleteEntries(ctx context.Context, ids []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := s.LoadAll(ctx)
	if err != nil {
		return fmt.Errorf("load existing memory store before delete: %w", err)
	}
	deleteSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		deleteSet[id] = struct{}{}
	}
	remaining := make([]*MemoryEntry, 0, len(existing))
	for _, e := range existing {
		if _, deleted := deleteSet[e.ID]; !deleted {
			remaining = append(remaining, e)
		}
	}
	return s.writeAll(remaining)
}

func (s *fileLongTermMemoryStore) writeAll(entries []*MemoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func loadLongTermMemoryStore() LongTermMemoryStore {
	backend, _ := g.Cfg().Get(context.Background(), "memory.store_backend")
	switch strings.TrimSpace(backend.String()) {
	case "redis":
		return NewRedisMemoryStore(g.Redis())
	case "file":
		path, _ := g.Cfg().Get(context.Background(), "memory.long_term_store_path")
		if p := strings.TrimSpace(path.String()); p != "" {
			return NewFileLongTermMemoryStore(p)
		}
		g.Log().Warning(context.Background(), "[ltm] memory.store_backend=file requires memory.long_term_store_path")
		return nil
	default:
		path, _ := g.Cfg().Get(context.Background(), "memory.long_term_store_path")
		if p := strings.TrimSpace(path.String()); p != "" {
			return NewFileLongTermMemoryStore(p)
		}
		return nil
	}
}
