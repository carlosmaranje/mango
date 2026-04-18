package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/carlosmaranje/goclaw/internal/llm"
)

type TaskEnvelope struct {
	ID       string
	Goal     string
	Reply    chan<- TaskResult
	Metadata map[string]string
}

type TaskResult struct {
	ID     string
	Result string
	Err    error
}

type Runner struct {
	Agent    *Agent
	Interval time.Duration

	taskCh chan TaskEnvelope

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	stopDone chan struct{}
}

func NewRunner(a *Agent, interval time.Duration) *Runner {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Runner{
		Agent:    a,
		Interval: interval,
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

func (r *Runner) heartbeat(ctx context.Context) {
	if r.Agent.Memory == nil {
		return
	}
	_ = r.Agent.Memory.Set("heartbeat/last", time.Now().UTC().Format(time.RFC3339))
}

func (r *Runner) executeTask(ctx context.Context, env TaskEnvelope) {
	result, err := r.invokeLLM(ctx, env.Goal)
	if env.Reply != nil {
		select {
		case env.Reply <- TaskResult{ID: env.ID, Result: result, Err: err}:
		case <-ctx.Done():
		}
	}
}

func (r *Runner) invokeLLM(ctx context.Context, goal string) (string, error) {
	if r.Agent.LLM == nil {
		return "", fmt.Errorf("agent %q has no LLM client", r.Agent.Name)
	}
	messages := []llm.Message{
		{Role: "system", Content: fmt.Sprintf("You are agent %q. Capabilities: %v.", r.Agent.Name, r.Agent.Capabilities)},
	}
	if r.Agent.Session != nil {
		messages = append(messages, r.Agent.Session.Snapshot()...)
	}
	messages = append(messages, llm.Message{Role: "user", Content: goal})

	out, err := r.Agent.LLM.Complete(ctx, llm.CompletionRequest{
		Model:     r.Agent.Model,
		Messages:  messages,
		MaxTokens: 1024,
	})
	if err != nil {
		return "", err
	}
	if r.Agent.Session != nil {
		r.Agent.Session.Append(llm.Message{Role: "user", Content: goal})
		r.Agent.Session.Append(llm.Message{Role: "assistant", Content: out})
	}
	return out, nil
}
