package discord

import "sync"

type ChannelBinding struct {
	ChannelID string
	AgentName string
}

type Router struct {
	mu       sync.RWMutex
	bindings map[string]string
}

func NewRouter(initial []ChannelBinding) *Router {
	r := &Router{bindings: make(map[string]string)}
	for _, b := range initial {
		r.bindings[b.ChannelID] = b.AgentName
	}
	return r
}

func (r *Router) Resolve(channelID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bindings[channelID]
}

func (r *Router) Bind(channelID, agentName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bindings[channelID] = agentName
}
