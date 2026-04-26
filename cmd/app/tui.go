package main

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── sections ──────────────────────────────────────────────────────────────────

type section int

const (
	sectionChat section = iota
	sectionTasks
	sectionAgents
	sectionConfig
)

var sectionNames = []string{"Chat", "Tasks", "Agents", "Config"}

type NavSection struct {
	Index section
	Name  string
	Icon  string
	View  string
}

var navSections = []NavSection{
	{Name: "Chat", Icon: "❯_", Index: sectionChat},
	{Name: "Tasks", Icon: "⫘", Index: sectionTasks},
	{Name: "Agents", Icon: "⎔", Index: sectionAgents},
	{Name: "Config", Icon: "⌥", Index: sectionConfig},
}

// ── messages ──────────────────────────────────────────────────────────────────

type agentsLoadedMsg []agentStatusDTO
type taskSubmittedMsg taskDTO
type taskUpdatedMsg taskDTO
type healthMsg bool
type errMsg struct{ err error }
type tickMsg time.Time

// ── tracked task ──────────────────────────────────────────────────────────────

type trackedTask struct {
	id          string
	goal        string
	status      string
	result      string
	errStr      string
	agent       string
	pollingDone bool
}

// ── model ─────────────────────────────────────────────────────────────────────

type tuiModel struct {
	client      *gatewayClient
	ctx         context.Context
	width       int
	height      int
	section     NavSection
	agents      []agentStatusDTO
	tasks       []trackedTask
	gatewayOK   bool
	gatewayMsg  string
	loading     bool
	spinner     spinner.Model
	input       textinput.Model
	agentInput  textinput.Model // "--agent" override
	resultVP    viewport.Model
	showResult  bool
	resultText  string
	showHelp    bool
	showAgentIn bool
	err         error
}

func newTUIModel(cfg *Config) tuiModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleStatusRunning

	ti := textinput.New()
	ti.Placeholder = "What should mango do? (press Enter)"
	ti.PlaceholderStyle = styleFaint
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorCream)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colorOrange)
	ti.Focus()
	ti.CharLimit = 512

	ai := textinput.New()
	ai.Placeholder = "agent name (optional)"
	ai.PlaceholderStyle = styleFaint
	ai.TextStyle = lipgloss.NewStyle().Foreground(colorAmber)
	ai.CharLimit = 64

	vp := viewport.New(80, 20)

	return tuiModel{
		client:     newGatewayClient(cfg.SocketPath),
		ctx:        context.Background(),
		spinner:    sp,
		input:      ti,
		agentInput: ai,
		resultVP:   vp,
		gatewayMsg: cfg.SocketPath,
	}
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		checkHealth(m.client, m.ctx),
		textinput.Blink,
		tick(),
	)
}

// ── entry point ───────────────────────────────────────────────────────────────

func runTUI(cfg *Config) error {
	m := newTUIModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
