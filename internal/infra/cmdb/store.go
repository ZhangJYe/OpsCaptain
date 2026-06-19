package cmdb

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type YAMLLoader struct {
	path        string
	mu          sync.RWMutex
	writeMu     sync.Mutex
	services    []CMDBServiceDTO
	index       map[string]int
	reverseDeps map[string][]string
	hosts       []HostDTO
	hostIndex   map[string]int
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

	l.hosts = file.Hosts
	l.hostIndex = make(map[string]int, len(l.hosts))
	for i, h := range l.hosts {
		l.hostIndex[h.Name] = i
	}
	return nil
}

func (l *YAMLLoader) Reload() error {
	return l.load()
}

func (l *YAMLLoader) GetService(name string) (CMDBServiceDTO, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	idx, ok := l.index[name]
	if !ok {
		return CMDBServiceDTO{}, false
	}
	return l.services[idx], true
}

func (l *YAMLLoader) ListAll() []CMDBServiceDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]CMDBServiceDTO, len(l.services))
	copy(result, l.services)
	return result
}

func (l *YAMLLoader) SearchServices(keyword string, limit int) []CMDBServiceDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	keyword = strings.ToLower(keyword)
	var results []CMDBServiceDTO
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

func (l *YAMLLoader) ListServicesByCluster(cluster string) []CMDBServiceDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []CMDBServiceDTO
	for _, svc := range l.services {
		if svc.Cluster == cluster {
			results = append(results, svc)
		}
	}
	return results
}

func (l *YAMLLoader) ListServicesByTeam(team string) []CMDBServiceDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []CMDBServiceDTO
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

func (l *YAMLLoader) save() error {
	data, err := yaml.Marshal(cmdbFile{Services: l.services, Hosts: l.hosts})
	if err != nil {
		return fmt.Errorf("marshal cmdb yaml: %w", err)
	}

	tmpPath := l.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write cmdb tmp: %w", err)
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("open cmdb tmp: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("fsync cmdb: %w", err)
	}
	f.Close()

	if err := os.Rename(tmpPath, l.path); err != nil {
		return fmt.Errorf("rename cmdb: %w", err)
	}
	return nil
}

func (l *YAMLLoader) rebuildIndex() {
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
	l.hostIndex = make(map[string]int, len(l.hosts))
	for i, h := range l.hosts {
		l.hostIndex[h.Name] = i
	}
}

func (l *YAMLLoader) CreateService(svc CMDBServiceDTO) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.index[svc.Name]; exists {
		return fmt.Errorf("service %q already exists", svc.Name)
	}
	l.services = append(l.services, svc)
	l.rebuildIndex()
	return l.save()
}

func (l *YAMLLoader) UpdateService(name string, svc CMDBServiceDTO) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	idx, exists := l.index[name]
	if !exists {
		return fmt.Errorf("service %q not found", name)
	}
	svc.Name = name
	l.services[idx] = svc
	l.rebuildIndex()
	return l.save()
}

func (l *YAMLLoader) DeleteService(name string) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	idx, exists := l.index[name]
	if !exists {
		return fmt.Errorf("service %q not found", name)
	}
	l.services = append(l.services[:idx], l.services[idx+1:]...)
	l.rebuildIndex()
	return l.save()
}

func (l *YAMLLoader) GetHost(name string) (HostDTO, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	idx, ok := l.hostIndex[name]
	if !ok {
		return HostDTO{}, false
	}
	return l.hosts[idx], true
}

func (l *YAMLLoader) ListHostsByService(service string) []HostDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []HostDTO
	for _, h := range l.hosts {
		if h.Service == service {
			results = append(results, h)
		}
	}
	return results
}

func (l *YAMLLoader) ListHostsByCluster(cluster string) []HostDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []HostDTO
	for _, h := range l.hosts {
		if h.Cluster == cluster {
			results = append(results, h)
		}
	}
	return results
}

func (l *YAMLLoader) ListAllHosts() []HostDTO {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]HostDTO, len(l.hosts))
	copy(result, l.hosts)
	return result
}

func (l *YAMLLoader) CreateHost(host HostDTO) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.hostIndex[host.Name]; exists {
		return fmt.Errorf("host %q already exists", host.Name)
	}
	l.hosts = append(l.hosts, host)
	l.rebuildIndex()
	return l.save()
}

func (l *YAMLLoader) UpdateHost(name string, host HostDTO) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	idx, exists := l.hostIndex[name]
	if !exists {
		return fmt.Errorf("host %q not found", name)
	}
	host.Name = name
	l.hosts[idx] = host
	l.rebuildIndex()
	return l.save()
}

func (l *YAMLLoader) DeleteHost(name string) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	idx, exists := l.hostIndex[name]
	if !exists {
		return fmt.Errorf("host %q not found", name)
	}
	l.hosts = append(l.hosts[:idx], l.hosts[idx+1:]...)
	l.rebuildIndex()
	return l.save()
}
