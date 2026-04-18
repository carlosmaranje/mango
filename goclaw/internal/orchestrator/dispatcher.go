package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/carlosmaranje/goclaw/internal/agent"
)

type Dispatcher struct {
	registry *agent.Registry
	runners  map[string]*agent.Runner

	mu    sync.RWMutex
	tasks map[string]*Task

	planner *Planner
}

func NewDispatcher(reg *agent.Registry, runners map[string]*agent.Runner, planner *Planner) *Dispatcher {
	return &Dispatcher{
		registry: reg,
		runners:  runners,
		tasks:    make(map[string]*Task),
		planner:  planner,
	}
}

func newTaskID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (d *Dispatcher) Submit(ctx context.Context, goal, agentName string) (*Task, error) {
	task := &Task{
		ID:        newTaskID(),
		Goal:      goal,
		AgentName: agentName,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
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
	_copy := *t
	return &_copy, true
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

	if task.AgentName == "" && d.planner != nil {
		result, err := d.planner.Run(ctx, task.Goal, d)
		d.finalize(task.ID, result, err)
		return
	}

	result, err := d.RunOnAgent(ctx, task.AgentName, task.Goal)
	d.finalize(task.ID, result, err)
}

func (d *Dispatcher) finalize(id, result string, err error) {
	d.update(id, func(t *Task) {
		if err != nil {
			t.Status = StatusFailed
			t.Error = err.Error()
			return
		}
		t.Status = StatusDone
		t.Result = result
	})
}

func (d *Dispatcher) RunOnAgent(ctx context.Context, agentName, goal string) (string, error) {
	runner, ok := d.runners[agentName]
	if !ok {
		return "", fmt.Errorf("no runner registered for agent %q", agentName)
	}
	if !runner.IsRunning() {
		return "", fmt.Errorf("agent %q is not running", agentName)
	}
	reply := make(chan agent.TaskResult, 1)
	runner.Submit(agent.TaskEnvelope{
		ID:    newTaskID(),
		Goal:  goal,
		Reply: reply,
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-reply:
		return r.Result, r.Err
	}
}

func (d *Dispatcher) FanOut(ctx context.Context, steps []PlannedTask) []StepResult {
	var wg sync.WaitGroup
	results := make([]StepResult, len(steps))
	for i, step := range steps {
		wg.Add(1)
		go func(idx int, s PlannedTask) {
			defer wg.Done()
			out, err := d.RunOnAgent(ctx, s.Agent, s.Goal)
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
