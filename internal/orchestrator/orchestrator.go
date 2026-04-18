package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/carlosmaranje/mango/internal/agent"
	"github.com/carlosmaranje/mango/internal/llm"
)

const DefaultMaxSteps = 5

type OrchestratedTask struct {
	Agent string `json:"agent"`
	Goal  string `json:"goal"`
	JSON  bool   `json:"json,omitempty"` // If true, the dispatcher will request a JSON response from the agent.
}

type orchestratorResponse struct {
	Action string             `json:"action"`
	Tasks  []OrchestratedTask `json:"tasks"`
	Final  string             `json:"final,omitempty"`
}

type StepResult struct {
	Agent  string
	Goal   string
	Result string
	Err    error
}

type Orchestrator struct {
	Agent    *agent.Agent
	MaxSteps int
	Registry *agent.Registry
}

func NewOrchestrator(a *agent.Agent, reg *agent.Registry) *Orchestrator {
	return &Orchestrator{Agent: a, Registry: reg, MaxSteps: DefaultMaxSteps}
}

func (p *Orchestrator) Run(ctx context.Context, goal string, history []llm.Message, d *Dispatcher) (string, error) {
	if p.Agent == nil || p.Agent.LLM == nil {
		return "", fmt.Errorf("orchestrator agent has no LLM client")
	}
	if p.Agent.SystemPrompt == "" {
		return "", fmt.Errorf("orchestrator agent %q has no system prompt (expected PULSE.md)", p.Agent.Name)
	}
	maxSteps := p.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}

	messages := []llm.Message{
		{Role: "system", Content: p.Agent.SystemPrompt + "\n\n" + p.agentCatalog()},
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	messages = append(messages, llm.Message{Role: "user", Content: "Goal: " + goal})

	for step := 0; step < maxSteps; step++ {
		resp, err := p.Agent.LLM.Complete(ctx, llm.CompletionRequest{
			Model:     p.Agent.Model,
			Messages:  messages,
			MaxTokens: 1024,
			JSON:      true,
		})
		if err != nil {
			return "", fmt.Errorf("orchestrator LLM: %w", err)
		}
		raw := resp.Content
		parsed, err := parseOrchestratorResponse(raw)
		if err != nil || parsed == nil {
			log.Printf("orchestrator: model %q returned non-JSON (raw=%q, err=%v). Retrying with corrective hint...", p.Agent.Model, raw, err)
			messages = append(messages,
				llm.Message{Role: "assistant", Content: raw},
				llm.Message{Role: "user", Content: "ERROR: Your response was not a valid JSON object. Please respond ONLY with a JSON object matching the schema. No preamble, no markdown fences. Remember to always include \"action\", \"tasks\", and \"final\" keys."},
			)
			continue
		}

		if parsed.Action == "finish" {
			if parsed.Final != "" {
				return parsed.Final, nil
			}
			log.Printf("orchestrator: model %q returned action=finish with empty \"final\"; retrying with corrective hint (raw=%q)", p.Agent.Model, raw)
			messages = append(messages,
				llm.Message{Role: "assistant", Content: raw},
				llm.Message{Role: "user", Content: "ERROR: You set action=finish but provided an empty \"final\" field. Please provide your final answer in the \"final\" field."},
			)
			continue
		}
		if len(parsed.Tasks) == 0 {
			if parsed.Final != "" {
				return parsed.Final, nil
			}
			log.Printf("orchestrator: model %q returned action=continue with no tasks; retrying with corrective hint (raw=%q)", p.Agent.Model, raw)
			messages = append(messages,
				llm.Message{Role: "assistant", Content: raw},
				llm.Message{Role: "user", Content: "Your previous response had action=continue with no tasks, which is invalid. If the goal can be answered from context, respond with action=finish and put the answer in \"final\". Otherwise, dispatch at least one task."},
			)
			continue
		}

		results := d.FanOut(ctx, parsed.Tasks)
		messages = append(messages,
			llm.Message{Role: "assistant", Content: raw},
			llm.Message{Role: "user", Content: renderStepResults(results)},
		)
	}

	return "", fmt.Errorf("orchestrator exceeded max steps (%d)", maxSteps)
}

func (p *Orchestrator) agentCatalog() string {
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

func parseOrchestratorResponse(raw string) (*orchestratorResponse, error) {
	candidate := stripJSONFence(strings.TrimSpace(raw))
	var resp orchestratorResponse
	if err := json.Unmarshal([]byte(candidate), &resp); err == nil {
		if resp.Action == "" {
			return nil, fmt.Errorf("missing action")
		}
		return &resp, nil
	}
	if obj, ok := extractJSONObject(candidate); ok {
		if err := json.Unmarshal([]byte(obj), &resp); err == nil {
			if resp.Action == "" {
				return nil, fmt.Errorf("missing action")
			}
			return &resp, nil
		}
	}
	return nil, fmt.Errorf("no parseable JSON object found")
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

// extractJSONObject returns the first balanced {...} block found in s.
// It tolerates preambles ("Here's the plan: {...}") and trailing commentary.
func extractJSONObject(s string) (string, bool) {
	start := strings.Index(s, "{")
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if inString {
			if c == '\\' {
				escape = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
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
