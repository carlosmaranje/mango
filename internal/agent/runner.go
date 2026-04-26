package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/carlosmaranje/mango/internal/llm"
	"github.com/carlosmaranje/mango/internal/tools"
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
	ID     string
	Result string
	Err    error
}

type Runner struct {
	Agent    *Agent
	Interval time.Duration
	toolReg  *tools.Registry

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
	result, err := r.invokeLLM(ctx, env.Goal, env.History, env.JSON)
	if env.Reply != nil {
		select {
		case env.Reply <- TaskResult{ID: env.ID, Result: result, Err: err}:
		case <-ctx.Done():
		}
	}
}

func (r *Runner) invokeLLM(ctx context.Context, goal string, history []llm.Message, jsonResponse bool) (string, error) {
	if r.Agent.LLM == nil {
		return "", fmt.Errorf("agent %q has no LLM client", r.Agent.Name)
	}
	if r.Agent.SystemPrompt == "" {
		return "", fmt.Errorf("agent %q has no system prompt", r.Agent.Name)
	}

	messages := []llm.Message{{Role: "system", Content: r.Agent.SystemPrompt}}
	if len(history) > 0 {
		messages = append(messages, history...)
	} else if r.Agent.Session != nil {
		messages = append(messages, r.Agent.Session.Snapshot()...)
	}
	messages = append(messages, llm.Message{Role: "user", Content: goal})

	var toolDefs []llm.ToolDef
	if r.toolReg != nil {
		toolDefs = r.toolReg.Definitions()
	}

	useSession := len(history) == 0 && r.Agent.Session != nil

	for step := 1; ; step++ {
		log.Printf("agent %q: step %d — sending %d messages to LLM (tools: %d)", r.Agent.Name, step, len(messages), len(toolDefs))
		resp, err := r.Agent.LLM.Complete(ctx, llm.CompletionRequest{
			Messages:  messages,
			MaxTokens: r.Agent.EffectiveMaxTokens(),
			JSON:      jsonResponse,
			Tools:     toolDefs,
		})
		if err != nil {
			return "", err
		}

		log.Printf("agent %q: step %d — content=%q toolCalls=%d", r.Agent.Name, step, resp.Content, len(resp.ToolCalls))

		if len(resp.ToolCalls) == 0 {
			if useSession {
				r.Agent.Session.Append(llm.Message{Role: "user", Content: goal})
				r.Agent.Session.Append(llm.Message{Role: "assistant", Content: resp.Content})
			}
			return resp.Content, nil
		}

		// Append the assistant turn (with tool calls) then execute each tool.
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		for _, tc := range resp.ToolCalls {
			log.Printf("agent %q: step %d — tool call %q input=%s", r.Agent.Name, step, tc.Name, tc.Input)
			result, execErr := r.toolReg.Execute(ctx, tc.Name, tc.Input)
			msg := llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Name,
			}
			if execErr != nil {
				log.Printf("agent %q: step %d — tool %q error: %v", r.Agent.Name, step, tc.Name, execErr)
				msg.Content = "error: " + execErr.Error()
			} else {
				log.Printf("agent %q: step %d — tool %q result=%s", r.Agent.Name, step, tc.Name, result)
				msg.Content = result
			}
			messages = append(messages, msg)
		}
	}
}
