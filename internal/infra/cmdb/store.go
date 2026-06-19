package cmdb

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type YAMLLoader struct {
	path     string
	mu       sync.RWMutex
	services []cmdbServiceDTO
	index    map[string]int
	reverseDeps map[string][]string
}

func NewYAMLLoader(path string) (*YAMLLoader, error) {
	l := &YAMLLoader{
		path: path,
	}
	if err := l.load(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *YAMLLoader) load() error {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return fmt.Errorf("read cmdb file: %w", err)
	}

	var file cmdbFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse cmdb yaml: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.services = file.Services
	l.index = make(map[string]int, len(l.services))
	for i, svc := range l.services {
		l.index[svc.Name] = i
	}

	l.reverseDeps = make(map[string][]string)
	for _, svc := range l.services {
		for _, dep := range svc.Dependencies {
			l.reverseDeps[dep] = append(l.reverseDeps[dep], svc.Name)
		}
	}
	return nil
}

func (l *YAMLLoader) Reload() error {
	return l.load()
}

func (l *YAMLLoader) GetService(name string) (cmdbServiceDTO, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	idx, ok := l.index[name]
	if !ok {
		return cmdbServiceDTO{}, false
	}
	return l.services[idx], true
}

func (l *YAMLLoader) ListAll() []cmdbServiceDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]cmdbServiceDTO, len(l.services))
	copy(result, l.services)
	return result
}

func (l *YAMLLoader) SearchServices(keyword string, limit int) []cmdbServiceDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	keyword = strings.ToLower(keyword)
	var results []cmdbServiceDTO
	for _, svc := range l.services {
		if strings.Contains(strings.ToLower(svc.Name), keyword) ||
			strings.Contains(strings.ToLower(svc.DisplayName), keyword) ||
			strings.Contains(strings.ToLower(svc.Description), keyword) {
			results = append(results, svc)
			if limit > 0 && len(results) >= limit {
				return results
			}
			continue
		}
		for _, tag := range svc.Tags {
			if strings.Contains(strings.ToLower(tag), keyword) {
				results = append(results, svc)
				if limit > 0 && len(results) >= limit {
					return results
				}
				break
			}
		}
	}
	return results
}

func (l *YAMLLoader) ListServicesByCluster(cluster string) []cmdbServiceDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []cmdbServiceDTO
	for _, svc := range l.services {
		if svc.Cluster == cluster {
			results = append(results, svc)
		}
	}
	return results
}

func (l *YAMLLoader) ListServicesByTeam(team string) []cmdbServiceDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []cmdbServiceDTO
	for _, svc := range l.services {
		if svc.Team == team {
			results = append(results, svc)
		}
	}
	return results
}

func (l *YAMLLoader) GetDependents(name string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.reverseDeps[name]
}
