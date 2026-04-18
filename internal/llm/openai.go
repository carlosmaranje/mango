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

type openaiMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"` // pointer so assistant tool-call messages can send null
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiTool struct {
	Type     string         `json:"type"` // "function"
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Parameters  openaiParameters `json:"parameters"`
}

type openaiParameters struct {
	Type       string                `json:"type"`
	Properties map[string]openaiProp `json:"properties,omitempty"`
	Required   []string              `json:"required,omitempty"`
}

type openaiProp struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type openaiRequest struct {
	Model          string                `json:"model"`
	Messages       []openaiMessage       `json:"messages"`
	MaxTokens      int                   `json:"max_tokens,omitempty"`
	ResponseFormat *openaiResponseFormat `json:"response_format,omitempty"`
	Tools          []openaiTool          `json:"tools,omitempty"`
}

type openaiResponseFormat struct {
	Type string `json:"type"`
}

type openaiResponseMessage struct {
	Role      string           `json:"role"`
	Content   *string          `json:"content"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
}

type openaiChoice struct {
	Message openaiResponseMessage `json:"message"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAICompatClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	oreq := openaiRequest{
		Model:     req.Model,
		Messages:  buildOpenAIMessages(req.Messages),
		MaxTokens: req.MaxTokens,
		Tools:     buildOpenAITools(req.Tools),
	}
	if req.JSON {
		oreq.ResponseFormat = &openaiResponseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(oreq)
	if err != nil {
		return CompletionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("openai-compatible request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CompletionResponse{}, fmt.Errorf("openai-compatible status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed openaiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return CompletionResponse{}, fmt.Errorf("decode openai-compatible response: %w", err)
	}
	if parsed.Error != nil {
		return CompletionResponse{}, fmt.Errorf("openai-compatible error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return CompletionResponse{}, nil
	}

	msg := parsed.Choices[0].Message
	cr := CompletionResponse{}
	if msg.Content != nil {
		cr.Content = *msg.Content
	}
	for _, tc := range msg.ToolCalls {
		cr.ToolCalls = append(cr.ToolCalls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		})
	}
	return cr, nil
}

func buildOpenAIMessages(messages []Message) []openaiMessage {
	out := make([]openaiMessage, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "tool":
			content := m.Content
			out = append(out, openaiMessage{
				Role:       "tool",
				Content:    &content,
				ToolCallID: m.ToolCallID,
				Name:       m.Name,
			})
		case "assistant":
			if len(m.ToolCalls) > 0 {
				tc := make([]openaiToolCall, len(m.ToolCalls))
				for i, call := range m.ToolCalls {
					tc[i] = openaiToolCall{
						ID:   call.ID,
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{Name: call.Name, Arguments: call.Input},
					}
				}
				var contentPtr *string
				if m.Content != "" {
					s := m.Content
					contentPtr = &s
				}
				out = append(out, openaiMessage{Role: "assistant", Content: contentPtr, ToolCalls: tc})
			} else {
				content := m.Content
				out = append(out, openaiMessage{Role: "assistant", Content: &content})
			}
		default:
			content := m.Content
			out = append(out, openaiMessage{Role: m.Role, Content: &content})
		}
	}
	return out
}

func buildOpenAITools(defs []ToolDef) []openaiTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]openaiTool, len(defs))
	for i, d := range defs {
		params := openaiParameters{
			Type:       "object",
			Properties: make(map[string]openaiProp),
		}
		for _, p := range d.Parameters {
			params.Properties[p.Name] = openaiProp{
				Type:        p.Type,
				Description: p.Description,
			}
			if p.Required {
				params.Required = append(params.Required, p.Name)
			}
		}
		out[i] = openaiTool{
			Type: "function",
			Function: openaiFunction{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  params,
			},
		}
	}
	return out
}
