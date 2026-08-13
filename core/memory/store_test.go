package memory

import "testing"

func TestStore_SetGet(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	if got != "v" {
		t.Errorf("got %q, want v", got)
	}
}

func TestStore_GetMissingReturnsEmpty(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	got, err := s.Get("nope")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestStore_SetOverwrites(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	_ = s.Set("k", "v1")
	_ = s.Set("k", "v2")
	got, _ := s.Get("k")
	if got != "v2" {
		t.Errorf("got %q, want v2", got)
	}
}

func TestStore_Delete(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	_ = s.Set("k", "v")
	if err := s.Delete("k"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("k")
	if got != "" {
		t.Errorf("expected empty after delete, got %q", got)
	}
}

func TestStore_ListByPrefix(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	_ = s.Set("a/1", "1")
	_ = s.Set("a/2", "2")
	_ = s.Set("b/1", "3")

	got, err := s.List("a/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}
	if got["a/1"] != "1" || got["a/2"] != "2" {
		t.Errorf("wrong values: %+v", got)
	}
}

func TestStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_ = s.Set("persist", "yes")
	s.Close()

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, _ := s2.Get("persist")
	if got != "yes" {
		t.Errorf("expected persisted value, got %q", got)
	}
}
