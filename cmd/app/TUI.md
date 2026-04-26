# How the TUI Works

Running `mango` (or `go run .`) with no subcommand launches the interactive terminal UI.
This document walks through every step from process start to pixels on screen.

---

## 1. Entry point — `main.go`

Cobra parses the command line. When no subcommand is given the root command's `RunE` fires:

```
main()
  └─ loadConfig(configPath)   // reads config.yaml, resolves socket path
  └─ runTUI(cfg)              // hands off to the TUI
```

`loadConfig` determines the Unix socket path (default `/var/run/mango/mango.sock` on Linux,
`~/.mango/mango.sock` on macOS). The socket path is the only thing the TUI needs from config —
everything else is fetched live from the gateway.

---

## 2. Starting the program — `runTUI` / `newTUIModel`

```go
func runTUI(cfg *Config) error {
    m := newTUIModel(cfg)
    p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
    _, err := p.Run()
    return err
}
```

`tea.NewProgram` takes over the terminal (alt screen = no scroll-back pollution).
`newTUIModel` builds the initial state:

| Field        | What it is                                                                        |
|--------------|-----------------------------------------------------------------------------------|
| `client`     | HTTP-over-Unix-socket client pointed at the gateway                               |
| `spinner`    | Charmbracelet spinner, styled orange, used while tasks run                        |
| `input`      | Main text input — goal entry                                                      |
| `agentInput` | Secondary input — optional `--agent` override                                     |
| `resultVP`   | Scrollable viewport for task results (not yet wired to a trigger in current code) |
| `section`    | Which panel is active: Tasks / Agents / Config                                    |
| `gatewayMsg` | Socket path string shown in the status bar                                        |

The model is a plain Go struct passed **by value**. Every state change in `Update` returns a new
copy — this is the core BubbleTea contract.

---

## 3. The event loop

BubbleTea runs three functions in a loop:

```
Init()   → returns initial tea.Cmd slice (runs once)
Update() → receives a tea.Msg, returns (new model, new tea.Cmd)
View()   → receives the current model, returns a string to render
```

The program never mutates state directly. Instead, `Update` returns a modified copy of the
model, and any async work is kicked off by returning a `tea.Cmd` (a `func() tea.Msg`).
BubbleTea runs those functions in goroutines and feeds their return values back into `Update`.

---

## 4. Init — what fires on startup

```go
func (m tuiModel) Init() tea.Cmd {
    return tea.Batch(
        m.spinner.Tick,          // starts the spinner animation
        checkHealth(m.client, m.ctx), // first gateway ping
        textinput.Blink,         // cursor blink
        tick(),                  // starts the 5-second refresh timer
    )
}
```

`checkHealth` fires a `GET /health` immediately. If the gateway responds, a `healthMsg(true)`
arrives in `Update`, which triggers `loadAgents` to populate the Agents panel.

---

## 5. Update — message dispatch

`Update` is a single switch on the message type. The full flow:

### WindowSizeMsg
First message the program always receives. Sets `m.width` / `m.height` and sizes the result
viewport. Until this arrives, `View()` returns `"  loading mango..."`.

### KeyMsg
Keys are routed in three layers:

1. **Overlay intercept** — if `showHelp` or `showResult` is active, the key is consumed there
   (any key closes help; esc/q/enter closes result; ↑↓ scrolls the result viewport).
2. **Global keys** — `q`/`ctrl+c`, `?`, `tab`/`shift+tab`.
3. **Section-specific keys** — `ctrl+a`, `esc`, `enter`, `r` only act in certain sections.

After key handling, `routeInputUpdate` forwards the message to whichever text input is active
so the input component can handle cursor movement, character insertion, etc.

### healthMsg
Updates `m.gatewayOK`. On the first successful ping (`wasOK == false → true`) it fires
`loadAgents` to populate the Agents panel.

### agentsLoadedMsg
Replaces `m.agents` in place. The Agents panel re-renders on the next `View()` call.

### taskSubmittedMsg
The gateway assigned an ID to a newly submitted task. The handler finds the first `pending`
task with no ID and upgrades it, then fires `pollTask2` to start polling.

### taskUpdatedMsg
A poll result arrived. The handler updates the matching task's `status`, `result`, and `errStr`.
If the status is `done` or `failed` it sets `pollingDone = true` and stops polling; otherwise it
fires another `pollTask2`. After updating, it recalculates `m.loading` by scanning all tasks.

### spinner.TickMsg / tickMsg
`spinner.TickMsg` advances the spinner animation frame.  
`tickMsg` fires every 5 seconds: re-checks health and, if the gateway is up, reloads agents.

---

## 6. View — rendering

```
View()
  ├─ width == 0 → "  loading mango..."
  ├─ showHelp   → viewHelp()    (full-screen overlay)
  ├─ showResult → viewResult()  (full-screen overlay)
  └─ normal layout:
       viewHeader()                 ← title bar (logo + tagline + spinner)
       ┌─ viewNav() │ viewContent() ┐  ← side-by-side via JoinHorizontal
       └─────────────────────────────┘
       viewStatusBar()              ← gateway status + socket path + key hints
```

`viewContent()` routes to one of three panels based on `m.section`:

| Section | Panel               | Contents                                                                                 |
|---------|---------------------|------------------------------------------------------------------------------------------|
| Tasks   | `viewTasks`         | ASCII logo → goal input → optional agent input → recent task list (last 8, newest first) |
| Agents  | `viewAgents`        | Live table of agent name / status / skills                                               |
| Config  | `viewConfigSection` | Reference list of `mango config` CLI commands                                            |

---

## 7. Task lifecycle end-to-end

```
User types goal, presses Enter
  └─ handleTasksEnter()
       ├─ appends trackedTask{status:"pending"} to m.tasks
       └─ fires submitTask → POST /tasks

taskSubmittedMsg arrives
  └─ assigns server ID to the pending task
  └─ fires pollTask2 → GET /tasks/:id (on 1.5s timer)

taskUpdatedMsg arrives (repeats every 1.5s)
  └─ updates status / result / errStr
  └─ if done or failed → stops polling, sets pollingDone = true
  └─ otherwise → fires another pollTask2

View renders the task row:
  ✓ green check   → done
  ✗ red cross     → failed
  ⟳ amber spinner → running / pending
  (result preview shown inline when pollingDone)
```

---

## 8. Gateway communication

All API calls go through `gatewayClient`, which wraps `net/http` with a Unix-socket dialer.
There is no TCP involved — every request dials the socket path directly.

| Command       | Method + Path    | Purpose                             |
|---------------|------------------|-------------------------------------|
| `checkHealth` | `GET /health`    | Liveness check                      |
| `loadAgents`  | `GET /agents`    | Fetch agent list                    |
| `submitTask`  | `POST /tasks`    | Submit goal (+ optional agent name) |
| `pollTask2`   | `GET /tasks/:id` | Fetch task status / result          |

If the gateway is not running, `request()` wraps the dial error into a human-readable message
(`"gateway not running at … — start with mango serve"`).

---

## 9. Key bindings summary

| Key                 | Action                                         |
|---------------------|------------------------------------------------|
| `tab` / `shift+tab` | Cycle sections (Tasks → Agents → Config → …)   |
| `enter`             | Submit task (Tasks) or reload agents (Agents)  |
| `ctrl+a`            | Toggle the agent-name input below the goal box |
| `esc`               | Dismiss agent input / close overlays           |
| `r`                 | Refresh agents                                 |
| `?`                 | Show keyboard shortcut help                    |
| `q` / `ctrl+c`      | Clear input if non-empty, otherwise quit       |
