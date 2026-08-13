package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/carlosmaranje/mango/core/agent"
)

func TestOrchestratorRun_InjectsProtocolAndAgentCatalog(t *testing.T) {
	mock := &mockLLM{response: `{"action":"finish","tasks":[],"final":"done"}`}
	a := &agent.Agent{
		Name:         "test-orchestrator",
		LLM:          mock,
		SystemPrompt: "HOST PERSONA ONLY",
	}
	worker := &agent.Agent{Name: "researcher", Skills: []string{"web_search"}}
	reg := agent.NewRegistry()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register orchestrator: %v", err)
	}
	if err := reg.Register(worker); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	orch := NewOrchestrator(a, reg)
	d := NewDispatcher(reg, nil, orch, nil)

	if _, err := orch.Run(context.Background(), "hi", nil, d); err != nil {
		t.Fatalf("Run: %v", err)
	}
	messages := mock.LastMessages()
	if len(messages) == 0 || messages[0].Role != "system" {
		t.Fatalf("missing system message: %#v", messages)
	}
	prompt := messages[0].Content
	for _, want := range []string{
		"HOST PERSONA ONLY",
		ProtocolPrompt,
		"Available agents:",
		"- researcher (skills: [web_search])",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt does not contain %q:\n%s", want, prompt)
		}
	}
	if strings.Index(prompt, ProtocolPrompt) < strings.Index(prompt, "HOST PERSONA ONLY") {
		t.Error("core protocol must follow the host persona so its contract takes precedence")
	}
}

func TestOrchestratorRun_RetriesOnNonJSON(t *testing.T) {
	mock := &mockLLM{responses: []string{
		"Bad response, not JSON",
		`{"action":"finish","tasks":[],"final":"Fixed response"}`},
	}
	a := &agent.Agent{
		Name:         "test-orchestrator",
		LLM:          mock,
		SystemPrompt: "You are the orchestrator.",
	}
	reg := agent.NewRegistry()
	orch := NewOrchestrator(a, reg)
	d := NewDispatcher(reg, nil, orch, nil)

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
	d := NewDispatcher(reg, nil, orch, nil)

	_, err := orch.Run(context.Background(), "hi", nil, d)
	if err == nil {
		t.Fatal("expected error after exceeding max steps, got nil")
	}
	if mock.CallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", mock.CallCount())
	}
}
