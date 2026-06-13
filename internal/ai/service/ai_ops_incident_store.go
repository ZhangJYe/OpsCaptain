package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type FileIncidentStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileIncidentStore(dir string) (*FileIncidentStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("incident store dir is empty")
	}
	return &FileIncidentStore{dir: dir}, nil
}

func (s *FileIncidentStore) Create(_ context.Context, incident *IncidentSession) error {
	if incident == nil || strings.TrimSpace(incident.IncidentID) == "" {
		return fmt.Errorf("incident id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	path := s.path(incident.IncidentID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("incident %s already exists", incident.IncidentID)
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeIncidentJSON(path, incident)
}

func (s *FileIncidentStore) Get(_ context.Context, incidentID string) (*IncidentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(incidentID)
}

func (s *FileIncidentStore) List(_ context.Context) ([]IncidentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	items := make([]IncidentSession, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		incident, err := s.read(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			g.Log().Warningf(context.Background(), "[incident] read file %s failed: %v", entry.Name(), err)
			continue
		}
		items = append(items, *incident)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return items, nil
}

func (s *FileIncidentStore) Update(_ context.Context, incidentID string, update func(*IncidentSession) error) (*IncidentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	incident, err := s.read(incidentID)
	if err != nil {
		return nil, err
	}
	if update != nil {
		if err := update(incident); err != nil {
			return nil, err
		}
	}
	incident.UpdatedAt = time.Now().UnixMilli()
	if err := writeIncidentJSON(s.path(incidentID), incident); err != nil {
		return nil, err
	}
	return incident, nil
}

func (s *FileIncidentStore) read(incidentID string) (*IncidentSession, error) {
	data, err := os.ReadFile(s.path(incidentID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrIncidentNotFound
		}
		return nil, err
	}
	var incident IncidentSession
	if err := json.Unmarshal(data, &incident); err != nil {
		return nil, err
	}
	return &incident, nil
}

func (s *FileIncidentStore) path(incidentID string) string {
	return filepath.Join(s.dir, strings.TrimSpace(incidentID)+".json")
}

func getOrCreateIncidentStore(ctx context.Context) (IncidentStore, error) {
	dir := incidentStoreDir(ctx)
	incidentStoreMu.Lock()
	defer incidentStoreMu.Unlock()
	if store, ok := incidentStores[dir]; ok {
		return store, nil
	}
	store, err := newIncidentStore(dir)
	if err != nil {
		return nil, err
	}
	incidentStores[dir] = store
	return store, nil
}

func writeIncidentJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
