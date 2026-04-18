package agent

import (
	"testing"

	"github.com/carlosmaranje/goclaw/internal/llm"
)

func TestSessionStore_AppendAndSnapshot(t *testing.T) {
	s := NewSessionStore()
	s.Append(llm.Message{Role: "user", Content: "hi"})
	s.Append(llm.Message{Role: "assistant", Content: "hello"})

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(snap))
	}
	if snap[0].Content != "hi" || snap[1].Content != "hello" {
		t.Errorf("unexpected messages: %+v", snap)
	}
}

func TestSessionStore_SnapshotReturnsCopy(t *testing.T) {
	s := NewSessionStore()
	s.Append(llm.Message{Role: "user", Content: "orig"})

	snap := s.Snapshot()
	snap[0].Content = "mutated"

	again := s.Snapshot()
	if again[0].Content != "orig" {
		t.Errorf("snapshot mutation leaked: %q", again[0].Content)
	}
}
