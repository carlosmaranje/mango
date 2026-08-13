package tools

import (
	"context"
	"encoding/json"
)

// DescribeReturnType marshals v to indented JSON so tools can derive their
// Returns() description directly from the response struct rather than
// maintaining a hand-written string.
func DescribeReturnType(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

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
	Returns() string
	Execute(ctx context.Context, input string) (string, error)
}
