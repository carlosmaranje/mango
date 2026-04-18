package orchestrator

import (
	"context"
	"sync"

	"github.com/carlosmaranje/goclaw/internal/llm"
)

// mockLLM is a test-only LLM client. Set either `response` for a single fixed
// reply, or `responses` for a scripted sequence indexed by call number.
type mockLLM struct {
	mu        sync.Mutex
	response  string
	responses []string
	err       error
	calls     []llm.CompletionRequest
}

func (m *mockLLM) Complete(ctx context.Context, req llm.CompletionRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := len(m.calls)
	m.calls = append(m.calls, req)
	if len(m.responses) > 0 {
		if idx < len(m.responses) {
			return m.responses[idx], m.err
		}
		return m.responses[len(m.responses)-1], m.err
	}
	return m.response, m.err
}

func (m *mockLLM) LastMessages() []llm.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	return append([]llm.Message(nil), m.calls[len(m.calls)-1].Messages...)
}

func (m *mockLLM) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}
