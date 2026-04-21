
  
                                ███╗   ███╗ █████╗ ███╗   ██╗ ██████╗  ██████╗
                                ████╗ ████║██╔══██╗████╗  ██║██╔════╝ ██╔═══██╗
                                ██╔████╔██║███████║██╔██╗ ██║██║  ███╗██║   ██║
                                ██║╚██╔╝██║██╔══██║██║╚██╗██║██║   ██║██║   ██║
                                ██║ ╚═╝ ██║██║  ██║██║ ╚████║╚██████╔╝╚██████╔╝
                                ╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝ ╚═════╝  ╚═════╝
                                ( ...napping in progress )


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

### Configuration

By default, Mango looks for configuration in `/etc/mango/config.yaml`, then `./config/config.yaml`, and finally `./config.yaml`.
You can override the default path by setting the `MANGO_CONFIG` environment variable.

You can use the CLI to initialize and manage your configuration:

```bash
# Set your Discord token
./mango config set discord.token "YOUR_DISCORD_TOKEN"

# Add an agent
./mango config agent add researcher --provider ollama --model llama3.2
```

### Agent Definitions & Skills

Each agent's system prompt is defined in a `.md` file located in the agents directory (default: `/etc/mango/agents/`). For example:

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
