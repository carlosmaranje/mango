package orchestrator

import (
	"context"
	"testing"

	"github.com/carlosmaranje/mango/internal/agent"
)

func TestOrchestratorRun_RetriesOnNonJSON(t *testing.T) {
	mock := &mockLLM{responses: []string{
		"Bad response, not JSON",
		`{"action":"finish","final":"Fixed response"}`},
	}
	a := &agent.Agent{
		Name:         "test-orchestrator",
		LLM:          mock,
		SystemPrompt: "You are the orchestrator.",
	}
	reg := agent.NewRegistry()
	orch := NewOrchestrator(a, reg)
	d := NewDispatcher(reg, nil, orch)

	result, err := orch.Run(context.Background(), "hi", nil, d)
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if result != "Fixed response" {
		t.Errorf("expected %q, got %q", "Fixed response", result)
	}
	if mock.CallCount() != 2 {
		t.Errorf("expected 2 calls, got %d", mock.CallCount())
	}
}

func TestOrchestratorRun_ExceedsMaxStepsOnConstantNonJSON(t *testing.T) {
	mock := &mockLLM{response: "Still not JSON"}
	a := &agent.Agent{
		Name:         "test-orchestrator",
		LLM:          mock,
		SystemPrompt: "You are the orchestrator.",
	}
	reg := agent.NewRegistry()
	orch := NewOrchestrator(a, reg)
	orch.MaxSteps = 3
	d := NewDispatcher(reg, nil, orch)

	_, err := orch.Run(context.Background(), "hi", nil, d)
	if err == nil {
		t.Fatal("expected error after exceeding max steps, got nil")
	}
	if mock.CallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", mock.CallCount())
	}
}
