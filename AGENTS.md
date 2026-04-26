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
  Dispatcher ──► Orchestrator (orchestrator agent)    Discord Bot
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
| `internal/orchestrator` | Task, Dispatcher, ReAct-style Orchestrator |
| `internal/discord` | Discord bot + channel router |
| `internal/llm` | LLM interface + Anthropic/OpenAI/Ollama clients |
| `internal/memory` | SQLite key-value store per agent |
| `internal/tools` | Tool interface, tool registry, built-in tools (GoSolarTool) |

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

## Installation (Linux)

`install.sh` builds the binary, installs the systemd unit, creates the `mango` system user, and writes a starter config to `/etc/mango/config.yaml` from `config/config.default.yaml` (orchestrator + worker scaffolding with empty LLM fields). It also seeds the agent definition files by copying `config/agents/<name>.md` → `/etc/mango/agents/<name>.md` for any agent whose definition file doesn't yet exist. It then runs two optional interactive prompts:

- **Discord setup**: asks for a bot token, then whether to bind the bot globally (all channels → orchestrator) or to a comma-separated list of channel IDs (each bound to a chosen agent, default `worker`). A `discord:` block (and `bindings:` if channels were provided) is prepended to the installed config.
- **LLM setup**: for each of `orchestrator` and `worker`, prompts for provider / model / api_key / base_url and applies them via `mango config agent edit`. Leaving provider blank skips that agent.

Skipping either step prints an `ACTION REQUIRED` block with the file path to edit and the `systemctl daemon-reload && systemctl restart mango` commands to apply changes.

---

## Agent Personalities & Skills — Definition Files

Agent system prompts are assembled at startup by combining agent definition files with skill definitions.

### Agent Definition Files

Each agent has a corresponding `.md` file (e.g., `ORCHESTRATOR.md`, `WORKER.md`, `researcher.md`) in the agents directory, configurable via `MANGO_AGENTS_DIR` (default: `/etc/mango/agents/`). For the default install:
- Orchestrator: `/etc/mango/agents/ORCHESTRATOR.md`
- Worker: `/etc/mango/agents/WORKER.md`

- **No hardcoded prompts.** At startup, `serve.go` reads each agent's definition file, trims it, appends any skills' definitions (in order), and sets `Agent.SystemPrompt`. Startup fails hard if the file is missing or empty — this is intentional: an agent with no persona should not silently run with stub behavior.
- **Orchestrator definition** must encode the JSON schema contract (`action`, `tasks`, `final`) that `parseOrchestratorResponse` expects. The orchestrator explicitly requests JSON mode from the LLM provider when possible. The dynamic agent catalog (names + skills pulled from the live registry) is still appended by `orchestrator.agentCatalog()`; don't duplicate it in the .md file.
- **Worker / custom agents** can contain any persona, tone, tool-use guidelines, etc. Edit the agent definition file, then `sudo systemctl restart mango` to reload.

### Skills

Skills are reusable system prompt snippets stored as `.md` files in the skills directory, configurable via `MANGO_SKILLS_DIR` (default: `/etc/mango/skills/`). Skills are declared in the agent config and their definitions are automatically appended to the agent's system prompt at startup, in the order listed.

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

# Route through orchestrator (orchestrator decomposes + fans out):
mango task submit "Research and summarize Go 1.24 features"

# Submit and wait for result:
mango task submit "..." --wait
```

### 6. Check Task Status
```bash
mango task status <task-id>
```

### 7. Discord (optional)
With `discord.token` set and channel bindings configured, users can message the bot in Discord. Messages in bound channels go directly to the named agent; unbound channels route through the orchestrator.

While the model is thinking the bot keeps a typing indicator active (refreshed every 8s). Per-channel conversation history (last 100 messages, `internal/discord/context.go`) is injected into the LLM call for both the direct-agent and orchestrator paths, so follow-up messages preserve context.

---

## Request Flow (CLI Path)

```
mango task submit "goal"
  └─► POST /tasks  (HTTP over Unix socket)
        └─► dispatcher.Submit(goal, agentName)          # or SubmitWithHistory from Discord
              ├─ agentName set   → RunOnAgentWithHistory → runner.executeTask → LLM.Complete
              └─ agentName ""   → orchestrator.Run(goal, history, d) (ReAct loop, max 5 steps)
                    └─► dispatcher.FanOut (parallel agent goroutines)
                          └─► each runner → LLM → result
                    └─► orchestrator LLM synthesizes final answer
```

---

## External Dependencies

| Service        | Config key                                | Required?           |
|----------------|-------------------------------------------|---------------------|
| Anthropic API  | `api_key: ${ANTHROPIC_API_KEY}` per agent | If using Anthropic  |
| OpenAI API     | `api_key` + `base_url` per agent          | If using OpenAI     |
| Ollama (local) | `base_url: http://localhost:11434/v1`     | If using local LLMs |
| Discord        | `discord.token: ${DISCORD_TOKEN}`         | Optional            |
| SQLite         | auto-created at `work_dir/memory.db`      | Auto                |

---

## Built-in Tools

### GoSolarTool
Calculates solar position and timing data (sunrise, sunset, solar noon) for any location. Useful for time-based scheduling, solar event alerts, or environmental monitoring.

**Input**: Latitude, longitude, date, timezone
**Output**: Sunrise time, sunset time, solar noon, solar position (elevation, azimuth)

Usage: Agents with access to this tool can calculate solar data directly without external APIs.

## Notable Gaps (Current State)
- Token cap is hardcoded at 1024 per LLM call (`runner.go`)
- Orchestrator fails hard if goal takes more than 5 orchestration steps
- Anthropic prompt caching is not enabled — each Discord turn re-sends the full history uncached
- Additional tools beyond GoSolarTool are not yet implemented
