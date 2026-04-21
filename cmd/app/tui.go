package main

import (
	"context"
	"fmt"
	"strings"
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
	sectionTasks section = iota
	sectionAgents
	sectionConfig
)

var sectionNames = []string{"Tasks", "Agents", "Config"}

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
	section     section
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
	showAgentIn bool // toggle agent name input
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

// ── Update ────────────────────────────────────────────────────────────────────

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resultVP.Width = m.contentWidth() - 4
		m.resultVP.Height = m.contentHeight() - 10

	case tea.KeyMsg:
		// help overlay
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		// result overlay
		if m.showResult {
			switch msg.String() {
			case "esc", "q", "enter":
				m.showResult = false
			default:
				var cmd tea.Cmd
				m.resultVP, cmd = m.resultVP.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.input.Focused() && m.input.Value() != "" {
				m.input.SetValue("")
				return m, nil
			}
			return m, tea.Quit

		case "?":
			m.showHelp = true

		case "tab", "right":
			m.section = (m.section + 1) % section(len(sectionNames))
			if m.section == sectionTasks {
				m.input.Focus()
			} else {
				m.input.Blur()
			}

		case "shift+tab", "left":
			m.section = (m.section + section(len(sectionNames)) - 1) % section(len(sectionNames))
			if m.section == sectionTasks {
				m.input.Focus()
			} else {
				m.input.Blur()
			}

		case "ctrl+a":
			if m.section == sectionTasks {
				m.showAgentIn = !m.showAgentIn
				if m.showAgentIn {
					m.input.Blur()
					m.agentInput.Focus()
				} else {
					m.agentInput.Blur()
					m.agentInput.SetValue("")
					m.input.Focus()
				}
			}

		case "esc":
			if m.showAgentIn {
				m.showAgentIn = false
				m.agentInput.Blur()
				m.agentInput.SetValue("")
				m.input.Focus()
			}

		case "enter":
			if m.section == sectionTasks {
				if m.showAgentIn && m.agentInput.Focused() {
					m.agentInput.Blur()
					m.input.Focus()
					m.showAgentIn = false
					return m, nil
				}
				goal := strings.TrimSpace(m.input.Value())
				if goal == "" {
					return m, nil
				}
				agentName := strings.TrimSpace(m.agentInput.Value())
				m.input.SetValue("")
				m.agentInput.SetValue("")
				m.showAgentIn = false
				t := trackedTask{goal: goal, status: "pending", agent: agentName}
				m.tasks = append(m.tasks, t)
				cmds = append(cmds, submitTask(m.client, m.ctx, goal, agentName, len(m.tasks)-1))
				m.loading = true
			}
			if m.section == sectionAgents {
				cmds = append(cmds, loadAgents(m.client, m.ctx))
			}

		case "r":
			if m.section == sectionAgents {
				cmds = append(cmds, loadAgents(m.client, m.ctx))
			}
		}

	case healthMsg:
		m.gatewayOK = bool(msg)
		if m.gatewayOK {
			cmds = append(cmds, loadAgents(m.client, m.ctx))
		}

	case agentsLoadedMsg:
		m.agents = []agentStatusDTO(msg)

	case taskSubmittedMsg:
		dto := taskDTO(msg)
		for i := range m.tasks {
			if m.tasks[i].status == "pending" && m.tasks[i].id == "" {
				m.tasks[i].id = dto.ID
				m.tasks[i].status = dto.Status
				cmds = append(cmds, pollTask2(m.client, m.ctx, dto.ID, i))
				break
			}
		}

	case taskUpdatedMsg:
		dto := taskDTO(msg)
		for i := range m.tasks {
			if m.tasks[i].id == dto.ID {
				m.tasks[i].status = dto.Status
				m.tasks[i].result = dto.Result
				m.tasks[i].errStr = dto.Error
				if dto.Status == "done" || dto.Status == "failed" {
					m.tasks[i].pollingDone = true
				} else {
					cmds = append(cmds, pollTask2(m.client, m.ctx, dto.ID, i))
				}
				break
			}
		}
		// check if any task still running
		m.loading = false
		for _, t := range m.tasks {
			if t.status != "done" && t.status != "failed" && t.id != "" {
				m.loading = true
				break
			}
		}

	case errMsg:
		m.err = msg.err

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tickMsg:
		cmds = append(cmds, tick())
		if m.gatewayOK {
			cmds = append(cmds, loadAgents(m.client, m.ctx))
		} else {
			cmds = append(cmds, checkHealth(m.client, m.ctx))
		}
	}

	if m.section == sectionTasks {
		if m.showAgentIn && m.agentInput.Focused() {
			var cmd tea.Cmd
			m.agentInput, cmd = m.agentInput.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m tuiModel) View() string {
	if m.width == 0 {
		return "  loading mango..."
	}

	if m.showHelp {
		return m.viewHelp()
	}

	if m.showResult {
		return m.viewResult()
	}

	header := m.viewHeader()
	nav := m.viewNav()
	content := m.viewContent()
	status := m.viewStatusBar()

	navW := navWidth
	contentW := m.width - navW - 2

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(navW).Height(m.contentHeight()).Render(nav),
		styleDivider.Render(strings.Repeat("│\n", m.contentHeight())),
		lipgloss.NewStyle().Width(contentW).Height(m.contentHeight()).Render(content),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		body,
		status,
	)
}

const navWidth = 18

func (m tuiModel) contentWidth() int  { return m.width - navWidth - 2 }
func (m tuiModel) contentHeight() int { return m.height - 4 } // header + status

func (m tuiModel) viewHeader() string {
	logo := styleTitle.Render("🥭 mango")
	tagline := styleSubtitle.Render("the sleepy agent orchestrator")
	right := ""
	if m.loading {
		right = m.spinner.View() + " " + styleFaint.Render("working...")
	}
	gap := m.width - lipgloss.Width(logo) - lipgloss.Width(tagline) - lipgloss.Width(right) - 6
	if gap < 1 {
		gap = 1
	}
	line := styleTitleBar.Width(m.width).Render(
		logo + "  " + tagline + strings.Repeat(" ", gap) + right,
	)
	return line
}

func (m tuiModel) viewNav() string {
	var b strings.Builder
	b.WriteString(styleSectionHeader.PaddingLeft(2).Render("NAVIGATE") + "\n\n")
	for i, name := range sectionNames {
		icon := navIcons[i]
		label := icon + " " + name
		if section(i) == m.section {
			b.WriteString(styleNavItemActive.Width(navWidth-2).Render(label) + "\n")
		} else {
			b.WriteString(styleNavItem.Width(navWidth-2).Render(label) + "\n")
		}
	}
	return b.String()
}

var navIcons = []string{"✦", "⬡", "⚙"}

func (m tuiModel) viewContent() string {
	switch m.section {
	case sectionTasks:
		return m.viewTasks()
	case sectionAgents:
		return m.viewAgents()
	case sectionConfig:
		return m.viewConfigSection()
	}
	return ""
}

func (m tuiModel) viewTasks() string {
	w := m.contentWidth() - 2
	var b strings.Builder

	b.WriteString(styleSectionHeader.Render("SUBMIT A TASK") + "\n\n")

	// input box
	inputBox := m.input.View()
	if m.input.Focused() {
		inputBox = styleInputFocused.Width(w - 4).Render(m.input.View())
	} else {
		inputBox = styleInput.Width(w - 4).Render(m.input.View())
	}
	b.WriteString(inputBox + "\n")

	// agent override toggle
	if m.showAgentIn {
		agentBox := styleInput.Width(w - 4).Render(m.agentInput.View())
		if m.agentInput.Focused() {
			agentBox = styleInputFocused.Width(w - 4).Render(m.agentInput.View())
		}
		b.WriteString(agentBox + "\n")
	}

	hint := styleFaint.Render("enter") + styleKeyDesc.Render(" submit  ") +
		styleFaint.Render("ctrl+a") + styleKeyDesc.Render(" set agent  ") +
		styleFaint.Render("?") + styleKeyDesc.Render(" help")
	b.WriteString(hint + "\n")

	if len(m.tasks) == 0 {
		b.WriteString("\n" + styleFaint.Render("  no tasks yet — mango is napping... 😴") + "\n")
		return b.String()
	}

	b.WriteString("\n" + styleSectionHeader.Render("RECENT TASKS") + "\n\n")

	// show last 8 tasks, newest first
	start := 0
	if len(m.tasks) > 8 {
		start = len(m.tasks) - 8
	}
	for i := len(m.tasks) - 1; i >= start; i-- {
		t := m.tasks[i]
		icon := taskStatusIcon(t.status)
		stStyle := taskStatusStyle(t.status)

		goal := t.goal
		maxG := w - 22
		if len(goal) > maxG {
			goal = goal[:maxG-1] + "…"
		}

		agentTag := ""
		if t.agent != "" {
			agentTag = stylePill.Render(t.agent)
		}

		idStr := ""
		if t.id != "" {
			idStr = styleFaint.Render("#" + t.id[:min8(t.id)])
		}

		row := fmt.Sprintf("  %s %s  %s %s",
			stStyle.Render(icon),
			lipgloss.NewStyle().Foreground(colorCream).Render(goal),
			agentTag,
			idStr,
		)
		b.WriteString(row + "\n")
		if t.result != "" && t.pollingDone {
			preview := t.result
			if len(preview) > w-6 {
				preview = preview[:w-7] + "…"
			}
			b.WriteString(styleFaint.PaddingLeft(4).Render(preview) + "\n")
		}
		if t.errStr != "" {
			b.WriteString(styleTaskFailed.PaddingLeft(4).Render("error: "+t.errStr) + "\n")
		}
	}

	return b.String()
}

func min8(s string) int {
	if len(s) < 8 {
		return len(s)
	}
	return 8
}

func (m tuiModel) viewAgents() string {
	var b strings.Builder
	b.WriteString(styleSectionHeader.Render("AGENT STATUS") + "\n\n")

	if !m.gatewayOK {
		b.WriteString(styleStatusErr.Render("  ✗ gateway offline") + "\n")
		b.WriteString(styleFaint.Render("  start with: mango serve") + "\n")
		return b.String()
	}

	if len(m.agents) == 0 {
		b.WriteString(styleFaint.Render("  no agents registered") + "\n")
		return b.String()
	}

	w := m.contentWidth() - 4
	for _, a := range m.agents {
		icon := agentStatusIcon(a.Status)
		name := lipgloss.NewStyle().Foreground(colorCream).Bold(true).Render(a.Name)
		status := lipgloss.NewStyle().Foreground(colorMuted).Render(a.Status)

		skills := ""
		if len(a.Skills) > 0 {
			var tags []string
			for _, sk := range a.Skills {
				tags = append(tags, stylePill.Render(sk))
			}
			skills = lipgloss.JoinHorizontal(lipgloss.Top, tags...)
		}

		row := fmt.Sprintf("  %s  %-18s  %-12s  %s", icon, name, status, skills)
		b.WriteString(lipgloss.NewStyle().Width(w).Render(row) + "\n")
	}

	b.WriteString("\n" + styleFaint.Render("  r to refresh  •  enter to reload") + "\n")
	return b.String()
}

func (m tuiModel) viewConfigSection() string {
	var b strings.Builder
	b.WriteString(styleSectionHeader.Render("CONFIGURATION") + "\n\n")
	b.WriteString(styleFaint.Render("  run mango config show to view the full config") + "\n\n")
	b.WriteString(styleFaint.Render("  common commands:") + "\n")

	cmds := [][]string{
		{"mango config show", "print current config"},
		{"mango config set <key> <val>", "set a config value"},
		{"mango config agent add <name>", "add a new agent"},
		{"mango config agent edit <name>", "edit agent settings"},
		{"mango add agent <name>", "interactive agent wizard"},
		{"mango add skill <name>", "create a new skill"},
	}
	for _, c := range cmds {
		b.WriteString(fmt.Sprintf("  %s  %s\n",
			styleKeyHint.Render(c[0]),
			styleKeyDesc.Render("— "+c[1]),
		))
	}
	return b.String()
}

func (m tuiModel) viewStatusBar() string {
	gwStatus := ""
	if m.gatewayOK {
		gwStatus = styleStatusOK.Render("● gateway ok")
	} else {
		gwStatus = styleStatusErr.Render("○ gateway offline")
	}

	socket := styleFaint.Render(" (" + m.gatewayMsg + ")")
	hint := styleFaint.Render("tab next  ? help  q quit")
	gap := m.width - lipgloss.Width(gwStatus) - lipgloss.Width(socket) - lipgloss.Width(hint) - 4
	if gap < 1 {
		gap = 1
	}
	return styleStatusBar.Width(m.width).Render(
		gwStatus + socket + strings.Repeat(" ", gap) + hint,
	)
}

func (m tuiModel) viewHelp() string {
	title := styleTitleBar.Width(m.width).Render("🥭 mango — keyboard shortcuts")

	keys := [][]string{
		{"tab / shift+tab", "switch sections"},
		{"enter", "submit task / confirm"},
		{"ctrl+a", "toggle agent name input"},
		{"esc", "cancel / close"},
		{"r", "refresh agents"},
		{"?", "toggle this help"},
		{"q / ctrl+c", "quit"},
	}

	var rows []string
	for _, k := range keys {
		rows = append(rows, fmt.Sprintf("  %s  %s",
			styleKeyHint.Width(24).Render(k[0]),
			styleKeyDesc.Render(k[1]),
		))
	}

	body := styleBase.Width(m.width).Padding(1, 2).Render(strings.Join(rows, "\n"))
	footer := styleStatusBar.Width(m.width).Render(styleFaint.Render("press any key to close"))
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

func (m tuiModel) viewResult() string {
	title := styleTitleBar.Width(m.width).Render("🥭 task result")
	body := styleResult.Width(m.width - 4).Height(m.height - 4).Render(m.resultVP.View())
	footer := styleStatusBar.Width(m.width).Render(styleFaint.Render("↑/↓ scroll  esc close"))
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

// ── commands ──────────────────────────────────────────────────────────────────

func tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func checkHealth(c *gatewayClient, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		var out map[string]any
		err := c.request(ctx, "GET", "/health", nil, &out)
		return healthMsg(err == nil)
	}
}

func loadAgents(c *gatewayClient, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		var out []agentStatusDTO
		if err := c.request(ctx, "GET", "/agents", nil, &out); err != nil {
			return errMsg{err}
		}
		return agentsLoadedMsg(out)
	}
}

func submitTask(c *gatewayClient, ctx context.Context, goal, agentName string, _ int) tea.Cmd {
	return func() tea.Msg {
		body := map[string]string{"goal": goal}
		if agentName != "" {
			body["agent"] = agentName
		}
		var out taskDTO
		if err := c.request(ctx, "POST", "/tasks", body, &out); err != nil {
			return errMsg{err}
		}
		return taskSubmittedMsg(out)
	}
}

func pollTask2(c *gatewayClient, ctx context.Context, id string, _ int) tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(_ time.Time) tea.Msg {
		var out taskDTO
		if err := c.request(ctx, "GET", "/tasks/"+id, nil, &out); err != nil {
			return errMsg{err}
		}
		return taskUpdatedMsg(out)
	})
}

// ── entry point ───────────────────────────────────────────────────────────────

func runTUI(cfg *Config) error {
	m := newTUIModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
