package agent

import (
	"sync"

	"github.com/carlosmaranje/goclaw/internal/llm"
	"github.com/carlosmaranje/goclaw/internal/memory"
	"github.com/carlosmaranje/goclaw/internal/tools"
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
	Model        string
	Role         string
	Capabilities []string
	LLM          llm.Client
	Tools        []tools.Tool
	Memory       memory.Store
	Session      *SessionStore
	AuthCreds    map[string]string
	SystemPrompt string
}

func (a *Agent) HasCapability(cap string) bool {
	for _, c := range a.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}
