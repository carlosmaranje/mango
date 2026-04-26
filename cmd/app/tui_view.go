package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const navWidth = 18

var logoLines = [6]string{
	`███╗   ███╗ █████╗ ███╗   ██╗ ██████╗  ██████╗`,
	`████╗ ████║██╔══██╗████╗  ██║██╔════╝ ██╔═══██╗`,
	`██╔████╔██║███████║██╔██╗ ██║██║  ███╗██║   ██║`,
	`██║╚██╔╝██║██╔══██║██║╚██╗██║██║   ██║██║   ██║`,
	`██║ ╚═╝ ██║██║  ██║██║ ╚████║╚██████╔╝╚██████╔╝`,
	`╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝ ╚═════╝  ╚═════╝`,
}

func (m tuiModel) contentWidth() int  { return m.width - navWidth - 2 }
func (m tuiModel) contentHeight() int { return m.height - 4 } // header + status bar

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

	navW := navWidth
	contentW := m.width - navW - 2

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(navW).Height(m.contentHeight()).Render(m.viewNav()),
		styleDivider.Render(strings.Repeat("│\n", m.contentHeight())),
		lipgloss.NewStyle().Width(contentW).Height(m.contentHeight()).Render(m.viewContent()),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewHeader(),
		body,
		m.viewStatusBar(),
	)
}

// ── header ────────────────────────────────────────────────────────────────────

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
	return styleTitleBar.Width(m.width).Render(
		logo + "  " + tagline + strings.Repeat(" ", gap) + right,
	)
}

// ── nav ───────────────────────────────────────────────────────────────────────

func (m tuiModel) viewNav() string {
	var b strings.Builder
	b.WriteString(styleSectionHeader.PaddingLeft(2).Render("NAVIGATE") + "\n\n")
	for _, navSection := range navSections {
		label := navSection.Icon + " " + navSection.Name
		if navSection.Index == m.section.Index {
			b.WriteString(styleNavItemActive.Width(navWidth-2).Render(label) + "\n")
		} else {
			b.WriteString(styleNavItem.Width(navWidth-2).Render(label) + "\n")
		}
	}
	return b.String()
}

// ── content router ────────────────────────────────────────────────────────────

func (m tuiModel) viewContent() string {
	switch m.section.Index {
	case sectionTasks, sectionChat:
		return m.viewTasks()
	case sectionAgents:
		return m.viewAgents()
	case sectionConfig:
		return m.viewConfigSection()
	}
	return ""
}

// ── tasks section ─────────────────────────────────────────────────────────────

func (m tuiModel) viewTasks() string {
	w := m.contentWidth() - 2
	var b strings.Builder

	b.WriteString(m.viewTasksLogo(w))
	b.WriteString(m.viewTasksInput(w))
	b.WriteString("\n" + styleSectionHeader.Render("RECENT TASKS") + "\n\n")
	b.WriteString(m.viewTaskList(w))
	return b.String()
}

func (m tuiModel) viewTasksLogo(w int) string {
	var b strings.Builder
	lw := lipgloss.Width(logoLines[0])
	logoPad := strings.Repeat(" ", max((w-lw)/2, 0))
	const tagline = "...napping in progress"
	tagPad := strings.Repeat(" ", max((w-lipgloss.Width(tagline))/2, 0))
	b.WriteString("\n")
	for _, line := range logoLines {
		b.WriteString(logoPad + styleLogoMini.Render(line) + "\n")
	}
	b.WriteString(tagPad + styleLogoTagline.Render(tagline) + "\n\n")
	return b.String()
}

func (m tuiModel) viewTasksInput(w int) string {
	var b strings.Builder

	inputBox := styleInput.Width(w - 4).Render(m.input.View())
	if m.input.Focused() {
		inputBox = styleInputFocused.Width(w - 4).Render(m.input.View())
	}
	b.WriteString(inputBox + "\n")

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
	return b.String()
}

func (m tuiModel) viewTaskList(w int) string {
	var b strings.Builder
	start := 0
	if len(m.tasks) > 8 {
		start = len(m.tasks) - 8
	}
	for i := len(m.tasks) - 1; i >= start; i-- {
		t := m.tasks[i]
		icon := taskStatusIcon(t.status)
		stStyle := taskStatusStyle(t.status)

		goal := t.goal
		if maxG := w - 22; len(goal) > maxG {
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

// ── agents section ────────────────────────────────────────────────────────────

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

		row := fmt.Sprintf("  %s  %-18s  %-12s  %s", agentStatusIcon(a.Status), name, status, skills)
		b.WriteString(lipgloss.NewStyle().Width(w).Render(row) + "\n")
	}

	b.WriteString("\n" + styleFaint.Render("  r to refresh  •  enter to reload") + "\n")
	return b.String()
}

// ── config section ────────────────────────────────────────────────────────────

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

// ── status bar ────────────────────────────────────────────────────────────────

func (m tuiModel) viewStatusBar() string {
	gwStatus := styleStatusErr.Render("○ gateway offline")
	if m.gatewayOK {
		gwStatus = styleStatusOK.Render("● gateway ok")
	}

	socket := styleFaint.Render(" (" + m.gatewayMsg + ")")
	hint := styleFaint.Render("tab/shift+tab nav  ? help  q quit")
	gap := m.width - lipgloss.Width(gwStatus) - lipgloss.Width(socket) - lipgloss.Width(hint) - 4
	if gap < 1 {
		gap = 1
	}
	return styleStatusBar.Width(m.width).Render(
		gwStatus + socket + strings.Repeat(" ", gap) + hint,
	)
}

// ── overlays ──────────────────────────────────────────────────────────────────

func (m tuiModel) viewHelp() string {
	title := styleTitleBar.Width(m.width).Render("🥭 mango — keyboard shortcuts")

	keys := [][]string{
		{"tab / shift+tab", "switch sections (←/→ move cursor in chat)"},
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

// ── helpers ───────────────────────────────────────────────────────────────────

func min8(s string) int {
	if len(s) < 8 {
		return len(s)
	}
	return 8
}
