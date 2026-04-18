package llm

import "context"

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // set when Role=="assistant" and model invoked tools
	ToolCallID string     `json:"tool_call_id,omitempty"` // set when Role=="tool" (result message)
	Name       string     `json:"name,omitempty"`         // tool name, set when Role=="tool"
}

type ToolParam struct {
	Name        string
	Type        string // "string", "number", "boolean", "object", "array"
	Description string
	Required    bool
}

type ToolDef struct {
	Name        string
	Description string
	Parameters  []ToolParam
}

type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"` // JSON-encoded arguments
}

type CompletionRequest struct {
	Model     string
	Messages  []Message
	MaxTokens int
	JSON      bool
	Tools     []ToolDef
}

type CompletionResponse struct {
	Content   string
	ToolCalls []ToolCall
}

type Client interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
