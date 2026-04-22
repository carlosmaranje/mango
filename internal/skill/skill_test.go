package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_LoadReadsFile(t *testing.T) {
	dir := t.TempDir()
	body := "# Web Search\n\nUse the web when local context is insufficient.\n"
	if err := os.WriteFile(filepath.Join(dir, "web_search.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	sk, err := NewLoader(dir).Load("web_search")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sk.Name != "web_search" {
		t.Errorf("Name: got %q", sk.Name)
	}
	if !strings.Contains(sk.Content, "Use the web") {
		t.Errorf("Content missing expected body: %q", sk.Content)
	}
}

func TestLoader_MissingSkillReturnsDescriptiveError(t *testing.T) {
	dir := t.TempDir()
	_, err := NewLoader(dir).Load("nope")
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
	msg := err.Error()
	if !strings.Contains(msg, `skill "nope" not found`) || !strings.Contains(msg, dir) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveSkillsDir_PrefersExplicit(t *testing.T) {
	t.Setenv("MANGO_SKILLS_DIR", "/env/path")
	if got := ResolveSkillsDir("/explicit"); got != "/explicit" {
		t.Errorf("got %q, want /explicit", got)
	}
}

func TestResolveSkillsDir_UsesEnv(t *testing.T) {
	t.Setenv("MANGO_SKILLS_DIR", "/env/path")
	if got := ResolveSkillsDir(""); got != "/env/path" {
		t.Errorf("got %q, want /env/path", got)
	}
}

func TestResolveSkillsDir_Default(t *testing.T) {
	t.Setenv("MANGO_SKILLS_DIR", "")
	expected := DefaultSkillsDir
	if os.Getenv("APPDATA") != "" {
		expected = filepath.Join(os.Getenv("APPDATA"), "mango", "skills")
	}
	if got := ResolveSkillsDir(""); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}
