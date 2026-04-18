package llm

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Model     string
	Messages  []Message
	MaxTokens int
}

type Client interface {
	Complete(ctx context.Context, req CompletionRequest) (string, error)
}
