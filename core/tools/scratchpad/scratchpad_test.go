package scratchpad

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/carlosmaranje/mango/core/memory"
)

func newStore(t *testing.T) memory.Store {
	t.Helper()
	s, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func call(t *testing.T, tool interface {
	Execute(context.Context, string) (string, error)
}, input string) result {
	t.Helper()
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute(%s): %v", input, err)
	}
	var r result
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	return r
}

func TestScratchpad_RoundTrip(t *testing.T) {
	sp := New(newStore(t), "worker")

	if r := call(t, sp, `{"op":"set","key":"url","value":"http://x"}`); !r.OK || r.Value != "http://x" {
		t.Fatalf("set: %+v", r)
	}
	if r := call(t, sp, `{"op":"get","key":"url"}`); !r.Found || r.Value != "http://x" {
		t.Fatalf("get: %+v", r)
	}

	call(t, sp, `{"op":"set","key":"count","value":"3"}`)
	r := call(t, sp, `{"op":"list"}`)
	if len(r.Keys) != 2 || r.Keys["url"] != "http://x" || r.Keys["count"] != "3" {
		t.Fatalf("list: %+v", r.Keys)
	}

	call(t, sp, `{"op":"delete","key":"url"}`)
	if r := call(t, sp, `{"op":"get","key":"url"}`); r.Found {
		t.Fatalf("expected url deleted, got %+v", r)
	}
}

func TestScratchpad_NamespaceIsolation(t *testing.T) {
	store := newStore(t)
	a := New(store, "a")
	b := New(store, "b")

	call(t, a, `{"op":"set","key":"k","value":"va"}`)
	call(t, b, `{"op":"set","key":"k","value":"vb"}`)

	if r := call(t, a, `{"op":"get","key":"k"}`); r.Value != "va" {
		t.Errorf("namespace a leaked: %+v", r)
	}
	if r := call(t, a, `{"op":"list"}`); len(r.Keys) != 1 || r.Keys["k"] != "va" {
		t.Errorf("a list should only show a's keys: %+v", r.Keys)
	}
}

func TestScratchpad_LeavesReservedKeysAlone(t *testing.T) {
	store := newStore(t)
	_ = store.Set("heartbeat/last", "now")
	sp := New(store, "worker")
	call(t, sp, `{"op":"set","key":"x","value":"1"}`)

	if r := call(t, sp, `{"op":"list"}`); func() bool { _, ok := r.Keys["heartbeat/last"]; return ok }() {
		t.Error("scratchpad list leaked a reserved key")
	}
	if v, _ := store.Get("heartbeat/last"); v != "now" {
		t.Error("scratchpad clobbered a reserved key")
	}
}

func TestScratchpad_UnknownOp(t *testing.T) {
	sp := New(newStore(t), "worker")
	if _, err := sp.Execute(context.Background(), `{"op":"frobnicate"}`); err == nil {
		t.Error("expected error for unknown op")
	}
}
