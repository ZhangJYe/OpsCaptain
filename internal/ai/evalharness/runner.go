package evalharness

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type Adapter interface {
	Name() SuiteName
	PayloadSchema() string
	Validate(SuiteConfig, DatasetRole, Profile) error
	RunCase(context.Context, CaseEnvelope) CaseResult
	Aggregate([]CaseResult) (domainSchema string, domainMetrics json.RawMessage, gates []GateResult, err error)
}

type SchemaAwareAdapter interface {
	SupportedPayloadSchemas() []string
}

type Registry struct {
	mu       sync.RWMutex
	adapters map[SuiteName]Adapter
}

func RejectLiveProfile(profile Profile) error {
	if profile == ProfileLive {
		return fmt.Errorf("live profile requires an explicitly authorized live adapter")
	}
	return nil
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[SuiteName]Adapter)}
}

func (r *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return fmt.Errorf("adapter is nil")
	}
	if !validSuite(adapter.Name()) {
		return fmt.Errorf("unsupported adapter suite %q", adapter.Name())
	}
	if adapter.PayloadSchema() == "" {
		return fmt.Errorf("adapter %q payload schema is required", adapter.Name())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.adapters[adapter.Name()]; ok {
		return fmt.Errorf("adapter %q already registered", adapter.Name())
	}
	r.adapters[adapter.Name()] = adapter
	return nil
}

func (r *Registry) Get(name SuiteName) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[name]
	return adapter, ok
}
