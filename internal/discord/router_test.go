package discord

import "testing"

func TestRouter_ResolveKnown(t *testing.T) {
	r := NewRouter([]ChannelBinding{{ChannelID: "c1", AgentName: "a1"}})
	if got := r.Resolve("c1"); got != "a1" {
		t.Errorf("expected a1, got %q", got)
	}
}

func TestRouter_ResolveUnknown(t *testing.T) {
	r := NewRouter(nil)
	if got := r.Resolve("missing"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRouter_BindOverwrites(t *testing.T) {
	r := NewRouter([]ChannelBinding{{ChannelID: "c1", AgentName: "a1"}})
	r.Bind("c1", "a2")
	if got := r.Resolve("c1"); got != "a2" {
		t.Errorf("bind should overwrite, got %q", got)
	}
}

func TestRouter_BindingsReturnsCopy(t *testing.T) {
	r := NewRouter([]ChannelBinding{{ChannelID: "c1", AgentName: "a1"}})
	b := r.Bindings()
	b["c1"] = "mutated"
	if got := r.Resolve("c1"); got != "a1" {
		t.Errorf("Bindings() did not return a copy; got %q", got)
	}
}
