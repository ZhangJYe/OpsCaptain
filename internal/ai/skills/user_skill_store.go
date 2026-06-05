package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// UserSkillStore provides persistence for user-created tools and skills.
type UserSkillStore interface {
	Load(ctx context.Context) (*UserRegistryData, error)
	Save(ctx context.Context, data *UserRegistryData) error
}

// fileUserSkillStore implements UserSkillStore using a local JSON file
// with atomic writes (write to .tmp then rename).
type fileUserSkillStore struct {
	mu   sync.Mutex
	path string
}

// NewFileUserSkillStore creates a file-backed UserSkillStore.
// path is the absolute path to the JSON file (e.g. /data/user_skills.json).
func NewFileUserSkillStore(path string) UserSkillStore {
	return &fileUserSkillStore{path: path}
}

// Load reads the registry data from disk. Returns empty data if the file
// does not exist. Creates the parent directory if needed.
func (s *fileUserSkillStore) Load(ctx context.Context) (*UserRegistryData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create registry dir: %w", err)
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &UserRegistryData{}, nil
		}
		return nil, fmt.Errorf("read registry file: %w", err)
	}

	var data UserRegistryData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("unmarshal registry: %w", err)
	}
	return &data, nil
}

// Save writes the registry data to disk atomically: marshal to a .tmp file,
// then rename over the target path.
func (s *fileUserSkillStore) Save(ctx context.Context, data *UserRegistryData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return fmt.Errorf("write tmp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename tmp file: %w", err)
	}

	return nil
}
