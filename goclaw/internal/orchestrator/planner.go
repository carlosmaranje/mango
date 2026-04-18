package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/carlosmaranje/goclaw/internal/agent"
	"github.com/carlosmaranje/goclaw/internal/llm"
)

const DefaultMaxSteps = 5

type PlannedTask struct {
	Agent string `json:"agent"`
	Goal  string `json:"goal"`
}

type plannerResponse struct {
	Action string        `json:"action"`
	Tasks  []PlannedTask `json:"tasks"`
	Final  string        `json:"final,omitempty"`
}

type StepResult struct {
	Agent  string
	Goal   string
	Result string
	Err    error
}

type Planner struct {
	Agent    *agent.Agent
	MaxSteps int
	Registry *agent.Registry
}

func NewPlanner(a *agent.Agent, reg *agent.Registry) *Planner {
	return &Planner{Agent: a, Registry: reg, MaxSteps: DefaultMaxSteps}
}

const plannerSystemPrompt = `You are the orchestrator for a multi-agent system. Decompose the user's goal into tasks routed to specific agents.
 
Respond with JSON only — no preamble, no markdown fences. Schema:
{
  "action": "continue" | "finish",
  "tasks": [ { "agent": "<agent_name>", "goal": "<sub-goal>" } ],
  "final": "<final answer, only when action=finish>"
}
 
Continue until you can return action=finish with a final answer synthesized from prior step results.`

func (p *Planner) Run(ctx context.Context, goal string, d *Dispatcher) (string, error) {
	if p.Agent == nil || p.Agent.LLM == nil {
		return "", fmt.Errorf("planner agent has no LLM client")
	}
	maxSteps := p.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}

	messages := []llm.Message{
		{Role: "system", Content: plannerSystemPrompt + "\n\n" + p.agentCatalog()},
		{Role: "user", Content: "Goal: " + goal},
	}

	for step := 0; step < maxSteps; step++ {
		raw, err := p.Agent.LLM.Complete(ctx, llm.CompletionRequest{
			Model:     p.Agent.Model,
			Messages:  messages,
			MaxTokens: 1024,
		})
		if err != nil {
			return "", fmt.Errorf("planner LLM: %w", err)
		}
		parsed, err := parsePlannerResponse(raw)
		if err != nil {
			return "", fmt.Errorf("planner parse: %w (raw=%q)", err, raw)
		}

		if parsed.Action == "finish" {
			if parsed.Final != "" {
				return parsed.Final, nil
			}
			return raw, nil
		}
		if len(parsed.Tasks) == 0 {
			return "", fmt.Errorf("planner returned action=continue with no tasks")
		}

		results := d.FanOut(ctx, parsed.Tasks)
		messages = append(messages,
			llm.Message{Role: "assistant", Content: raw},
			llm.Message{Role: "user", Content: renderStepResults(results)},
		)
	}

	return "", fmt.Errorf("planner exceeded max steps (%d)", maxSteps)
}

func (p *Planner) agentCatalog() string {
	if p.Registry == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available agents:\n")
	for _, a := range p.Registry.List() {
		if a.Role == "orchestrator" {
			continue
		}
		fmt.Fprintf(&b, "- %s (capabilities: %v)\n", a.Name, a.Capabilities)
	}
	return b.String()
}

func parsePlannerResponse(raw string) (*plannerResponse, error) {
	cleaned := stripJSONFence(strings.TrimSpace(raw))
	var resp plannerResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, err
	}
	if resp.Action == "" {
		return nil, fmt.Errorf("missing action")
	}
	return &resp, nil
}

func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func renderStepResults(results []StepResult) string {
	var b strings.Builder
	b.WriteString("Step results:\n")
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(&b, "- [%s] ERROR: %s\n", r.Agent, r.Err)
			continue
		}
		fmt.Fprintf(&b, "- [%s] %s\n", r.Agent, r.Result)
	}
	return b.String()
}
