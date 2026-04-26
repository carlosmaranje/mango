package agent

import (
	"slices"

	"github.com/carlosmaranje/mango/internal/llm"
	"github.com/carlosmaranje/mango/internal/memory"
)

const DefaultMaxTokens = 4096

type Agent struct {
	Name         string
	WorkDir      string
	LLM          llm.Client
	Skills       []string
	SystemPrompt string
	Memory       memory.Store
	AuthCreds    map[string]string
	MaxTokens    int
}

func (a *Agent) EffectiveMaxTokens() int {
	if a.MaxTokens > 0 {
		return a.MaxTokens
	}
	return DefaultMaxTokens
}

func (a *Agent) HasSkill(name string) bool {
	return slices.Contains(a.Skills, name)
}
