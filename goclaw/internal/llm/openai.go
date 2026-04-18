package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OpenAICompatClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewOpenAICompatClient(cfg ProviderConfig) *OpenAICompatClient {
	return &OpenAICompatClient{
		apiKey:  cfg.APIKey,
		baseURL: cfg.BaseURL,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

type openaiRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

type openaiChoice struct {
	Message Message `json:"message"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAICompatClient) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	body, err := json.Marshal(openaiRequest{
		Model:     req.Model,
		Messages:  req.Messages,
		MaxTokens: req.MaxTokens,
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("openai-compatible request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai-compatible status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed openaiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode openai-compatible response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("openai-compatible error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", nil
	}
	return parsed.Choices[0].Message.Content, nil
}
