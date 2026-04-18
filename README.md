# 🥭 Mango

**Mango** is a multi-agent orchestration gateway that brings the power of agentic AI to Discord and your terminal. It allows you to define specialized agents with different capabilities and LLM backends, orchestrated by a central planner to solve complex tasks.

## ✨ Features

- **Multi-Agent Orchestration**: Automatically decompose high-level goals into sub-tasks for specialized agents.
- **Provider Agnostic**: Built-in support for **Anthropic**, **OpenAI**, and local models via **Ollama**.
- **Discord Integration**: Interact with specific agents or the whole system through Discord channels. See [DISCORD_SETUP.md](DISCORD_SETUP.md) for a detailed guide.
- **CLI Control Plane**: A powerful command-line interface to manage the gateway, check status, and dispatch tasks.
- **Persistent Memory**: SQLite-backed key-value store for agents to maintain state across sessions.
- **Unix Socket Gateway**: Efficient local communication between the CLI and the background server.

## Getting Started

### Prerequisites

- [Go](https://golang.org/doc/install) 1.24+
- [Ollama](https://ollama.com/) (optional, for local models)
- A Discord Bot Token (for Discord integration, see [DISCORD_SETUP.md](DISCORD_SETUP.md))

### Installation

1.  **Clone the repository**:
    ```bash
    git clone https://github.com/carlosmaranje/goclaw.git
    cd goclaw
    ```

2.  **Build the binary**:
    ```bash
    go build -o mango ./cmd/app
    ```

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

Example configuration structure:

```yaml
discord:
  token: "YOUR_DISCORD_TOKEN"

agents:
  - name: orchestrator
    role: orchestrator
    llm:
      provider: anthropic
      model: claude-3-5-sonnet-latest
      api_key: "${ANTHROPIC_API_KEY}"

  - name: researcher
    capabilities: [web_search]
    llm:
      provider: ollama
      model: llama3.2
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

## 📦 Deployment

Production-ready service files are provided in the `deploy/` directory:

-   **Linux (systemd)**: `deploy/mango.service`
-   **macOS (launchd)**: `deploy/mango.plist`

## 📄 License

This project is licensed under the terms of the LICENSE file included in the repository.
