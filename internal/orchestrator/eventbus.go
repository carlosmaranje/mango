package orchestrator

import (
	"sync"
	"sync/atomic"
)

// Event is a lifecycle event emitted by the dispatcher.
type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// EventBus is a fan-out pub/sub channel for gateway events.
type EventBus struct {
	mu   sync.RWMutex
	subs map[uint64]chan Event
	seq  atomic.Uint64
}

func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[uint64]chan Event)}
}

// Subscribe registers a new subscriber and returns its ID and receive-only channel.
func (b *EventBus) Subscribe() (uint64, <-chan Event) {
	ch := make(chan Event, 64)
	id := b.seq.Add(1)
	b.mu.Lock()
	b.subs[id] = ch
	b.mu.Unlock()
	return id, ch
}

// Unsubscribe removes a subscriber by ID.
func (b *EventBus) Unsubscribe(id uint64) {
	b.mu.Lock()
	delete(b.subs, id)
	b.mu.Unlock()
}

// Emit broadcasts an event to all subscribers. Drops the event if a subscriber's buffer is full.
func (b *EventBus) Emit(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
