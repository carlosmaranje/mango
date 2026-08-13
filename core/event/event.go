// Package event is a leaf package holding the engine-wide event bus and event
// vocabulary. It lives below both core/agent and core/orchestrator so either can
// emit onto the same bus without an import cycle (orchestrator imports agent, so
// the bus cannot live in orchestrator if the agent runner must also emit).
package event

import (
	"sync"
	"sync/atomic"
)

// Event type strings. Task- and agent-level events are emitted by the
// dispatcher/engine; step- and tool-level events are emitted by the agent runner
// (only when a bus is attached — see core.Options.EmitStepEvents).
const (
	TypeTaskCreated  = "task.created"
	TypeTaskUpdated  = "task.updated"
	TypeAgentStarted = "agent.started"
	TypeAgentStopped = "agent.stopped"

	TypeStepStarted       = "step.started"
	TypeStepCompleted     = "step.completed"
	TypeToolCalled        = "tool.called"
	TypeToolCompleted     = "tool.completed"
	TypeInvocationStopped = "invocation.stopped"
)

// Event is a lifecycle event. Payload carries a type-specific value (e.g. a
// *orchestrator.Task for task.*, a StepInfo for step.*, a ToolInfo for tool.*).
type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// StepInfo is the payload for step.started / step.completed / invocation.stopped.
type StepInfo struct {
	AgentName  string `json:"agent_name"`
	TaskID     string `json:"task_id,omitempty"`
	Step       int    `json:"step"`
	StopReason string `json:"stop_reason,omitempty"`
}

// ToolInfo is the payload for tool.called / tool.completed.
type ToolInfo struct {
	AgentName string `json:"agent_name"`
	TaskID    string `json:"task_id,omitempty"`
	Tool      string `json:"tool"`
	Step      int    `json:"step"`
	Attempt   int    `json:"attempt,omitempty"`
	Err       string `json:"error,omitempty"`
}

// Bus is a fan-out pub/sub channel. Emit is non-blocking: an event is dropped for
// any subscriber whose buffer is full.
type Bus struct {
	mu   sync.RWMutex
	subs map[uint64]chan Event
	seq  atomic.Uint64
}

func NewBus() *Bus {
	return &Bus{subs: make(map[uint64]chan Event)}
}

// Subscribe registers a new subscriber and returns its ID and receive-only channel.
func (b *Bus) Subscribe() (uint64, <-chan Event) {
	ch := make(chan Event, 64)
	id := b.seq.Add(1)
	b.mu.Lock()
	b.subs[id] = ch
	b.mu.Unlock()
	return id, ch
}

// Unsubscribe removes a subscriber by ID.
func (b *Bus) Unsubscribe(id uint64) {
	b.mu.Lock()
	delete(b.subs, id)
	b.mu.Unlock()
}

// Emit broadcasts an event to all subscribers. Drops the event for any subscriber
// whose buffer is full. A nil bus is a no-op, so callers can hold an optional bus.
func (b *Bus) Emit(e Event) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
