package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/carlosmaranje/mango/internal/agent"
	"github.com/carlosmaranje/mango/internal/llm"
)

const defaultSessionSize = 200

type sessionStore struct {
	mu      sync.Mutex
	maxSize int
	data    map[string][]llm.Message
}

func newSessionStore() *sessionStore {
	return &sessionStore{maxSize: defaultSessionSize, data: make(map[string][]llm.Message)}
}

func (s *sessionStore) get(id string) []llm.Message {
	if id == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := s.data[id]
	if len(buf) == 0 {
		return nil
	}
	out := make([]llm.Message, len(buf))
	copy(out, buf)
	return out
}

func (s *sessionStore) append(id string, msg llm.Message) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := s.data[id]
	buf = append(buf, msg)
	if len(buf) > s.maxSize {
		buf = buf[len(buf)-s.maxSize:]
	}
	s.data[id] = buf
}

type Dispatcher struct {
	registry *agent.Registry
	runners  map[string]*agent.Runner

	mu    sync.RWMutex
	tasks map[string]*Task

	orchestrator *Orchestrator
	sessions     *sessionStore
}

func NewDispatcher(reg *agent.Registry, runners map[string]*agent.Runner, orch *Orchestrator) *Dispatcher {
	return &Dispatcher{
		registry:     reg,
		runners:      runners,
		tasks:        make(map[string]*Task),
		orchestrator: orch,
		sessions:     newSessionStore(),
	}
}

func newTaskID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (d *Dispatcher) Submit(ctx context.Context, goal, agentName, sessionID string) (*Task, error) {
	history := d.sessions.get(sessionID)
	if sessionID != "" {
		d.sessions.append(sessionID, llm.Message{Role: "user", Content: goal})
	}

	task := &Task{
		ID:        newTaskID(),
		Goal:      goal,
		AgentName: agentName,
		SessionID: sessionID,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		history:   history,
	}
	d.mu.Lock()
	d.tasks[task.ID] = task
	d.mu.Unlock()

	go d.run(ctx, task)
	return task, nil
}

func (d *Dispatcher) Get(id string) (*Task, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	t, ok := d.tasks[id]
	if !ok {
		return nil, false
	}
	taskCopy := *t
	return &taskCopy, true
}

func (d *Dispatcher) update(id string, fn func(*Task)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.tasks[id]; ok {
		fn(t)
		t.UpdatedAt = time.Now().UTC()
	}
}

func (d *Dispatcher) run(ctx context.Context, task *Task) {
	d.update(task.ID, func(t *Task) { t.Status = StatusRunning })

	if task.AgentName == "" && d.orchestrator != nil {
		result, err := d.orchestrator.Run(ctx, task.Goal, task.history, d)
		d.finalize(task.ID, result, err)
		return
	}

	result, err := d.RunOnAgentWithHistory(ctx, task.AgentName, task.Goal, task.history, false)
	d.finalize(task.ID, result, err)
}

func (d *Dispatcher) finalize(id, result string, err error) {
	var sessionID string
	d.update(id, func(t *Task) {
		sessionID = t.SessionID
		if err != nil {
			t.Status = StatusFailed
			t.Error = err.Error()
			return
		}
		t.Status = StatusDone
		t.Result = result
	})
	if err == nil && result != "" {
		d.sessions.append(sessionID, llm.Message{Role: "assistant", Content: result})
	}
}

func (d *Dispatcher) RunOnAgent(ctx context.Context, agentName, goal string, jsonResponse bool) (string, error) {
	return d.RunOnAgentWithHistory(ctx, agentName, goal, nil, jsonResponse)
}

func (d *Dispatcher) RunOnAgentWithHistory(ctx context.Context, agentName, goal string, history []llm.Message, jsonResponse bool) (string, error) {
	runner, ok := d.runners[agentName]
	if !ok {
		return "", fmt.Errorf("no runner registered for agent %q", agentName)
	}
	if !runner.IsRunning() {
		return "", fmt.Errorf("agent %q is not running", agentName)
	}
	reply := make(chan agent.TaskResult, 1)
	runner.Submit(agent.TaskEnvelope{
		ID:      newTaskID(),
		Goal:    goal,
		Reply:   reply,
		History: history,
		JSON:    jsonResponse,
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-reply:
		return r.Result, r.Err
	}
}

func (d *Dispatcher) FanOut(ctx context.Context, steps []OrchestratedTask) []StepResult {
	var wg sync.WaitGroup
	results := make([]StepResult, len(steps))
	for i, step := range steps {
		wg.Add(1)
		go func(idx int, s OrchestratedTask) {
			defer wg.Done()
			out, err := d.RunOnAgent(ctx, s.Agent, s.Goal, s.JSON)
			results[idx] = StepResult{Agent: s.Agent, Goal: s.Goal, Result: out, Err: err}
		}(i, step)
	}
	wg.Wait()
	return results
}

func (d *Dispatcher) List() []*Task {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]*Task, 0, len(d.tasks))
	for _, t := range d.tasks {
		copy := *t
		out = append(out, &copy)
	}
	return out
}
