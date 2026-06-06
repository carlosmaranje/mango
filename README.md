
  
                                ███╗   ███╗ █████╗ ███╗   ██╗ ██████╗  ██████╗
                                ████╗ ████║██╔══██╗████╗  ██║██╔════╝ ██╔═══██╗
                                ██╔████╔██║███████║██╔██╗ ██║██║  ███╗██║   ██║
                                ██║╚██╔╝██║██╔══██║██║╚██╗██║██║   ██║██║   ██║
                                ██║ ╚═╝ ██║██║  ██║██║ ╚████║╚██████╔╝╚██████╔╝
                                ╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝ ╚═════╝  ╚═════╝
                                          ...napping in progress


> Mango is a lazy orange cat who loves to eat, sleep, and — on good days — orchestrate AI agents.
> It's also a tropical fruit.
> Mostly, though, it naps.

**Mango** is a multi-agent orchestration gateway that brings the power of agentic AI to Discord and your terminal. Define specialized agents with different capabilities and LLM backends; a central orchestrator decomposes goals into parallel sub-tasks and fans them out while the cat sleeps.

## ✨ Features

- **Multi-Agent Orchestration**: Automatically decompose high-level goals into sub-tasks for specialized agents.
- **Provider Agnostic**: Built-in support for **Anthropic**, **OpenAI**, and local models via **Ollama**.
- **Flexible Agent Personalities**: Define agent behaviors via markdown files (agent definitions) and reusable skills.
- **Discord Integration**: Interact with specific agents or the whole system through Discord channels. See [DISCORD_SETUP.md](DISCORD_SETUP.md) for a detailed guide.
- **CLI Control Plane**: A powerful command-line interface to manage the gateway, check status, and dispatch tasks.
- **Built-in Tools**: Agents can access tools like GoSolarTool for calculations without external APIs.
- **Persistent Memory**: SQLite-backed key-value store for agents to maintain state across sessions.
- **Unix Socket Gateway**: Efficient local communication between the CLI and the background server.
- **REST / Browser API**: Optional TCP listener with CORS and Server-Sent Events for real-time task updates.

## Getting Started

### Prerequisites

- [Go](https://golang.org/doc/install) 1.24+
- [Ollama](https://ollama.com/) (optional, for local models)
- A Discord Bot Token (for Discord integration, see [DISCORD_SETUP.md](DISCORD_SETUP.md))

### Installation

**1. Clone the repository:**
```bash
git clone https://github.com/carlosmaranje/mango.git
cd mango
```

**2. Run the installer for your platform:**

#### Linux (requires Go, `git`, `systemd`)
```bash
./install.sh
```
The script builds the binary, creates a `mango` system user, installs the systemd unit, and walks you through configuring your LLM providers and optional Discord bot interactively.

```bash
sudo systemctl enable mango
sudo systemctl start mango
```

**To uninstall:** `./install.sh uninstall`

#### macOS (requires Go, `git`)
```bash
./install-mac.sh
```
The script builds the binary, sets up config at `~/.mango/config.yaml`, installs a launchd agent (auto-starts on login), and walks you through the same interactive setup.

The agent starts automatically after install. To manage it manually:
```bash
launchctl unload  ~/Library/LaunchAgents/com.mango.gateway.plist
launchctl load    ~/Library/LaunchAgents/com.mango.gateway.plist
```

**To uninstall:** `./install-mac.sh uninstall`

### Directory Layout

All runtime files live under `MANGO_DIR` (default: `~/.mango`). Set the `MANGO_DIR` environment variable to move everything at once — it is the only path-related env var.

| Path | Purpose |
|---|---|
| `$MANGO_DIR/config.yaml` | Main configuration file |
| `$MANGO_DIR/agents/<NAME>.md` | Agent persona / system prompt (uppercase filename) |
| `$MANGO_DIR/agents/<name>/` | Agent work directory (SQLite memory lives here) |
| `$MANGO_DIR/skills/<name>.md` | Reusable skill snippets |
| `$MANGO_DIR/mango.sock` *(macOS)* | Unix socket for CLI ↔ server IPC |
| `/var/run/mango/mango.sock` *(Linux)* | Unix socket (system install default) |

Override the socket path by setting `socket_path` in config. Override the config file path with the `--config` flag.

### Configuration

Mango searches for config in this order:

1. `--config /path/to/config.yaml` (CLI flag)
2. `$MANGO_DIR/config.yaml`
3. `./config/config.yaml`
4. `./config.yaml`

Use the CLI to manage your configuration:

```bash
# Set your Discord token
./mango config set discord.token "YOUR_DISCORD_TOKEN"

# Add an agent
./mango config agent add researcher --provider ollama --model llama3.2
```

### Agent Definitions & Skills

Each agent's system prompt is defined in a `.md` file located in the agents directory (`$MANGO_DIR/agents/`). For example:

**Orchestrator Agent** (`/etc/mango/agents/ORCHESTRATOR.md`):
```markdown
# Orchestrator Agent

You are a task orchestrator. Your role is to decompose user goals into parallel sub-tasks and delegate them to specialized agents.

## Core Responsibility

When given a goal, analyze it to determine:
1. Whether it can be solved in one step or requires multiple sub-tasks
2. Which agents are best suited for each sub-task
3. How to combine their results into a final answer

## Response Format

You MUST respond ONLY with a valid JSON object:
...
```

**Skills** are reusable system prompt snippets stored as `.md` files in the skills directory (default: `/etc/mango/skills/`). List skills in your agent config and they are automatically appended to the agent's system prompt at startup:

```yaml
agents:
  - name: researcher
    skills: [web_search, code_analysis]
    llm:
      provider: ollama
      model: llama3.2
```

At startup, the researcher agent's system prompt is assembled as:
```
[researcher.md content]

---

[web_search.md content]

---

[code_analysis.md content]
```

Example configuration structure:

```yaml
agents:
  - name: orchestrator
    role: orchestrator
    llm:
      provider: anthropic
      model: claude-sonnet-4-20250514
      api_key: "${ANTHROPIC_API_KEY}"

  - name: researcher
    skills: [web_search]
    llm:
      provider: ollama
      model: llama3.2

discord:
  token: "${DISCORD_TOKEN}"

bindings:
  - channel_id: "123456789"
    agent: researcher
```

## Usage

### Starting the Gateway

To start the Discord bot and the orchestration server:

```bash
./mango serve
```

### Using the CLI

You can interact with the running gateway from another terminal:

- **Check Status**:
  ```bash
  ./mango status
  ```

- **Submit a Task**:
  ```bash
  ./mango task "Research the latest trends in Go 1.24 and summarize them."
  ```

- **Manage Agents**:
  ```bash
  ./mango agent list
  ```

## REST API & Browser Access

Add `http_addr` to your config to expose the gateway over TCP:

```yaml
http_addr: ":8080"
```

The gateway binds both the Unix socket (for the CLI) and the TCP port simultaneously. CORS headers are set automatically so browser clients can connect directly.

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |
| `GET` | `/agents` | List agents |
| `POST` | `/agents/start` | Start an agent |
| `POST` | `/agents/stop` | Stop an agent |
| `GET` | `/tasks` | List all tasks |
| `POST` | `/tasks` | Submit a task (async, 202) |
| `GET` | `/tasks/{id}` | Poll task status |
| `DELETE` | `/tasks/{id}` | Cancel a task |
| `POST` | `/chat` | Submit and wait for result (sync) |
| `GET` | `/events` | SSE stream of real-time events |

The `/events` endpoint streams `task.created`, `task.updated`, `agent.started`, `agent.stopped`, and `ping` (keepalive every 15s) events as Server-Sent Events. Connect once with the browser's `EventSource` API — no polling required.

## 🛠️ Built-in Tools

Mango includes built-in tools that agents can access:

### GoSolarTool
Calculates solar position and timing data (sunrise, sunset, solar noon) for any location.

**Inputs:**
- Latitude: Decimal degrees (-90 to 90)
- Longitude: Decimal degrees (-180 to 180)
- Date: YYYY-MM-DD format
- Timezone: IANA timezone string (e.g., "America/New_York")

**Outputs:**
- Sunrise time
- Sunset time
- Solar noon
- Solar position (elevation and azimuth angles)

Example use cases: Solar event alerts, time-based scheduling, environmental monitoring.

## 📦 Deployment

Production-ready service files are provided in the `deploy/` directory:

-   **Linux (systemd)**: `deploy/mango.service`
-   **macOS (launchd)**: `deploy/mango.plist`

## 📄 License

This project is licensed under the terms of the LICENSE file included in the repository.
