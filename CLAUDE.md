# Mango — Project Guide

## What It Is

**Mango** is a multi-agent AI orchestration gateway. It runs a persistent background server that manages a pool of named AI agents, accepts goals from a Discord bot or a local CLI, and routes them either directly to a named agent or through an LLM-powered orchestrator that decomposes goals into parallel sub-tasks.

---

## Architecture Overview

```
CLI (mango <cmd>)          Browser / Remote Client
       │                            │
       ▼ HTTP/Unix socket           ▼ HTTP/TCP (http_addr) + SSE (/events)
  Gateway Server  ──────────────────────────────────────
       │                                                │
       ▼                                                ▼
  Dispatcher ──► Orchestrator (orchestrator agent)    Discord Bot
       │              │                                │
       ▼              ▼ FanOut                         ▼
   Agent Runners  (parallel goroutines)        router.Resolve(channelID)
       │
       ▼
   LLM Client (Anthropic / OpenAI / Ollama)
       │
       ▼
   Memory Store (SQLite per agent)
```

---

## Two Modules: `mango-core` + `mango`

The repo is split into two Go modules:

- **`core/` — `github.com/carlosmaranje/mango/core`**: a reusable, embeddable
  multi-agent orchestration engine with **zero** knowledge of Discord, HTTP,
  config files, or `MANGO_DIR`. Driven entirely through one input/output
  contract (`core.Request` / `core.Result` / `core.Event`) and the `core.Engine`
  facade. See [`core/README.md`](core/README.md). Wired into the app via a
  `replace` directive in the root `go.mod`.
- **`mango` (root module)**: the application — CLI, config, gateway, Discord bot,
  concrete tools — built **on top of** `core`. It plugs its tools, LLM clients,
  and composed personas into a `core.Engine`.

### `mango-core` Packages (`core/`)

| Package | Role |
|---|---|
| `core` | Embeddable facade: `Engine`, `Request`, `Result`, `Event`, `Options`, `AgentSpec` |
| `core/agent` | Agent registry and runner, including the LLM/tool loop and invocation limits |
| `core/orchestrator` | In-memory tasks/sessions, dispatcher, fan-out, and ReAct-style orchestrator |
| `core/event` | Non-blocking event bus and task/agent/step/tool event vocabulary |
| `core/llm` | LLM interface + Anthropic/OpenAI/Ollama clients |
| `core/memory` | SQLite key-value store per agent (`Store` interface) |
| `core/tools` | Tool interface, registry, definitions, and retry/timeout execution policy |
| `core/tools/scratchpad` | Optional durable scratchpad over an agent's memory store |

### `mango` App Packages (`internal/`, `cmd/`)

| Package | Role |
|---|---|
| `cmd/app` | Cobra CLI, Bubble Tea TUI, config commands, Unix-socket client, and engine assembly |
| `internal/gateway` | Unix/TCP HTTP server and REST/SSE adapters over `core.Engine` |
| `internal/discord` | Discord bot and channel router over `core.Engine` |
| `internal/agentdef` | Loads agent `.md` personas + composes system prompts from skills (MANGO_DIR) |
| `internal/skill` | Skill definition loader (MANGO_DIR/skills) |
| `internal/constants` | `MANGO_DIR` path resolution |
| `internal/tools` | Application tools: `gosolar`, `datetime`, and per-agent `identity` |

---

## Directory Layout

All runtime files live under a single root called `MANGO_DIR`.

| Env var | Default | Fallback |
|---|---|---|
| `MANGO_DIR` | `~/.mango` | `/etc/mango` (when `$HOME` is unavailable) |

`MANGO_DIR` is the **only** env var that controls paths. There is no `MANGO_CONFIG`, `MANGO_SOCKET_PATH`, `MANGO_AGENTS_DIR`, or `MANGO_SKILLS_DIR`.

| Path | Purpose | Override |
|---|---|---|
| `$MANGO_DIR/config.yaml` | Main config file | `--config` flag |
| `$MANGO_DIR/agents/<NAME>.md` | Agent definition (uppercase) | — |
| `$MANGO_DIR/agents/<name>/` | Agent work dir (SQLite memory) | — |
| `$MANGO_DIR/skills/<name>.md` | Skill definition | — |
| `$MANGO_DIR/mango.sock` *(macOS/Windows)* | Unix socket | `socket_path` in config |
| `/var/run/mango/mango.sock` *(Linux)* | Unix socket | `socket_path` in config |

---

## Configuration Path Priority

1. **Explicit flag**: `--config /path/to/config.yaml`
2. **`$MANGO_DIR/config.yaml`** (MANGO_DIR defaults to `~/.mango`)
3. **`/etc/mango/config.yaml`** (system-wide fallback)
4. **`./config/config.yaml`**
5. **`./config.yaml`**

When no config file is found, reads use defaults and future config-writing commands target `$MANGO_DIR/config.yaml`.

---

## Socket Path Configuration

The Unix socket path can be configured in two ways, in order of priority:

1. **Config file** (`config.yaml`): `socket_path: /path/to/socket`
2. **Platform default**:
   - macOS / Windows: `$MANGO_DIR/mango.sock`
   - Linux: `/var/run/mango/mango.sock`

To use a custom path, set it in config:
```yaml
socket_path: /tmp/mango.sock
```

---

## HTTP / Browser Access

Set `http_addr` to expose the gateway over TCP (required for browser clients and remote access):

```yaml
http_addr: "127.0.0.1:9696"
```

When set, the gateway binds both the Unix socket and a TCP listener. The shipped `config/config.default.yaml` currently sets `http_addr: ":9696"`, which listens on all interfaces. Set it to `""` to disable TCP.

**Security:** the TCP API has no authentication or authorization and applies `Access-Control-Allow-Origin: *`. Anyone who can reach it can submit/cancel tasks and start/stop agents. Keep it disabled, bind it to loopback, or place it behind a trusted authenticated reverse proxy; never expose it directly to an untrusted network.

### REST API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check → `{"ok":true}` |
| `GET` | `/agents` | List agents with status and skills |
| `POST` | `/agents/start` | Start a named agent |
| `POST` | `/agents/stop` | Stop a named agent |
| `GET` | `/tasks` | List all tasks |
| `POST` | `/tasks` | Submit a task (async, returns 202 with task ID) |
| `GET` | `/tasks/{id}` | Poll a task by ID |
| `DELETE` | `/tasks/{id}` | Cancel a pending or running task |
| `POST` | `/chat` | Submit a task and **block** until complete (sync) |
| `GET` | `/events` | SSE stream of task and agent lifecycle events |

### SSE Event Types (`GET /events`)

```
event: task.created   data: {id, goal, agent_name, session_id, status, created_at}
event: task.updated   data: {id, status, result?, error?, updated_at}
event: agent.started  data: {name}
event: agent.stopped  data: {name}
event: ping           data: {}  ← sent immediately and then every 15s
```

`mango-core` can also emit `step.started`, `step.completed`, `tool.called`, `tool.completed`, and `invocation.stopped` when `Options.EmitStepEvents` is enabled. The Mango host does not currently enable these events, and the gateway's SSE serializer does not expose their detail payload yet.

### Task Status Values

`pending` → `running` → `done` | `failed` | `cancelled`

Cancel a running task: `DELETE /tasks/{id}` → `{"id":"...","status":"cancelled"}`

---

## Installation (Linux)

`install.sh` builds the binary, installs the systemd unit, creates the `mango` system user, and writes a starter config to `/etc/mango/config.yaml` from `config/config.default.yaml` (orchestrator + worker scaffolding with empty LLM fields). It also seeds the agent definition files by copying `config/agents/<name>.md` → `/etc/mango/agents/<name>.md` for any agent whose definition file doesn't yet exist. It then runs two optional interactive prompts:

- **Discord setup**: asks for a bot token, then whether to bind the bot globally (all channels → orchestrator) or to a comma-separated list of channel IDs (each bound to a chosen agent, default `worker`). A `discord:` block (and `bindings:` if channels were provided) is prepended to the installed config.
- **LLM setup**: for each of `orchestrator` and `worker`, prompts for provider / model / api_key / base_url and applies them via `mango config agent edit`. Leaving provider blank skips that agent.

Skipping either step prints an `ACTION REQUIRED` block with the file path to edit and the `systemctl daemon-reload && systemctl restart mango` commands to apply changes.

---

## Agent Personalities & Skills — Definition Files

Host-defined agent prompts are assembled at startup by combining agent definition files with skill definitions. Core adds its orchestration protocol at invocation time.

### Agent Definition Files

Each agent has a corresponding `.md` file (e.g., `ORCHESTRATOR.md`, `WORKER.md`, `researcher.md`) in the agents directory (`$MANGO_DIR/agents/`). For the default install:
- Orchestrator: `$MANGO_DIR/agents/ORCHESTRATOR.md`
- Worker: `$MANGO_DIR/agents/WORKER.md`

- **No hardcoded personas.** At startup, `serve.go` reads each agent's definition file, trims it, appends any skills' definitions (in order), and sets `Agent.SystemPrompt`. Startup fails hard if the file is missing or empty — this is intentional: an agent with no persona should not silently run with stub behavior.
- **Orchestrator definition** contains only its host-defined persona and delegation strategy. `mango-core` automatically appends `orchestrator.ProtocolPrompt`, which owns the `action` / `tasks` / `final` JSON contract expected by `parseOrchestratorResponse`, followed by the live agent catalog. The orchestrator also requests JSON mode from the LLM provider when possible. Don't duplicate the protocol or catalog in the `.md` file.
- **Worker / custom agents** can contain any persona, tone, tool-use guidelines, etc. Edit the agent definition file, then `sudo systemctl restart mango` to reload.

### Skills

Skills are reusable system prompt snippets stored as `.md` files in the skills directory (`$MANGO_DIR/skills/`). Skills are declared in the agent config and their definitions are automatically appended to the agent's system prompt at startup, in the order listed.

Example skill definition:
```markdown
# Web Search Skill

You have access to a web search tool. Use it to find current information.

## Guidelines
- Search for recent information when needed
- Cite sources in your responses
```

To use a skill, list it in the agent config:
```yaml
agents:
  - name: researcher
    skills:
      - web_search
      - code_analysis
```

At startup, the system prompt is assembled as:
```
[researcher.md content]

---

[web_search.md content]

---

[code_analysis.md content]
```

---

## Logical Order to Run It

### 1. Configuration
Edit `/etc/mango/config.yaml` (or use the `mango config` CLI) to define your agents, LLM providers, and optionally Discord and bindings. The repo ships `config/config.default.yaml` as the minimal two-agent (orchestrator + worker) starter used by `install.sh`.

```yaml
agents:
  - name: orchestrator
    role: orchestrator              # marks this agent as the task decomposer
    llm:
      provider: anthropic
      model: claude-sonnet-4-20250514
      api_key: ${ANTHROPIC_API_KEY}

  - name: researcher
    skills: [web_search, summarize]  # skills are appended to agent's system prompt
    llm:
      provider: ollama
      model: qwen2.5-coder

discord:
  token: ${DISCORD_TOKEN}           # optional

bindings:
  - channel_id: "123456"
    agent: researcher               # route that channel to a specific agent
```

### 2. Start the Server
```bash
mango serve
# or with explicit config:
mango serve --config /path/to/config.yaml
```
This boots the full pipeline:
- Starts all agent runner goroutines
- Starts the Unix socket HTTP gateway
- Starts the Discord bot (if token configured)

### 3. Open the TUI

With the gateway running in another process, launch the interactive UI with no subcommand:

```bash
mango
```

Chat is the default panel. Chat uses synchronous `POST /chat`; Tasks uses asynchronous `POST /tasks`; Agents shows live runner status; Config lists common management commands. Both Chat and Tasks currently use the shared in-memory session ID `tui`.

### 4. Check Health
```bash
mango status
# → gateway: ok (socket=/Users/.../.mango/mango.sock)
```

### 5. List Agents
```bash
mango agent list
```

### 6. Submit a Task (CLI)
```bash
# Route to a specific agent:
mango task submit "Summarize Go 1.24 release notes" --agent researcher

# Route through orchestrator (orchestrator decomposes + fans out):
mango task submit "Research and summarize Go 1.24 features"

# Submit and wait for result:
mango task submit "..." --wait
```

### 7. Check Task Status
```bash
mango task status <task-id>
```

### 8. Discord (optional)
With `discord.token` set and channel bindings configured, users can message the bot in Discord. Messages in bound channels go directly to the named agent; unbound channels route through the orchestrator.

While the model is thinking, the bot refreshes Discord's typing indicator every eight seconds. The Discord channel ID becomes the core `SessionID`, so both direct-agent and orchestrator follow-ups use the dispatcher's latest 200 in-memory messages. This history is lost on restart. `internal/discord/context.go` remains in the tree but is not used by the live bot path.

---

## Request Flow (CLI Path)

```
mango task submit "goal"
  └─► POST /tasks  (HTTP over Unix socket)
        └─► core.Engine.Submit(core.Request)
              └─► dispatcher.Submit(TaskRequest)
                    ├─ Agent set → RunOnAgentWithHistory → runner tool loop → LLM.Complete
                    └─ Agent ""  → orchestrator.Run(goal, history, d) (ReAct loop, max 5 rounds)
                    └─► dispatcher.FanOut (parallel agent goroutines)
                          └─► each runner → LLM → result
                    └─► orchestrator LLM synthesizes final answer
```

`POST /chat` fills an empty agent with `Engine.DefaultAgent()` and then calls `SubmitAndWait`, so chat bypasses orchestration unless the request explicitly names the orchestrator. With multiple non-orchestrator agents, callers should name one because the current default depends on map iteration order.

---

## External Dependencies

| Service | Config key | Required? |
|---|---|---|
| Anthropic API | `api_key: ${ANTHROPIC_API_KEY}` per agent | If using Anthropic |
| OpenAI API | `api_key` + `base_url` per agent | If using OpenAI |
| Ollama (local) | `base_url: http://localhost:11434/v1` | If using local LLMs |
| Discord | `discord.token: ${DISCORD_TOKEN}` | Optional |
| SQLite | auto-created at `work_dir/memory.db` | Auto |

---

## Built-in Tools and Core Capabilities

- **`gosolar`** (shared): calculates solar position, angles, sunrise, sunset, solar noon, and related orbital data.
- **`datetime`** (shared): resolves current/local time and calendar information for a date and IANA timezone.
- **`identity`** (per agent): reports the running agent, host, OS/architecture, work directory, socket/config paths, and uptime.
- **`scratchpad`** (core, opt-in): durable namespaced `get`/`set`/`list`/`delete` operations over `memory.Store`. The Mango host does not currently enable it.

The runner supports up to 12 tool-loop steps by default, context windowing, no-progress detection, optional invocation deadlines/token budgets, optional tool timeouts/retries, and opt-in step/tool events. Mango's YAML does not yet expose `agent.Limits`, `EnableScratchpad`, or `EmitStepEvents`, so the host uses one tool attempt, no scratchpad, and coarse events.

SQLite provides per-agent key-value storage. Task records and conversation sessions remain in memory and are not restored after restart.

## Notable Gaps (Current State)
- Orchestrator fails hard if a goal takes more than five orchestration rounds.
- Anthropic prompt caching is not enabled; each turn re-sends retained history uncached.
- The default starter config exposes unauthenticated, wildcard-CORS TCP on `:9696`.
- Tasks and session history are process-local rather than persisted.
- Direct chat's implicit worker selection is nondeterministic when several workers exist.
- Core invocation limits, retry policy, scratchpad, and detailed events are not configurable from Mango YAML yet.
- Tests are strongest in `core`; `cmd/app`, `internal/gateway`, the live Discord bot flow, and host tools have little or no direct coverage.
