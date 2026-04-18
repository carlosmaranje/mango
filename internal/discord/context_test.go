package discord

import (
	"testing"

	"github.com/carlosmaranje/goclaw/internal/llm"
)

func TestChannelHistory_DefaultSize(t *testing.T) {
	h := NewChannelHistory(0)
	if h.size != DefaultHistorySize {
		t.Errorf("expected default %d, got %d", DefaultHistorySize, h.size)
	}
	h2 := NewChannelHistory(-5)
	if h2.size != DefaultHistorySize {
		t.Errorf("negative size should fall back to default, got %d", h2.size)
	}
}

func TestChannelHistory_AppendAndGet(t *testing.T) {
	h := NewChannelHistory(10)
	h.Append("c", llm.Message{Role: "user", Content: "a"})
	h.Append("c", llm.Message{Role: "assistant", Content: "b"})

	got := h.Get("c")
	if len(got) != 2 || got[0].Content != "a" || got[1].Content != "b" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestChannelHistory_RingBuffer(t *testing.T) {
	h := NewChannelHistory(3)
	for _, c := range []string{"1", "2", "3", "4", "5"} {
		h.Append("c", llm.Message{Role: "user", Content: c})
	}
	got := h.Get("c")
	if len(got) != 3 {
		t.Fatalf("expected cap=3, got %d", len(got))
	}
	if got[0].Content != "3" || got[2].Content != "5" {
		t.Errorf("wrong sliding window: %+v", got)
	}
}

func TestChannelHistory_ChannelsIsolated(t *testing.T) {
	h := NewChannelHistory(5)
	h.Append("a", llm.Message{Role: "user", Content: "A"})
	h.Append("b", llm.Message{Role: "user", Content: "B"})

	if got := h.Get("a"); len(got) != 1 || got[0].Content != "A" {
		t.Errorf("channel a bleed: %+v", got)
	}
	if got := h.Get("b"); len(got) != 1 || got[0].Content != "B" {
		t.Errorf("channel b bleed: %+v", got)
	}
}

func TestChannelHistory_GetReturnsCopy(t *testing.T) {
	h := NewChannelHistory(5)
	h.Append("c", llm.Message{Role: "user", Content: "orig"})

	got := h.Get("c")
	got[0].Content = "mutated"

	again := h.Get("c")
	if again[0].Content != "orig" {
		t.Error("Get() did not return a copy")
	}
}

func TestChannelHistory_DefaultSizeIs100(t *testing.T) {
	if DefaultHistorySize != 100 {
		t.Errorf("DefaultHistorySize = %d, want 100", DefaultHistorySize)
	}
}
