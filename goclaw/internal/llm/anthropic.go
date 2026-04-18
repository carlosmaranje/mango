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

const (
	anthropicDefaultBaseURL = "https://api.anthropic.com"
	anthropicAPIVersion     = "2023-06-01"
)

type AnthropicClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewAnthropicClient(cfg ProviderConfig) *AnthropicClient {
	base := cfg.BaseURL
	if base == "" {
		base = anthropicDefaultBaseURL
	}
	return &AnthropicClient{
		apiKey:  cfg.APIKey,
		baseURL: base,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	Content []anthropicResponseContent `json:"content"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	var system string
	msgs := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
			continue
		}
		msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	body, err := json.Marshal(anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  msgs,
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode anthropic response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}
	if len(parsed.Content) == 0 {
		return "", nil
	}
	return parsed.Content[0].Text, nil
}
