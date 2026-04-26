package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlosmaranje/mango/internal/constants"
)

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

// ResolveSkillsDir returns the explicit path when non-empty, otherwise
// MANGO_DIR/skills (see constants.MangoDir).
func ResolveSkillsDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return filepath.Join(constants.MangoDir(), "skills")
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
