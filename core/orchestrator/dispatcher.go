package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/carlosmaranje/mango/core/agent"
	"github.com/carlosmaranje/mango/core/llm"
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

	mu      sync.RWMutex
	tasks   map[string]*Task
	waiters map[string][]chan struct{}

	orchestrator *Orchestrator
	sessions     *sessionStore
	bus          *EventBus
}

func NewDispatcher(reg *agent.Registry, runners map[string]*agent.Runner, orch *Orchestrator, bus *EventBus) *Dispatcher {
	return &Dispatcher{
		registry:     reg,
		runners:      runners,
		tasks:        make(map[string]*Task),
		waiters:      make(map[string][]chan struct{}),
		orchestrator: orch,
		sessions:     newSessionStore(),
		bus:          bus,
	}
}

func newTaskID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// TaskRequest is the dispatcher-level input for submitting a task. History
// overrides the session-derived history when non-nil; JSON requests a JSON
// response on the direct-agent path.
type TaskRequest struct {
	Goal      string
	AgentName string
	SessionID string
	History   []llm.Message
	JSON      bool
}

func (d *Dispatcher) Submit(ctx context.Context, req TaskRequest) (*Task, error) {
	history := req.History
	if history == nil {
		history = d.sessions.get(req.SessionID)
	}
	if req.SessionID != "" {
		d.sessions.append(req.SessionID, llm.Message{Role: "user", Content: req.Goal})
	}

	taskCtx, cancel := context.WithCancel(ctx)
	task := &Task{
		ID:        newTaskID(),
		Goal:      req.Goal,
		AgentName: req.AgentName,
		SessionID: req.SessionID,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		history:   history,
		json:      req.JSON,
		cancel:    cancel,
	}
	d.mu.Lock()
	d.tasks[task.ID] = task
	snapshot := *task
	d.mu.Unlock()

	if d.bus != nil {
		busCopy := snapshot
		d.bus.Emit(Event{Type: "task.created", Payload: &busCopy})
	}

	// run mutates the stored task under the lock; hand the caller an immutable
	// snapshot so reads don't race with the goroutine below.
	go d.run(taskCtx, task)
	return &snapshot, nil
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
	// If already cancelled before we start, go straight to finalize.
	select {
	case <-ctx.Done():
		d.finalize(task.ID, "", ctx.Err())
		return
	default:
	}

	d.update(task.ID, func(t *Task) { t.Status = StatusRunning })

	if task.AgentName == "" {
		if d.orchestrator == nil {
			d.finalize(task.ID, "", fmt.Errorf("no agent specified and no orchestrator configured — check your config"))
			return
		}
		result, err := d.orchestrator.Run(ctx, task.Goal, task.history, d)
		d.finalize(task.ID, result, err)
		return
	}

	result, err := d.RunOnAgentWithHistory(ctx, task.AgentName, task.Goal, task.history, task.json)
	d.finalize(task.ID, result, err)
}

func (d *Dispatcher) finalize(id, result string, err error) {
	var sessionID string
	var taskCopy Task
	d.mu.Lock()
	if t, ok := d.tasks[id]; ok {
		if t.Status != StatusCancelled {
			sessionID = t.SessionID
			if err != nil {
				t.Status = StatusFailed
				t.Error = err.Error()
			} else {
				t.Status = StatusDone
				t.Result = result
			}
		}
		t.UpdatedAt = time.Now().UTC()
		if t.cancel != nil {
			t.cancel()
		}
		taskCopy = *t
	}
	waiters := d.waiters[id]
	delete(d.waiters, id)
	d.mu.Unlock()

	for _, ch := range waiters {
		close(ch)
	}

	if d.bus != nil {
		d.bus.Emit(Event{Type: "task.updated", Payload: &taskCopy})
	}

	if err == nil && result != "" {
		d.sessions.append(sessionID, llm.Message{Role: "assistant", Content: result})
	}
}

// Wait blocks until the task with the given ID reaches a terminal state or ctx is done.
func (d *Dispatcher) Wait(ctx context.Context, id string) (*Task, error) {
	ch := make(chan struct{})

	d.mu.Lock()
	t, ok := d.tasks[id]
	if !ok {
		d.mu.Unlock()
		return nil, fmt.Errorf("task not found")
	}
	switch t.Status {
	case StatusDone, StatusFailed, StatusCancelled:
		copy := *t
		d.mu.Unlock()
		return &copy, nil
	}
	d.waiters[id] = append(d.waiters[id], ch)
	d.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ch:
		t, ok := d.Get(id)
		if !ok {
			return nil, fmt.Errorf("task not found")
		}
		return t, nil
	}
}

// Cancel cancels a pending or running task by ID.
func (d *Dispatcher) Cancel(id string) (*Task, error) {
	d.mu.Lock()
	t, ok := d.tasks[id]
	if !ok {
		d.mu.Unlock()
		return nil, fmt.Errorf("task not found")
	}
	switch t.Status {
	case StatusDone, StatusFailed, StatusCancelled:
		d.mu.Unlock()
		return nil, fmt.Errorf("task not running")
	}
	t.Status = StatusCancelled
	t.UpdatedAt = time.Now().UTC()
	cancel := t.cancel
	taskCopy := *t
	d.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if d.bus != nil {
		d.bus.Emit(Event{Type: "task.updated", Payload: &taskCopy})
	}

	return &taskCopy, nil
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

// DefaultAgentName returns the name of the first non-orchestrator runner,
// used as the default target for direct (chat) requests.
func (d *Dispatcher) DefaultAgentName() string {
	orchName := ""
	if d.orchestrator != nil {
		orchName = d.orchestrator.Agent.Name
	}
	for name := range d.runners {
		if name != orchName {
			return name
		}
	}
	return orchName
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
