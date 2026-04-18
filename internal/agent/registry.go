package agent

import (
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	agents map[string]*Agent
}

func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]*Agent)}
}

func (r *Registry) Register(a *Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[a.Name]; exists {
		return fmt.Errorf("agent %q already registered", a.Name)
	}
	r.agents[a.Name] = a
	return nil
}

func (r *Registry) Get(name string) (*Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[name]
	return a, ok
}

func (r *Registry) List() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Agent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) FindBySkill(skill string) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Agent
	for _, a := range r.agents {
		if a.HasSkill(skill) {
			out = append(out, a)
		}
	}
	return out
}
