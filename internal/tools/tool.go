package tools

import "context"

type Parameter struct {
	Name        string
	Type        string // "string", "number", "boolean", "object", "array"
	Description string
	Required    bool
}

type Tool interface {
	Name() string
	Description() string
	Parameters() []Parameter
	Execute(ctx context.Context, input string) (string, error)
}
