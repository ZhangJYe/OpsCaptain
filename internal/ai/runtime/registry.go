package runtime

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	agents map[string]Agent
}

func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]Agent)}
}

func (r *Registry) Register(agent Agent) error {
	if agent == nil {
		return fmt.Errorf("agent is nil")
	}
	name := agent.Name()
	if name == "" {
		return fmt.Errorf("agent name is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[name]; exists {
		return fmt.Errorf("agent %q already registered", name)
	}
	r.agents[name] = agent
	return nil
}

func (r *Registry) Get(name string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agents[name]
	return agent, ok
}
