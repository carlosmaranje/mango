package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/carlosmaranje/mango/core/event"
	"github.com/carlosmaranje/mango/core/llm"
	"github.com/carlosmaranje/mango/core/tools"
)

type TaskEnvelope struct {
	ID       string
	Goal     string
	Reply    chan<- TaskResult
	Metadata map[string]string
	History  []llm.Message
	JSON     bool
}

type TaskResult struct {
	ID         string
	Result     string
	StopReason StopReason
	Err        error
}

type Runner struct {
	Agent    *Agent
	Interval time.Duration
	toolReg  *tools.Registry
	bus      *event.Bus

	taskCh chan TaskEnvelope

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	stopDone chan struct{}
}

func NewRunner(a *Agent, toolReg *tools.Registry, interval time.Duration) *Runner {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Runner{
		Agent:    a,
		Interval: interval,
		toolReg:  toolReg,
		taskCh:   make(chan TaskEnvelope, 64),
	}
}

// SetBus attaches an event bus so the runner emits step/tool lifecycle events.
// Optional: with no bus, no fine-grained events are emitted (or even built).
func (r *Runner) SetBus(b *event.Bus) {
	r.bus = b
}

func (r *Runner) Submit(env TaskEnvelope) {
	r.taskCh <- env
}

func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *Runner) Start(parent context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return fmt.Errorf("runner for %q already running", r.Agent.Name)
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.running = true
	r.stopDone = make(chan struct{})
	go r.loop(ctx)
	return nil
}

func (r *Runner) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	cancel := r.cancel
	done := r.stopDone
	r.mu.Unlock()

	cancel()
	<-done
}

func (r *Runner) loop(ctx context.Context) {
	defer func() {
		r.mu.Lock()
		r.running = false
		close(r.stopDone)
		r.mu.Unlock()
	}()

	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case env := <-r.taskCh:
			go r.executeTask(ctx, env)
		case <-ticker.C:
			r.heartbeat(ctx)
		}
	}
}

func (r *Runner) heartbeat(_ context.Context) {
	if r.Agent.Memory == nil {
		return
	}
	_ = r.Agent.Memory.Set("heartbeat/last", time.Now().UTC().Format(time.RFC3339))
}

func (r *Runner) executeTask(ctx context.Context, env TaskEnvelope) {
	result, reason, err := r.runSteps(ctx, env.ID, env.Goal, env.History, env.JSON)
	if env.Reply != nil {
		select {
		case env.Reply <- TaskResult{ID: env.ID, Result: result, StopReason: reason, Err: err}:
		case <-ctx.Done():
		}
	}
}

// invokeLLM runs the agent's bounded tool loop and returns the final content.
// It is a thin wrapper over runSteps that drops the StopReason, kept for callers
// (and tests) that only care about the output.
func (r *Runner) invokeLLM(ctx context.Context, goal string, history []llm.Message, jsonResponse bool) (string, error) {
	out, _, err := r.runSteps(ctx, "", goal, history, jsonResponse)
	return out, err
}

// runSteps drives the ReAct-style tool loop for one invocation. It is bounded by
// the agent's Limits (steps, token estimate, wall-clock deadline, no-progress),
// trims context to a budget each iteration, and runs each tool call under the
// agent's retry/timeout policy. On hitting a limit it stops GRACEFULLY: it
// returns the best-effort latest content plus a StopReason (not an error). Only a
// genuine LLM transport error (not a context cancellation) aborts with an error.
func (r *Runner) runSteps(ctx context.Context, taskID, goal string, history []llm.Message, jsonResponse bool) (string, StopReason, error) {
	if r.Agent.LLM == nil {
		return "", "", fmt.Errorf("agent %q has no LLM client", r.Agent.Name)
	}
	if r.Agent.SystemPrompt == "" {
		return "", "", fmt.Errorf("agent %q has no system prompt", r.Agent.Name)
	}

	lim := r.Agent.EffectiveLimits()
	parent := ctx
	if lim.Deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, lim.Deadline)
		defer cancel()
	}

	messages := []llm.Message{{Role: "system", Content: r.Agent.SystemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: goal})

	var toolDefs []llm.ToolDef
	if r.toolReg != nil {
		toolDefs = r.toolReg.Definitions()
	}

	tokenEst := 0
	lastSig := ""
	lastContent := ""
	repeats := 0

	for step := 1; step <= lim.MaxSteps; step++ {
		if ctx.Err() != nil {
			return lastContent, r.stop(taskID, step, r.ctxReason(parent, ctx, lim.Deadline > 0)), nil
		}

		messages = windowMessages(messages, lim.ContextBudget)

		promptEst := 0
		for _, m := range messages {
			promptEst += estTokens(m)
		}

		r.emitStep(event.TypeStepStarted, taskID, step, "")
		log.Printf("agent %q: step %d — sending %d messages to LLM (tools: %d)", r.Agent.Name, step, len(messages), len(toolDefs))

		resp, err := r.Agent.LLM.Complete(ctx, llm.CompletionRequest{
			Messages:  messages,
			MaxTokens: r.Agent.EffectiveMaxTokens(),
			JSON:      jsonResponse,
			Tools:     toolDefs,
		})
		if err != nil {
			if ctx.Err() != nil {
				return lastContent, r.stop(taskID, step, r.ctxReason(parent, ctx, lim.Deadline > 0)), nil
			}
			return "", "", err
		}

		lastContent = resp.Content
		tokenEst += promptEst + estTokens(llm.Message{Content: resp.Content})
		r.emitStep(event.TypeStepCompleted, taskID, step, "")
		log.Printf("agent %q: step %d — content=%q toolCalls=%d", r.Agent.Name, step, resp.Content, len(resp.ToolCalls))

		// No tool calls means the model produced its final answer.
		if len(resp.ToolCalls) == 0 {
			return resp.Content, r.stop(taskID, step, StopCompleted), nil
		}

		// No-progress: identical content + identical tool calls repeated.
		sig := stepSignature(resp.Content, resp.ToolCalls)
		if sig == lastSig {
			repeats++
			if repeats >= lim.NoProgressN {
				return resp.Content, r.stop(taskID, step, StopNoProgress), nil
			}
		} else {
			repeats = 0
		}
		lastSig = sig

		if lim.MaxTokens > 0 && tokenEst >= lim.MaxTokens {
			return resp.Content, r.stop(taskID, step, StopTokenBudget), nil
		}

		// Append the assistant turn (with tool calls) then execute each tool.
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		for _, tc := range resp.ToolCalls {
			r.emitTool(event.TypeToolCalled, taskID, tc.Name, step, 0, nil)
			log.Printf("agent %q: step %d — tool call %q input=%s", r.Agent.Name, step, tc.Name, tc.Input)

			var exec tools.ExecResult
			if r.toolReg != nil {
				exec = r.toolReg.ExecuteWithPolicy(ctx, tc.Name, tc.Input, lim.ToolPolicy)
			} else {
				exec = tools.ExecResult{Err: fmt.Errorf("tool %q not found", tc.Name)}
			}

			msg := llm.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Name}
			if exec.Err != nil {
				log.Printf("agent %q: step %d — tool %q error (attempts=%d): %v", r.Agent.Name, step, tc.Name, exec.Attempts, exec.Err)
				msg.Content = "error: " + exec.Err.Error()
			} else {
				log.Printf("agent %q: step %d — tool %q result=%s", r.Agent.Name, step, tc.Name, exec.Output)
				msg.Content = exec.Output
			}
			r.emitTool(event.TypeToolCompleted, taskID, tc.Name, step, exec.Attempts, exec.Err)
			messages = append(messages, msg)
		}
	}

	return lastContent, r.stop(taskID, lim.MaxSteps, StopMaxSteps), nil
}

// stop emits an invocation.stopped event and returns the reason.
func (r *Runner) stop(taskID string, step int, reason StopReason) StopReason {
	r.emitStep(event.TypeInvocationStopped, taskID, step, reason)
	return reason
}

// ctxReason classifies a context-driven stop: the agent's own Deadline firing
// (parent still alive) is StopDeadline; otherwise the parent was cancelled.
func (r *Runner) ctxReason(parent, ctx context.Context, deadlineSet bool) StopReason {
	if deadlineSet && parent.Err() == nil && ctx.Err() != nil {
		return StopDeadline
	}
	return StopContextCanceled
}

func (r *Runner) emitStep(typ, taskID string, step int, reason StopReason) {
	if r.bus == nil {
		return
	}
	r.bus.Emit(event.Event{Type: typ, Payload: event.StepInfo{
		AgentName:  r.Agent.Name,
		TaskID:     taskID,
		Step:       step,
		StopReason: string(reason),
	}})
}

func (r *Runner) emitTool(typ, taskID, tool string, step, attempt int, err error) {
	if r.bus == nil {
		return
	}
	r.bus.Emit(event.Event{Type: typ, Payload: event.ToolInfo{
		AgentName: r.Agent.Name,
		TaskID:    taskID,
		Tool:      tool,
		Step:      step,
		Attempt:   attempt,
		Err:       errString(err),
	}})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// stepSignature fingerprints an assistant turn (content + each tool call's name
// and input) so two consecutive identical actions register as no-progress.
func stepSignature(content string, calls []llm.ToolCall) string {
	var b strings.Builder
	b.WriteString(content)
	for _, c := range calls {
		b.WriteByte('\n')
		b.WriteString(c.Name)
		b.WriteByte(' ')
		b.WriteString(c.Input)
	}
	return b.String()
}
