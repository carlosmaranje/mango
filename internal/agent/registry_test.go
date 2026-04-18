package agent

import "testing"

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	a := &Agent{Name: "alpha"}
	if err := r.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, ok := r.Get("alpha")
	if !ok || got != a {
		t.Errorf("Get(alpha): got %v ok=%v, want %v", got, ok, a)
	}
	if _, ok := r.Get("missing"); ok {
		t.Error("Get(missing) should return ok=false")
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Agent{Name: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&Agent{Name: "alpha"}); err == nil {
		t.Fatal("duplicate register should error")
	}
}

func TestRegistry_ListSorted(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Agent{Name: "zeta"})
	_ = r.Register(&Agent{Name: "alpha"})
	_ = r.Register(&Agent{Name: "mango"})

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].Name != "alpha" || list[1].Name != "mango" || list[2].Name != "zeta" {
		t.Errorf("not sorted: %s, %s, %s", list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestRegistry_FindByCapability(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Agent{Name: "a", Capabilities: []string{"search"}})
	_ = r.Register(&Agent{Name: "b", Capabilities: []string{"code"}})
	_ = r.Register(&Agent{Name: "c", Capabilities: []string{"search", "code"}})

	if got := r.FindByCapability("search"); len(got) != 2 {
		t.Errorf("search: expected 2, got %d", len(got))
	}
	if got := r.FindByCapability("nope"); len(got) != 0 {
		t.Errorf("nope: expected 0, got %d", len(got))
	}
}

func TestRegistry_FindByRole(t *testing.T) {
	r := NewRegistry()
	o := &Agent{Name: "o", Role: "orchestrator"}
	_ = r.Register(o)
	_ = r.Register(&Agent{Name: "w"})

	if got := r.FindByRole("orchestrator"); got != o {
		t.Errorf("expected orchestrator %p, got %p", o, got)
	}
	if got := r.FindByRole("nonexistent"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestAgent_HasCapability(t *testing.T) {
	a := &Agent{Capabilities: []string{"search", "code"}}
	if !a.HasCapability("search") {
		t.Error("expected true for 'search'")
	}
	if a.HasCapability("absent") {
		t.Error("expected false for 'absent'")
	}
}
