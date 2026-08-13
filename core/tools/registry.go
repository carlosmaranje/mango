package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/carlosmaranje/mango/core/llm"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[t.Name()]; exists {
		return fmt.Errorf("tool %q already registered", t.Name())
	}
	r.tools[t.Name()] = t
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

func (r *Registry) Execute(ctx context.Context, name, input string) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}
	return t.Execute(ctx, input)
}

// ExecPolicy controls how a tool call is executed: how many attempts, the base
// backoff between them (scaled linearly by attempt), and a per-attempt timeout.
// The zero value (MaxAttempts<=1, no backoff, no timeout) reproduces Execute.
type ExecPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
	Timeout     time.Duration
}

// ExecResult is the outcome of ExecuteWithPolicy.
type ExecResult struct {
	Output   string
	Err      error
	Attempts int
	TimedOut bool
}

// ExecuteWithPolicy runs a tool with retry/timeout. It retries on any error up to
// MaxAttempts, applying a per-attempt timeout and linear backoff, and stops early
// if the parent context is cancelled. Only the final attempt's output/error is
// returned (retries are transparent to the caller).
func (r *Registry) ExecuteWithPolicy(ctx context.Context, name, input string, p ExecPolicy) ExecResult {
	attempts := p.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	var res ExecResult
	for i := 1; i <= attempts; i++ {
		res.Attempts = i

		callCtx := ctx
		var cancel context.CancelFunc
		if p.Timeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, p.Timeout)
		}
		out, err := r.Execute(callCtx, name, input)
		timedOut := callCtx.Err() == context.DeadlineExceeded
		if cancel != nil {
			cancel()
		}

		res.Output, res.Err, res.TimedOut = out, err, timedOut
		if err == nil {
			return res
		}
		if ctx.Err() != nil { // parent cancelled/expired — stop retrying
			return res
		}
		if i < attempts && p.Backoff > 0 {
			select {
			case <-time.After(p.Backoff * time.Duration(i)):
			case <-ctx.Done():
				return res
			}
		}
	}
	return res
}

// Clone returns a new Registry pre-populated with the same tools.
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := NewRegistry()
	for _, t := range r.tools {
		c.tools[t.Name()] = t
	}
	return c
}

// Definitions converts all registered tools to the LLM tool-definition format
// so they can be included in a CompletionRequest.
func (r *Registry) Definitions() []llm.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]llm.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		def := llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Returns:     t.Returns(),
		}
		for _, p := range t.Parameters() {
			def.Parameters = append(def.Parameters, llm.ToolParam{
				Name:        p.Name,
				Type:        p.Type,
				Description: p.Description,
				Required:    p.Required,
			})
		}
		out = append(out, def)
	}
	return out
}
