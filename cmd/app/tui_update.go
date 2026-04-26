package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// ── Update ────────────────────────────────────────────────────────────────────

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resultVP.Width = m.contentWidth() - 4
		m.resultVP.Height = m.contentHeight() - 10

	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.showResult {
			return m.handleResultKey(msg)
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
		case "tab":
			m = m.cycleSection(+1)
		case "shift+tab":
			m = m.cycleSection(-1)
		case "ctrl+a":
			if m.section == sectionTasks {
				m = m.toggleAgentInput()
			}
		case "esc":
			if m.showAgentIn {
				m = m.closeAgentInput()
			}
		case "enter":
			if m.section == sectionTasks {
				var enterCmds []tea.Cmd
				m, enterCmds = m.handleTasksEnter()
				cmds = append(cmds, enterCmds...)
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
		wasOK := m.gatewayOK
		m.gatewayOK = bool(msg)
		if m.gatewayOK && !wasOK {
			cmds = append(cmds, loadAgents(m.client, m.ctx))
		}

	case agentsLoadedMsg:
		m.agents = []agentStatusDTO(msg)

	case taskSubmittedMsg:
		var submitCmds []tea.Cmd
		m, submitCmds = m.handleTaskSubmitted(taskDTO(msg))
		cmds = append(cmds, submitCmds...)

	case taskUpdatedMsg:
		var updateCmds []tea.Cmd
		m, updateCmds = m.handleTaskUpdated(taskDTO(msg))
		cmds = append(cmds, updateCmds...)

	case errMsg:
		m.err = msg.err

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tickMsg:
		cmds = append(cmds, tick(), checkHealth(m.client, m.ctx))
		if m.gatewayOK {
			cmds = append(cmds, loadAgents(m.client, m.ctx))
		}
	}

	var inputCmd tea.Cmd
	m, inputCmd = m.routeInputUpdate(msg)
	cmds = append(cmds, inputCmd)

	return m, tea.Batch(cmds...)
}

// ── key handlers ──────────────────────────────────────────────────────────────

func (m tuiModel) handleResultKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		m.showResult = false
		return m, nil
	}
	var cmd tea.Cmd
	m.resultVP, cmd = m.resultVP.Update(msg)
	return m, cmd
}

// cycleSection advances the active section by dir (+1 or -1) and updates focus.
func (m tuiModel) cycleSection(dir int) tuiModel {
	m.section = section((int(m.section) + dir + len(sectionNames)) % len(sectionNames))
	if m.section == sectionTasks {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
	return m
}

func (m tuiModel) toggleAgentInput() tuiModel {
	m.showAgentIn = !m.showAgentIn
	if m.showAgentIn {
		m.input.Blur()
		m.agentInput.Focus()
	} else {
		m.agentInput.Blur()
		m.agentInput.SetValue("")
		m.input.Focus()
	}
	return m
}

func (m tuiModel) closeAgentInput() tuiModel {
	m.showAgentIn = false
	m.agentInput.Blur()
	m.agentInput.SetValue("")
	m.input.Focus()
	return m
}

// ── task handlers ─────────────────────────────────────────────────────────────

func (m tuiModel) handleTasksEnter() (tuiModel, []tea.Cmd) {
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
	m.loading = true
	m.tasks = append(m.tasks, trackedTask{goal: goal, status: "pending", agent: agentName})
	return m, []tea.Cmd{submitTask(m.client, m.ctx, goal, agentName, len(m.tasks)-1)}
}

func (m tuiModel) handleTaskSubmitted(dto taskDTO) (tuiModel, []tea.Cmd) {
	for i := range m.tasks {
		if m.tasks[i].status == "pending" && m.tasks[i].id == "" {
			m.tasks[i].id = dto.ID
			m.tasks[i].status = dto.Status
			return m, []tea.Cmd{pollTask2(m.client, m.ctx, dto.ID, i)}
		}
	}
	return m, nil
}

func (m tuiModel) handleTaskUpdated(dto taskDTO) (tuiModel, []tea.Cmd) {
	var cmds []tea.Cmd
	for i := range m.tasks {
		if m.tasks[i].id != dto.ID {
			continue
		}
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
	m.loading = false
	for _, t := range m.tasks {
		if t.id != "" && t.status != "done" && t.status != "failed" {
			m.loading = true
			break
		}
	}
	return m, cmds
}

// routeInputUpdate forwards msg to the active text input when in the Tasks section.
func (m tuiModel) routeInputUpdate(msg tea.Msg) (tuiModel, tea.Cmd) {
	if m.section != sectionTasks {
		return m, nil
	}
	var cmd tea.Cmd
	if m.showAgentIn && m.agentInput.Focused() {
		m.agentInput, cmd = m.agentInput.Update(msg)
	} else {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}
