# Mango — Project Guide

## What It Is

**Mango** is a multi-agent AI orchestration gateway. It runs a persistent background server that manages a pool of named AI agents, accepts goals from a Discord bot or a local CLI, and routes them either directly to a named agent or through an LLM-powered orchestrator that decomposes goals into parallel sub-tasks.

---

## Architecture Overview

```
CLI (mango <cmd>)
       │
       ▼ HTTP over Unix socket (~/.mango/mango.sock)
  Gateway Server  ─────────────────────────────────────
       │                                               │
       ▼                                               ▼
  Dispatcher ──► Planner (orchestrator agent)    Discord Bot
       │              │                               │
       ▼              ▼ FanOut                        ▼
   Agent Runners  (parallel goroutines)       router.Resolve(channelID)
       │
       ▼
   LLM Client (Anthropic / OpenAI / Ollama)
       │
       ▼
   Memory Store (SQLite per agent)
```

---

## Key Packages

| Package | Role |
|---|---|
| `cmd/app` | CLI entry point, Cobra commands, Unix socket HTTP client |
| `internal/gateway` | Unix socket HTTP server + route handlers |
| `internal/agent` | Agent struct, Registry, Runner goroutine loop |
| `internal/orchestrator` | Task, Dispatcher, ReAct-style Planner |
| `internal/discord` | Discord bot + channel router |
| `internal/llm` | LLM interface + Anthropic/OpenAI/Ollama clients |
| `internal/memory` | SQLite key-value store per agent |
| `internal/tools` | Tool interface (stub, not yet wired) |

---

## Configuration Path Priority

1. **Explicit flag**: `--config /path/to/config.yaml`
2. **Environment variable**: `MANGO_CONFIG=/path/to/config.yaml`
3. **System-wide**: `/etc/mango/config.yaml`
4. **Project config dir**: `./config/config.yaml`
5. **Current directory**: `./config.yaml`

New configuration created via the CLI (e.g., `mango config set`) defaults to `MANGO_CONFIG` if set, otherwise `/etc/mango/config.yaml` (if no configuration is found).

---

## Socket Path Configuration

The socket path (Unix domain socket for IPC) can be configured in three ways, in order of priority:

1. **Config file** (`config.yaml`): Set `socket_path` explicitly
2. **Environment variable**: `MANGO_SOCKET_PATH=/path/to/socket`
3. **Default**: 
   - macOS: `~/.mango/mango.sock`
   - Linux: `/var/run/mango/mango.sock`

Example with env var:
```bash
export MANGO_SOCKET_PATH=/tmp/mango.sock
mango serve
```

---

## Logical Order to Run It

### 1. Configuration
Edit `/etc/mango/config.yaml` (or use the `mango config` CLI) to define your agents, LLM providers, and optionally Discord and bindings.

```yaml
# socket_path is optional (uses default if omitted):
# socket_path: /custom/path/mango.sock

agents:
  - name: manager
    role: orchestrator              # optional: acts as task planner
    llm:
      provider: anthropic
      model: claude-3-5-haiku-20241022
      api_key: ${ANTHROPIC_API_KEY}

  - name: researcher
    capabilities: [research, summarize]
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

### 3. Check Health
```bash
mango status
# → gateway: ok (socket=/Users/.../.mango/mango.sock)
```

### 4. List Agents
```bash
mango agent list
```

### 5. Submit a Task (CLI)
```bash
# Route to a specific agent:
mango task submit "Summarize Go 1.24 release notes" --agent researcher

# Route through orchestrator (planner decomposes + fans out):
mango task submit "Research and summarize Go 1.24 features"

# Submit and wait for result:
mango task submit "..." --wait
```

### 6. Check Task Status
```bash
mango task status <task-id>
```

### 7. Discord (optional)
With `discord.token` set and channel bindings configured, users can message the bot in Discord. Messages in bound channels go directly to the named agent; unbound channels route through the orchestrator planner.

---

## Request Flow (CLI Path)

```
mango task submit "goal"
  └─► POST /tasks  (HTTP over Unix socket)
        └─► dispatcher.Submit(goal, agentName)
              ├─ agentName set   → runner.executeTask → LLM.Complete
              └─ agentName ""   → planner.Run (ReAct loop, max 5 steps)
                    └─► dispatcher.FanOut (parallel agent goroutines)
                          └─► each runner → LLM → result
                    └─► orchestrator LLM synthesizes final answer
```

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

## Notable Gaps (Current State)
- `tools.Tool` interface exists but no implementations are wired to agents yet
- `ChannelHistory` per Discord channel is stored but not injected into LLM calls
- Token cap is hardcoded at 1024 per LLM call (`runner.go`)
- Planner fails hard if goal takes more than 5 orchestration steps
