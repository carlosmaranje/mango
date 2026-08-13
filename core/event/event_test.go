package event

import (
	"testing"
	"time"
)

func TestBus_FanOut(t *testing.T) {
	b := NewBus()
	_, ch1 := b.Subscribe()
	_, ch2 := b.Subscribe()

	b.Emit(Event{Type: TypeTaskCreated})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Type != TypeTaskCreated {
				t.Errorf("subscriber %d got %q", i, e.Type)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber %d received no event", i)
		}
	}
}

func TestBus_Unsubscribe(t *testing.T) {
	b := NewBus()
	id, ch := b.Subscribe()
	b.Unsubscribe(id)
	b.Emit(Event{Type: TypeTaskCreated})

	select {
	case e, ok := <-ch:
		if ok {
			t.Errorf("expected no event after unsubscribe, got %q", e.Type)
		}
	default:
	}
}

func TestBus_DropsWhenFull(t *testing.T) {
	b := NewBus()
	_, ch := b.Subscribe() // buffered to 64, never drained here

	done := make(chan struct{})
	go func() {
		for range 200 {
			b.Emit(Event{Type: TypeStepStarted})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked when a subscriber's buffer was full")
	}

	count := 0
	for {
		select {
		case <-ch:
			count++
			continue
		default:
		}
		break
	}
	if count == 0 || count > 64 {
		t.Errorf("buffered events = %d, want 1..64 (drop-on-full)", count)
	}
}

func TestBus_NilEmitIsNoop(t *testing.T) {
	var b *Bus
	b.Emit(Event{Type: TypeTaskCreated}) // must not panic
}
