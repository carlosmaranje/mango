package orchestrator

import (
	"context"
	"testing"

	"github.com/carlosmaranje/goclaw/internal/agent"
	"github.com/carlosmaranje/goclaw/internal/llm"
)

type mockLLM struct {
	response string
}

func (m *mockLLM) Complete(ctx context.Context, req llm.CompletionRequest) (string, error) {
	return m.response, nil
}

func TestPlannerRun_NonJSONResponse(t *testing.T) {
	mock := &mockLLM{response: "This is not JSON"}
	a := &agent.Agent{
		Name:         "test-orchestrator",
		Role:         "orchestrator",
		Model:        "tinydolphin",
		LLM:          mock,
		SystemPrompt: "You are the orchestrator.",
	}
	reg := agent.NewRegistry()
	p := NewPlanner(a, reg)
	d := NewDispatcher(reg, nil, p)

	_, err := p.Run(context.Background(), "test goal", nil, d)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expected := "the model \"tinydolphin\" might not be suitable for the \"orchestrator\" role: it returned a non-JSON response"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}

	t.Logf("Error: %v", err)
}
