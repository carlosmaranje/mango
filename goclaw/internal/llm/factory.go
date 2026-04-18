package llm

import "fmt"

type ProviderConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
}

func NewClient(cfg ProviderConfig) (Client, error) {
	switch cfg.Provider {
	case "anthropic":
		return NewAnthropicClient(cfg), nil
	case "ollama":
		if cfg.BaseURL == "" {
			cfg.BaseURL = OllamaDefaultBaseURL
		}
		if cfg.APIKey == "" {
			cfg.APIKey = "ollama"
		}
		return NewOpenAICompatClient(cfg), nil
	case "openai", "openai-compatible":
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("provider %q requires base_url", cfg.Provider)
		}
		return NewOpenAICompatClient(cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}
