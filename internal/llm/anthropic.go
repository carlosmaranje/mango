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
	model   string
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
		model:   cfg.Model,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

// anthropicContentBlock covers text, tool_use (outbound) and tool_result (inbound) blocks.
type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicRequestMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []anthropicContentBlock
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema anthropicSchema `json:"input_schema"`
}

type anthropicSchema struct {
	Type       string                   `json:"type"`
	Properties map[string]anthropicProp `json:"properties,omitempty"`
	Required   []string                 `json:"required,omitempty"`
}

type anthropicProp struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type anthropicRequest struct {
	Model     string                    `json:"model"`
	MaxTokens int                       `json:"max_tokens"`
	System    string                    `json:"system,omitempty"`
	Messages  []anthropicRequestMessage `json:"messages"`
	Tools     []anthropicTool           `json:"tools,omitempty"`
}

type anthropicResponseContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicResponse struct {
	Content    []anthropicResponseContent `json:"content"`
	StopReason string                     `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	system, msgs := buildAnthropicMessages(req.Messages)

	model := req.Model
	if model == "" {
		model = c.model
	}
	body, err := json.Marshal(anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  msgs,
		Tools:     buildAnthropicTools(req.Tools),
	})
	if err != nil {
		return CompletionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CompletionResponse{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return CompletionResponse{}, fmt.Errorf("decode anthropic response: %w", err)
	}
	if parsed.Error != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}

	return parseAnthropicResponse(parsed), nil
}

func buildAnthropicMessages(messages []Message) (system string, out []anthropicRequestMessage) {
	i := 0
	for i < len(messages) {
		m := messages[i]
		switch m.Role {
		case "system":
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
			i++

		case "assistant":
			if len(m.ToolCalls) > 0 {
				blocks := make([]anthropicContentBlock, 0, len(m.ToolCalls)+1)
				if m.Content != "" {
					blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					input := json.RawMessage(`{}`)
					if tc.Input != "" {
						input = json.RawMessage(tc.Input)
					}
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Name,
						Input: input,
					})
				}
				out = append(out, anthropicRequestMessage{Role: "assistant", Content: blocks})
			} else {
				out = append(out, anthropicRequestMessage{Role: "assistant", Content: m.Content})
			}
			i++

		case "tool":
			// Group all consecutive tool-result messages into one user message.
			var blocks []anthropicContentBlock
			for i < len(messages) && messages[i].Role == "tool" {
				blocks = append(blocks, anthropicContentBlock{
					Type:      "tool_result",
					ToolUseID: messages[i].ToolCallID,
					Content:   messages[i].Content,
				})
				i++
			}
			out = append(out, anthropicRequestMessage{Role: "user", Content: blocks})

		default: // user
			out = append(out, anthropicRequestMessage{Role: "user", Content: m.Content})
			i++
		}
	}
	return
}

func buildAnthropicTools(defs []ToolDef) []anthropicTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]anthropicTool, len(defs))
	for i, d := range defs {
		schema := anthropicSchema{
			Type:       "object",
			Properties: make(map[string]anthropicProp),
		}
		for _, p := range d.Parameters {
			schema.Properties[p.Name] = anthropicProp{
				Type:        p.Type,
				Description: p.Description,
			}
			if p.Required {
				schema.Required = append(schema.Required, p.Name)
			}
		}
		desc := d.Description
		if d.Returns != "" {
			desc += "\nReturns: " + d.Returns
		}
		out[i] = anthropicTool{
			Name:        d.Name,
			Description: desc,
			InputSchema: schema,
		}
	}
	return out
}

func parseAnthropicResponse(resp anthropicResponse) CompletionResponse {
	var cr CompletionResponse
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			cr.Content += block.Text
		case "tool_use":
			cr.ToolCalls = append(cr.ToolCalls, ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: string(block.Input),
			})
		}
	}
	return cr
}
