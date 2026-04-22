package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultSkillsDir = "/etc/mango/skills"

type Skill struct {
	Name    string
	Content string
}

type Loader struct {
	Dir string
}

func NewLoader(dir string) *Loader {
	if dir == "" {
		dir = ResolveSkillsDir("")
	}
	return &Loader{Dir: dir}
}

// ResolveSkillsDir returns the explicit path when non-empty, otherwise the
// value of MANGO_SKILLS_DIR, otherwise DefaultSkillsDir.
func ResolveSkillsDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv("MANGO_SKILLS_DIR"); env != "" {
		return env
	}
	return defaultSkillsDir()
}

func defaultSkillsDir() string {
	if os.Getenv("MANGO_SKILLS_DIR") != "" {
		return os.Getenv("MANGO_SKILLS_DIR")
	}
	// Try to find it relative to config dir on Windows
	if os.Getenv("APPDATA") != "" {
		return filepath.Join(os.Getenv("APPDATA"), "mango", "skills")
	}
	return "/etc/mango/skills"
}

func (l *Loader) Load(name string) (*Skill, error) {
	path := filepath.Join(l.Dir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill %q not found in %s", name, l.Dir)
		}
		return nil, fmt.Errorf("read skill %q: %w", name, err)
	}
	return &Skill{
		Name:    name,
		Content: strings.TrimSpace(string(data)),
	}, nil
}
