# mango-core

**A bare-bones, embeddable multi-agent orchestration engine for Go.**

`mango-core` is the reusable heart extracted from [mango](../). It knows nothing
about Discord, HTTP gateways, config files, or the `MANGO_DIR` filesystem
layout. It does one thing: run a pool of named LLM-backed agents and tools, and
route goals to them — either directly to a named agent, or through an
orchestrator that decomposes a goal and fans it out.

You embed it in any Go program, plug in your own agents, tools, and LLM clients,
and drive it through one small, stable input/output contract.

```
import "github.com/carlosmaranje/mango/core"
```

---

## The Interface: Input & Output

The entire engine is driven through three types.

### Input — `core.Request`

```go
type Request struct {
    Goal      string            // required: the task/prompt to execute
    Agent     string            // target agent; "" routes through the orchestrator
    SessionID string            // per-conversation history key; "" = stateless
    History   []Message         // explicit history; overrides session history when set
    Metadata  map[string]string // arbitrary host tags (uninterpreted by core)
    JSON      bool              // request a JSON-object response (direct-agent path)
}
```

`Goal` is the only required field. Leave `Agent` empty to route through the
orchestrator (the agent registered with `Role: "orchestrator"`); set it to
dispatch straight to that named agent.

### Output — `core.Result`

The polled / synchronous result of a submission:

```go
type Result struct {
    ID        string
    Goal      string
    Agent     string
    SessionID string
    Status    Status    // pending | running | done | failed | cancelled
    Output    string    // the final answer text
    Error     string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### Output (streaming) — `core.Event`

A live lifecycle stream, obtained from `Engine.Subscribe()`:

```go
type Event struct {
    Type  EventType // task.created | task.updated | agent.started | agent.stopped
    Task  *Result   // populated for task.* events
    Agent string    // populated for agent.* events
}
```

---

## The Engine API

```go
eng, err := core.New(core.Options{ /* Tools, Agents, MaxSteps, RunnerInterval */ })

eng.RegisterTool(t)              // add a shared tool        (before Start)
eng.AddAgent(spec)               // add an agent             (before Start)
eng.Start(ctx)                   // materialize + start runners

eng.Submit(ctx, req)             // async  -> initial *Result
eng.SubmitAndWait(ctx, req)      // sync   -> terminal *Result
eng.Wait(ctx, id) / eng.Get(id)  // poll
eng.Cancel(id)                   // cancel a running/pending task
eng.List()                       // all tasks

eng.Agents()                     // []AgentInfo (name, status, skills)
eng.StartAgent(ctx, name) / eng.StopAgent(name)
eng.DefaultAgent()               // first non-orchestrator agent

unsub, events := eng.Subscribe() // <-chan Event; call unsub() to stop

eng.Stop()                       // stop all runners, release resources
```

---

## Minimal Embedding

```go
package main

import (
    "context"
    "fmt"

    "github.com/carlosmaranje/mango/core"
    "github.com/carlosmaranje/mango/core/llm"
)

func main() {
    ctx := context.Background()

    // Any llm.Client works — Anthropic/OpenAI/Ollama ship in core/llm,
    // or implement the one-method interface yourself.
    client, _ := llm.NewClient(llm.ProviderConfig{
        Provider: "anthropic",
        Model:    "claude-sonnet-4-20250514",
        APIKey:   "sk-...",
    })

    eng, _ := core.New(core.Options{
        Agents: []core.AgentSpec{{
            Name:         "worker",
            SystemPrompt: "You are a concise, helpful worker.",
            LLM:          client,
        }},
    })
    _ = eng.Start(ctx)
    defer eng.Stop()

    res, _ := eng.SubmitAndWait(ctx, core.Request{Goal: "Say hello.", Agent: "worker"})
    fmt.Println(res.Output)
}
```

---

## Extension Points (the "plug-in" surface)

`mango-core` is deliberately empty of opinions. The host supplies everything:

| You plug in | Interface | Notes |
|---|---|---|
| **Agents** | `core.AgentSpec` | Fully composed `SystemPrompt` — core does **not** read persona/skill files. |
| **Tools** | `core/tools.Tool` | `Name/Description/Parameters/Returns/Execute`. Shared via `Options.Tools` or per-agent via `AgentSpec.Tools`. |
| **LLM clients** | `core/llm.Client` | One method: `Complete(ctx, CompletionRequest)`. Anthropic/OpenAI/Ollama included. |
| **Memory** | `core/memory.Store` | Optional per-agent KV store; a SQLite implementation is provided. |

A custom tool:

```go
type Upper struct{}
func (Upper) Name() string                  { return "upper" }
func (Upper) Description() string            { return "Uppercase text." }
func (Upper) Returns() string               { return "the uppercased string" }
func (Upper) Parameters() []tools.Parameter { return []tools.Parameter{{Name: "text", Type: "string", Required: true}} }
func (Upper) Execute(ctx context.Context, input string) (string, error) { /* ... */ }

eng.RegisterTool(Upper{}) // before Start; available to every agent
```

See [`engine_test.go`](./engine_test.go) for runnable, dependency-free examples
of the direct-agent path, the full tool-call loop, the orchestrator path, and
the event stream.

---

## Package Layout

| Package | Role |
|---|---|
| `core` | The embeddable facade: `Engine`, `Request`, `Result`, `Event`, `Options`, `AgentSpec`. |
| `core/agent` | `Agent`, `Registry`, `Runner` (the per-agent LLM + tool loop). |
| `core/orchestrator` | `Dispatcher`, `Orchestrator` (ReAct decomposition), `EventBus`, `Task`. |
| `core/llm` | `Client` interface + Anthropic / OpenAI / Ollama clients. |
| `core/tools` | `Tool` interface + `Registry`. Ships **no** concrete tools. |
| `core/memory` | `Store` interface + SQLite implementation. |

`core/*` never imports the host (`mango`) packages — the module boundary
enforces it. That is what makes the core reusable in a new project unchanged.
