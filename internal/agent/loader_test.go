package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosmaranje/mango/internal/skill"
)

func TestLoadDefinition_ReadsUppercaseMD(t *testing.T) {
	dir := t.TempDir()
	body := "Researcher persona"
	if err := os.WriteFile(filepath.Join(dir, "RESEARCHER.md"), []byte(body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadDefinition(dir, "researcher")
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if got != body {
		t.Errorf("got %q, want %q", got, body)
	}
}

func TestLoadDefinition_MissingReturnsDescriptiveError(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadDefinition(dir, "orchestrator")
	if err == nil {
		t.Fatal("expected error for missing definition")
	}
	msg := err.Error()
	for _, want := range []string{`"orchestrator"`, dir, "ORCHESTRATOR.md"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestLoadDefinition_EmptyFileIsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "X.md"), []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDefinition(dir, "x"); err == nil {
		t.Fatal("expected error for empty definition")
	}
}

func TestComposeSystemPrompt_AppendsSkillsInOrder(t *testing.T) {
	agentsDir := t.TempDir()
	skillsDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(agentsDir, "CODER.md"), []byte("Coder base"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "web_search.md"), []byte("SKILL:web"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "code_execution.md"), []byte("SKILL:code"), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := skill.NewLoader(skillsDir)
	got, err := ComposeSystemPrompt(agentsDir, "coder", []string{"code_execution", "web_search"}, loader)
	if err != nil {
		t.Fatalf("ComposeSystemPrompt: %v", err)
	}
	want := "Coder base" + PromptSeparator + "SKILL:code" + PromptSeparator + "SKILL:web"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestComposeSystemPrompt_MissingSkillBubblesError(t *testing.T) {
	agentsDir := t.TempDir()
	skillsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(agentsDir, "A.md"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ComposeSystemPrompt(agentsDir, "a", []string{"ghost"}, skill.NewLoader(skillsDir))
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
	if !strings.Contains(err.Error(), `skill "ghost" not found`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveAgentsDir(t *testing.T) {
	t.Setenv("MANGO_AGENTS_DIR", "/env/agents")
	if got := ResolveAgentsDir("/explicit"); got != "/explicit" {
		t.Errorf("explicit precedence: got %q", got)
	}
	if got := ResolveAgentsDir(""); got != "/env/agents" {
		t.Errorf("env fallback: got %q", got)
	}
	t.Setenv("MANGO_AGENTS_DIR", "")
	if got := ResolveAgentsDir(""); got != DefaultAgentsDir {
		t.Errorf("default fallback: got %q", got)
	}
}
