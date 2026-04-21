package main

import "github.com/charmbracelet/lipgloss"

// Mango palette — warm orange cat meets tropical fruit
var (
	colorOrange      = lipgloss.Color("#FF8C00")
	colorAmber       = lipgloss.Color("#FFB347")
	colorYellow      = lipgloss.Color("#FFD166")
	colorTropicalGrn = lipgloss.Color("#06D6A0")
	colorCream       = lipgloss.Color("#FFF8E7")
	colorSoftWhite   = lipgloss.Color("#F0E6D3")
	colorMuted       = lipgloss.Color("#8B7355")
	colorDark        = lipgloss.Color("#1A0F00")
	colorDarker      = lipgloss.Color("#120A00")
	colorBorder      = lipgloss.Color("#3D2B1F")
	colorActiveBg    = lipgloss.Color("#2A1A0A")
	colorError       = lipgloss.Color("#FF4500")
	colorSuccess     = lipgloss.Color("#7FFF00")
	colorRunning     = lipgloss.Color("#FFB347")
	colorDim         = lipgloss.Color("#5C4A35")
)

var (
	styleBase = lipgloss.NewStyle().
			Background(colorDarker).
			Foreground(colorCream)

	styleTitle = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true)

	styleTitleBar = lipgloss.NewStyle().
			Background(colorDark).
			Foreground(colorOrange).
			Bold(true).
			Padding(0, 2)

	styleSubtitle = lipgloss.NewStyle().
			Foreground(colorAmber).
			Italic(true)

	styleSectionHeader = lipgloss.NewStyle().
				Foreground(colorYellow).
				Bold(true).
				MarginTop(1)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	styleActiveBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorOrange)

	styleNavItem = lipgloss.NewStyle().
			Foreground(colorMuted).
			PaddingLeft(2)

	styleNavItemActive = lipgloss.NewStyle().
				Foreground(colorOrange).
				Background(colorActiveBg).
				Bold(true).
				PaddingLeft(1).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorOrange)

	styleStatusBar = lipgloss.NewStyle().
			Background(colorDark).
			Foreground(colorMuted).
			Padding(0, 1)

	styleStatusOK = lipgloss.NewStyle().
			Foreground(colorTropicalGrn).
			Bold(true)

	styleStatusErr = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	styleStatusRunning = lipgloss.NewStyle().
				Foreground(colorRunning).
				Bold(true)

	styleInput = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAmber).
			Padding(0, 1)

	styleInputFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorOrange).
				Padding(0, 1)

	stylePill = lipgloss.NewStyle().
			Background(colorActiveBg).
			Foreground(colorAmber).
			Padding(0, 1).
			Margin(0, 1, 0, 0)

	stylePillActive = lipgloss.NewStyle().
			Background(colorOrange).
			Foreground(colorDarker).
			Bold(true).
			Padding(0, 1).
			Margin(0, 1, 0, 0)

	styleKeyHint = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)

	styleKeyDesc = lipgloss.NewStyle().
			Foreground(colorDim)

	styleResult = lipgloss.NewStyle().
			Foreground(colorSoftWhite).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	styleLogoMini = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true)

	styleLogoTagline = lipgloss.NewStyle().
				Foreground(colorAmber).
				Italic(true)

	styleTaskDone = lipgloss.NewStyle().
			Foreground(colorSuccess)

	styleTaskFailed = lipgloss.NewStyle().
			Foreground(colorError)

	styleTaskRunning = lipgloss.NewStyle().
				Foreground(colorRunning)

	styleAgentOnline = lipgloss.NewStyle().
				Foreground(colorTropicalGrn)

	styleAgentOffline = lipgloss.NewStyle().
				Foreground(colorMuted)

	styleDivider = lipgloss.NewStyle().
			Foreground(colorBorder)

	styleFaint = lipgloss.NewStyle().
			Foreground(colorDim)
)

func taskStatusStyle(status string) lipgloss.Style {
	switch status {
	case "done":
		return styleTaskDone
	case "failed":
		return styleTaskFailed
	default:
		return styleTaskRunning
	}
}

func taskStatusIcon(status string) string {
	switch status {
	case "done":
		return "✓"
	case "failed":
		return "✗"
	default:
		return "⟳"
	}
}

func agentStatusIcon(status string) string {
	if status == "running" {
		return styleAgentOnline.Render("●")
	}
	return styleAgentOffline.Render("○")
}
