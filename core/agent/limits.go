package agent

import (
	"time"

	"github.com/carlosmaranje/mango/core/tools"
)

const (
	defaultMaxSteps      = 12
	defaultNoProgressN   = 2
	defaultContextBudget = 100_000 // token-equivalent units (rough len/4 estimate)
)

// StopReason explains why an agent invocation's step loop ended.
type StopReason string

const (
	StopCompleted       StopReason = "completed"        // model returned a final answer (no tool calls)
	StopMaxSteps        StopReason = "max_steps"        // hit Limits.MaxSteps
	StopTokenBudget     StopReason = "token_budget"     // hit Limits.MaxTokens
	StopDeadline        StopReason = "deadline"         // hit Limits.Deadline
	StopNoProgress      StopReason = "no_progress"      // repeated identical no-tool turns
	StopContextCanceled StopReason = "context_canceled" // parent context cancelled/expired
)

// Limits bounds a single agent invocation so a long tool chain can't loop
// forever, blow the context window, or run unboundedly. The zero value is
// filled with generous defaults by EffectiveLimits, so existing callers that
// never set Limits keep their current behavior.
type Limits struct {
	MaxSteps      int              // max tool-loop iterations
	MaxTokens     int              // cumulative prompt+completion token estimate; 0 = no budget
	Deadline      time.Duration    // wall-clock per invocation; 0 = none
	NoProgressN   int              // consecutive identical no-tool turns before stopping
	ContextBudget int              // token-equivalent budget for in-invocation windowing
	ToolPolicy    tools.ExecPolicy // per-tool-call retry/timeout policy
}

// EffectiveLimits returns the agent's limits with defaults applied. Mirrors
// EffectiveMaxTokens.
func (a *Agent) EffectiveLimits() Limits {
	l := a.Limits
	if l.MaxSteps <= 0 {
		l.MaxSteps = defaultMaxSteps
	}
	if l.NoProgressN <= 0 {
		l.NoProgressN = defaultNoProgressN
	}
	if l.ContextBudget <= 0 {
		l.ContextBudget = defaultContextBudget
	}
	if l.ToolPolicy.MaxAttempts < 1 {
		l.ToolPolicy.MaxAttempts = 1
	}
	return l
}
