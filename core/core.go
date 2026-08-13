package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/carlosmaranje/mango/core/agent"
	"github.com/carlosmaranje/mango/core/event"
	"github.com/carlosmaranje/mango/core/llm"
	"github.com/carlosmaranje/mango/core/memory"
	"github.com/carlosmaranje/mango/core/orchestrator"
	"github.com/carlosmaranje/mango/core/tools"
	"github.com/carlosmaranje/mango/core/tools/scratchpad"
)

// AgentSpec declaratively describes one agent for the engine to materialize.
//
// The host portion of the system prompt is supplied fully composed: mango-core
// does not read persona/skill files. Hosts assemble personas however they like
// and pass the result here. For an orchestrator, core automatically appends its
// response protocol and live agent catalog. An empty SystemPrompt is rejected
// — an agent with no persona must never silently run with stub behavior.
type AgentSpec struct {
	// Name uniquely identifies the agent within the engine.
	Name string
	// SystemPrompt is the host-composed persona and instructions. Required. Core
	// appends the orchestration protocol when Role is "orchestrator".
	SystemPrompt string
	// LLM is the chat client backing this agent. Required.
	LLM llm.Client
	// Role marks special agents. "orchestrator" designates the task decomposer.
	Role string
	// Skills are descriptive labels surfaced in the orchestrator's agent catalog
	// and queryable via Registry.FindBySkill. They do not load any content here.
	Skills []string
	// MaxTokens caps the per-completion token budget (0 => engine/agent default).
	MaxTokens int
	// WorkDir, when set and Memory is nil, opens a SQLite store under that dir.
	WorkDir string
	// Memory is an explicit key-value store for the agent (optional).
	Memory memory.Store
	// AuthCreds are arbitrary per-agent credentials made available to tools.
	AuthCreds map[string]string
	// Tools are extra tools available only to this agent, layered on top of the
	// engine's shared tools.
	Tools []tools.Tool
	// Limits bounds this agent's tool loop (steps, tokens, deadline, no-progress,
	// per-tool retry). Zero values are filled with generous defaults.
	Limits agent.Limits
	// EnableScratchpad registers the durable scratchpad tool for this agent when a
	// memory store is available. Overrides Options.EnableScratchpad when true.
	EnableScratchpad bool
}

const orchestratorRole = "orchestrator"

// Options configures a new Engine.
type Options struct {
	// Tools are shared tools cloned into every agent's tool registry.
	Tools []tools.Tool
	// Agents are the agents to materialize on Start.
	Agents []AgentSpec
	// MaxSteps caps orchestrator decomposition rounds (0 => orchestrator default).
	MaxSteps int
	// RunnerInterval is the agent heartbeat interval (0 => runner default).
	RunnerInterval time.Duration
	// EnableScratchpad registers the durable scratchpad tool for every agent that
	// has a memory store. Per-agent AgentSpec.EnableScratchpad can also opt in.
	EnableScratchpad bool
	// EmitStepEvents attaches the event bus to agent runners so step-/tool-level
	// events flow to Subscribe. Default off: the stream then carries only the
	// historical task.*/agent.* events (preserving existing consumers byte-for-byte).
	EmitStepEvents bool
}

// Engine is the embeddable mango-core orchestrator. It owns the agent registry,
// runners, tool registry, dispatcher, orchestrator and event bus, and exposes
// the Request/Result/Event contract. The zero value is not usable; call New.
type Engine struct {
	mu sync.Mutex

	specs            []AgentSpec
	specNames        map[string]struct{}
	sharedTools      *tools.Registry
	maxSteps         int
	interval         time.Duration
	enableScratchpad bool
	emitStepEvents   bool

	registry   *agent.Registry
	runners    map[string]*agent.Runner
	bus        *event.Bus
	orch       *orchestrator.Orchestrator
	dispatcher *orchestrator.Dispatcher
	closers    []func() error
	started    bool
}

// New builds an Engine from Options. Agents are validated but not started until
// Start is called. Additional tools/agents may be registered before Start.
func New(opts Options) (*Engine, error) {
	e := &Engine{
		specNames:        make(map[string]struct{}),
		sharedTools:      tools.NewRegistry(),
		maxSteps:         opts.MaxSteps,
		interval:         opts.RunnerInterval,
		enableScratchpad: opts.EnableScratchpad,
		emitStepEvents:   opts.EmitStepEvents,
		registry:         agent.NewRegistry(),
		runners:          make(map[string]*agent.Runner),
		bus:              event.NewBus(),
	}
	for _, t := range opts.Tools {
		if err := e.RegisterTool(t); err != nil {
			return nil, err
		}
	}
	for _, spec := range opts.Agents {
		if err := e.AddAgent(spec); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// RegisterTool adds a shared tool available to all agents. Call before Start.
func (e *Engine) RegisterTool(t tools.Tool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return fmt.Errorf("cannot register tool after Start")
	}
	return e.sharedTools.Register(t)
}

// AddAgent registers an agent spec. Call before Start.
func (e *Engine) AddAgent(spec AgentSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return fmt.Errorf("cannot add agent after Start")
	}
	if spec.Name == "" {
		return fmt.Errorf("agent spec has empty name")
	}
	if _, dup := e.specNames[spec.Name]; dup {
		return fmt.Errorf("agent %q already added", spec.Name)
	}
	e.specNames[spec.Name] = struct{}{}
	e.specs = append(e.specs, spec)
	return nil
}

// Start materializes all agents into runners, wires the orchestrator and
// dispatcher, and starts the runner goroutines. It is not safe to call twice.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return fmt.Errorf("engine already started")
	}

	var orchAgent *agent.Agent
	for _, spec := range e.specs {
		a, runner, err := e.buildAgent(spec)
		if err != nil {
			return err
		}
		if err := e.registry.Register(a); err != nil {
			return err
		}
		e.runners[a.Name] = runner
		if spec.Role == orchestratorRole {
			orchAgent = a
		}
	}

	if orchAgent != nil {
		e.orch = orchestrator.NewOrchestrator(orchAgent, e.registry)
		if e.maxSteps > 0 {
			e.orch.MaxSteps = e.maxSteps
		}
	}
	e.dispatcher = orchestrator.NewDispatcher(e.registry, e.runners, e.orch, e.bus)

	for _, runner := range e.runners {
		if err := runner.Start(ctx); err != nil {
			return err
		}
	}
	e.started = true
	return nil
}

func (e *Engine) buildAgent(spec AgentSpec) (*agent.Agent, *agent.Runner, error) {
	if spec.LLM == nil {
		return nil, nil, fmt.Errorf("agent %q has no LLM client", spec.Name)
	}
	if spec.SystemPrompt == "" {
		return nil, nil, fmt.Errorf("agent %q has an empty system prompt", spec.Name)
	}

	mem := spec.Memory
	if mem == nil && spec.WorkDir != "" {
		opened, err := memory.Open(spec.WorkDir)
		if err != nil {
			return nil, nil, fmt.Errorf("agent %q memory: %w", spec.Name, err)
		}
		mem = opened
		e.closers = append(e.closers, opened.Close)
	}

	a := &agent.Agent{
		Name:         spec.Name,
		WorkDir:      spec.WorkDir,
		LLM:          spec.LLM,
		Skills:       spec.Skills,
		SystemPrompt: spec.SystemPrompt,
		Memory:       mem,
		AuthCreds:    spec.AuthCreds,
		MaxTokens:    spec.MaxTokens,
		Limits:       spec.Limits,
	}

	toolReg := e.sharedTools.Clone()
	for _, t := range spec.Tools {
		if err := toolReg.Register(t); err != nil {
			return nil, nil, fmt.Errorf("agent %q: register tool: %w", spec.Name, err)
		}
	}
	if mem != nil && (e.enableScratchpad || spec.EnableScratchpad) {
		if err := toolReg.Register(scratchpad.New(mem, spec.Name)); err != nil {
			return nil, nil, fmt.Errorf("agent %q: register scratchpad: %w", spec.Name, err)
		}
	}

	runner := agent.NewRunner(a, toolReg, e.interval)
	if e.emitStepEvents {
		runner.SetBus(e.bus)
	}
	return a, runner, nil
}

// Stop stops all runners and releases per-agent resources (e.g. memory stores).
func (e *Engine) Stop() {
	e.mu.Lock()
	runners := make([]*agent.Runner, 0, len(e.runners))
	for _, r := range e.runners {
		runners = append(runners, r)
	}
	closers := e.closers
	e.mu.Unlock()

	for _, r := range runners {
		r.Stop()
	}
	for _, c := range closers {
		_ = c()
	}
}

func (e *Engine) requireStarted() error {
	if e.dispatcher == nil {
		return fmt.Errorf("engine not started")
	}
	return nil
}

// Submit dispatches a Request asynchronously and returns the initial Result.
func (e *Engine) Submit(ctx context.Context, req Request) (*Result, error) {
	if err := e.requireStarted(); err != nil {
		return nil, err
	}
	task, err := e.dispatcher.Submit(ctx, orchestrator.TaskRequest{
		Goal:      req.Goal,
		AgentName: req.Agent,
		SessionID: req.SessionID,
		History:   req.History,
		JSON:      req.JSON,
	})
	if err != nil {
		return nil, err
	}
	return taskToResult(task), nil
}

// SubmitAndWait dispatches a Request and blocks until it reaches a terminal
// state or ctx is done.
func (e *Engine) SubmitAndWait(ctx context.Context, req Request) (*Result, error) {
	res, err := e.Submit(ctx, req)
	if err != nil {
		return nil, err
	}
	return e.Wait(ctx, res.ID)
}

// Wait blocks until the identified task is terminal or ctx is done.
func (e *Engine) Wait(ctx context.Context, id string) (*Result, error) {
	if err := e.requireStarted(); err != nil {
		return nil, err
	}
	task, err := e.dispatcher.Wait(ctx, id)
	if err != nil {
		return nil, err
	}
	return taskToResult(task), nil
}

// Get returns the current Result for a task ID.
func (e *Engine) Get(id string) (*Result, bool) {
	if e.dispatcher == nil {
		return nil, false
	}
	task, ok := e.dispatcher.Get(id)
	if !ok {
		return nil, false
	}
	return taskToResult(task), true
}

// Cancel cancels a pending or running task.
func (e *Engine) Cancel(id string) (*Result, error) {
	if err := e.requireStarted(); err != nil {
		return nil, err
	}
	task, err := e.dispatcher.Cancel(id)
	if err != nil {
		return nil, err
	}
	return taskToResult(task), nil
}

// List returns all known tasks.
func (e *Engine) List() []*Result {
	if e.dispatcher == nil {
		return nil
	}
	tasks := e.dispatcher.List()
	out := make([]*Result, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskToResult(t))
	}
	return out
}

// DefaultAgent returns the name of the first non-orchestrator agent, the
// conventional target for direct/chat requests.
func (e *Engine) DefaultAgent() string {
	if e.dispatcher == nil {
		return ""
	}
	return e.dispatcher.DefaultAgentName()
}

// Agents returns each registered agent with its current run status and skills.
func (e *Engine) Agents() []AgentInfo {
	out := make([]AgentInfo, 0, len(e.runners))
	for _, a := range e.registry.List() {
		status := "stopped"
		if r, ok := e.runners[a.Name]; ok && r.IsRunning() {
			status = "running"
		}
		out = append(out, AgentInfo{Name: a.Name, Status: status, Skills: a.Skills})
	}
	return out
}

// StartAgent starts a stopped agent's runner and emits an agent.started event.
func (e *Engine) StartAgent(ctx context.Context, name string) error {
	r, ok := e.runners[name]
	if !ok {
		return fmt.Errorf("agent %q not found", name)
	}
	if err := r.Start(ctx); err != nil {
		return err
	}
	e.bus.Emit(event.Event{Type: event.TypeAgentStarted, Payload: map[string]string{"name": name}})
	return nil
}

// StopAgent stops a running agent's runner and emits an agent.stopped event.
func (e *Engine) StopAgent(name string) error {
	r, ok := e.runners[name]
	if !ok {
		return fmt.Errorf("agent %q not found", name)
	}
	r.Stop()
	e.bus.Emit(event.Event{Type: event.TypeAgentStopped, Payload: map[string]string{"name": name}})
	return nil
}

// Subscribe returns a stream of lifecycle Events and an unsubscribe function.
// The returned channel is closed when unsubscribe is called.
func (e *Engine) Subscribe() (unsubscribe func(), events <-chan Event) {
	id, raw := e.bus.Subscribe()
	out := make(chan Event, 64)
	done := make(chan struct{})

	go func() {
		defer close(out)
		for {
			select {
			case <-done:
				return
			case ev, ok := <-raw:
				if !ok {
					return
				}
				select {
				case out <- translateEvent(ev):
				case <-done:
					return
				}
			}
		}
	}()

	var once sync.Once
	unsubscribe = func() {
		once.Do(func() {
			e.bus.Unsubscribe(id)
			close(done)
		})
	}
	return unsubscribe, out
}
