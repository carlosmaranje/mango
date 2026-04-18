package agent

import (
	"slices"
	"sync"

	"github.com/carlosmaranje/mango/internal/llm"
	"github.com/carlosmaranje/mango/internal/memory"
)

type SessionStore struct {
	mu      sync.RWMutex
	history []llm.Message
}

func NewSessionStore() *SessionStore {
	return &SessionStore{}
}

func (s *SessionStore) Append(m llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, m)
}

func (s *SessionStore) Snapshot() []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]llm.Message, len(s.history))
	copy(out, s.history)
	return out
}

type Agent struct {
	Name         string
	WorkDir      string
	LLM          llm.Client
	Skills       []string
	SystemPrompt string
	Memory       memory.Store
	Session      *SessionStore
	AuthCreds    map[string]string
}

func (a *Agent) HasSkill(name string) bool {
	return slices.Contains(a.Skills, name)
}
