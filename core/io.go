// Package core is the public, embeddable API of mango-core: a bare-bones,
// pluggable multi-agent orchestration engine.
//
// mango-core knows nothing about Discord, HTTP gateways, config files, or the
// MANGO_DIR filesystem layout. A host program wires those concerns in and
// drives the engine through a single, stable input/output contract:
//
//   - Request  — the INPUT: a goal, an optional target agent, optional session
//     key, optional explicit history, and flags.
//   - Result   — the synchronous/polled OUTPUT: status + final output text.
//   - Event    — the streaming OUTPUT: task and agent lifecycle notifications.
//
// A minimal host:
//
//	eng, _ := core.New(core.Options{Agents: []core.AgentSpec{{
//		Name:         "worker",
//		SystemPrompt: "You are a helpful worker.",
//		LLM:          client, // any core/llm.Client
//	}}})
//	_ = eng.Start(ctx)
//	res, _ := eng.SubmitAndWait(ctx, core.Request{Goal: "hello", Agent: "worker"})
//	fmt.Println(res.Output)
package core

import (
	"time"

	"github.com/carlosmaranje/mango/core/llm"
	"github.com/carlosmaranje/mango/core/orchestrator"
)

// Message is a single conversational turn. It is re-exported from core/llm so
// hosts can build Request.History without importing the llm package directly.
type Message = llm.Message

// Status is the lifecycle state of a submitted task.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusDone      Status = "done"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Request is the INPUT contract for mango-core.
//
// Goal is the only required field. Leaving Agent empty routes the goal through
// the orchestrator (which decomposes and fans out to other agents); setting
// Agent dispatches directly to that named agent.
type Request struct {
	// Goal is the task/prompt to execute.
	Goal string `json:"goal"`
	// Agent names the target agent. Empty routes through the orchestrator.
	Agent string `json:"agent,omitempty"`
	// SessionID keys per-conversation history. Empty means stateless.
	SessionID string `json:"session_id,omitempty"`
	// History, when non-nil, overrides any session-derived history for this call.
	History []Message `json:"history,omitempty"`
	// Metadata carries arbitrary host tags. mango-core does not interpret it.
	Metadata map[string]string `json:"metadata,omitempty"`
	// JSON requests a JSON-object response from the agent (direct-agent path).
	JSON bool `json:"json,omitempty"`
}

// Result is the polled/synchronous OUTPUT contract for mango-core.
type Result struct {
	ID        string    `json:"id"`
	Goal      string    `json:"goal"`
	Agent     string    `json:"agent,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Status    Status    `json:"status"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Done reports whether the task has reached a terminal state.
func (r *Result) Done() bool {
	switch r.Status {
	case StatusDone, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// EventType enumerates the streaming output events.
type EventType string

const (
	EventTaskCreated  EventType = "task.created"
	EventTaskUpdated  EventType = "task.updated"
	EventAgentStarted EventType = "agent.started"
	EventAgentStopped EventType = "agent.stopped"
)

// Event is the streaming OUTPUT contract. Task is populated for task.* events;
// Agent is populated for agent.* events.
type Event struct {
	Type  EventType `json:"type"`
	Task  *Result   `json:"task,omitempty"`
	Agent string    `json:"agent,omitempty"`
}

// AgentInfo describes a registered agent and its current run state.
type AgentInfo struct {
	Name   string   `json:"name"`
	Status string   `json:"status"` // "running" | "stopped"
	Skills []string `json:"skills,omitempty"`
}

// taskToResult maps an internal orchestrator.Task onto the public Result.
func taskToResult(t *orchestrator.Task) *Result {
	if t == nil {
		return nil
	}
	return &Result{
		ID:        t.ID,
		Goal:      t.Goal,
		Agent:     t.AgentName,
		SessionID: t.SessionID,
		Status:    Status(t.Status),
		Output:    t.Result,
		Error:     t.Error,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// translateEvent maps an internal orchestrator.Event onto the public Event.
func translateEvent(ev orchestrator.Event) Event {
	out := Event{Type: EventType(ev.Type)}
	switch p := ev.Payload.(type) {
	case *orchestrator.Task:
		out.Task = taskToResult(p)
	case map[string]string:
		out.Agent = p["name"]
	}
	return out
}
