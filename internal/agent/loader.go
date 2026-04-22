package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlosmaranje/mango/internal/skill"
)

const (
	DefaultAgentsDir = "/etc/mango/agents"
	PromptSeparator  = "\n\n---\n\n"
)

// ResolveAgentsDir returns the explicit path when non-empty, otherwise the
// value of MANGO_AGENTS_DIR, otherwise DefaultAgentsDir.
func ResolveAgentsDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv("MANGO_AGENTS_DIR"); env != "" {
		return env
	}
	return defaultAgentsDir()
}

func defaultAgentsDir() string {
	if os.Getenv("MANGO_AGENTS_DIR") != "" {
		return os.Getenv("MANGO_AGENTS_DIR")
	}
	// Try to find it relative to config dir on Windows
	if os.Getenv("APPDATA") != "" {
		return filepath.Join(os.Getenv("APPDATA"), "mango", "agents")
	}
	return "/etc/mango/agents"
}

// AgentDefinitionPath returns the canonical path to an agent's .md file
// (NAME.md, uppercased) under the given agents directory.
func AgentDefinitionPath(agentsDir, name string) string {
	return filepath.Join(agentsDir, strings.ToUpper(name)+".md")
}

// LoadDefinition reads the user-defined .md file for an agent and returns its
// trimmed contents. Returns a descriptive error when the file is missing or
// empty — an agent with no persona should never silently run.
func LoadDefinition(agentsDir, name string) (string, error) {
	path := AgentDefinitionPath(agentsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("agent definition for %q not found in %s (expected %s.md)",
				name, agentsDir, strings.ToUpper(name))
		}
		return "", fmt.Errorf("read agent definition %q: %w", path, err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", fmt.Errorf("agent definition %s is empty", path)
	}
	return content, nil
}

// ComposeSystemPrompt assembles an agent's effective system prompt from its
// base .md definition plus the listed skills, in order, separated by
// PromptSeparator. Skills are loaded via the supplied loader.
func ComposeSystemPrompt(agentsDir string, name string, skills []string, skillLoader *skill.Loader) (string, error) {
	base, err := LoadDefinition(agentsDir, name)
	if err != nil {
		return "", err
	}
	parts := []string{base}
	for _, skillName := range skills {
		if skillLoader == nil {
			return "", fmt.Errorf("agent %q lists skill %q but no skill loader was configured", name, skillName)
		}
		sk, err := skillLoader.Load(skillName)
		if err != nil {
			return "", fmt.Errorf("agent %q: %w", name, err)
		}
		if sk.Content != "" {
			parts = append(parts, sk.Content)
		}
	}
	return strings.Join(parts, PromptSeparator), nil
}
