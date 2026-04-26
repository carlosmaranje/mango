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
		m.chatVP.Width = m.contentWidth() - 4
		m.chatVP.Height = m.chatVPHeight()
		m = m.updateChatVP()

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
		case "shift+?":
			m.showHelp = true
		case "tab":
			m = m.cycleSection(+1)
		case "shift+tab":
			m = m.cycleSection(-1)
		case "ctrl+a":
			if m.section.Index == sectionTasks || m.section.Index == sectionChat {
				m = m.toggleAgentInput()
			}
		case "esc":
			if m.showAgentIn {
				m = m.closeAgentInput()
			}
		case "up", "down", "pgup", "pgdown":
			if m.section.Index == sectionChat {
				var cmd tea.Cmd
				m.chatVP, cmd = m.chatVP.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		case "enter":
			if m.section.Index == sectionTasks {
				var enterCmds []tea.Cmd
				m, enterCmds = m.handleTasksEnter()
				cmds = append(cmds, enterCmds...)
			}
			if m.section.Index == sectionChat {
				var enterCmds []tea.Cmd
				m, enterCmds = m.handleChatEnter()
				cmds = append(cmds, enterCmds...)
			}
			if m.section.Index == sectionAgents {
				cmds = append(cmds, loadAgents(m.client, m.ctx))
			}
		case "r":
			if m.section.Index == sectionAgents {
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

	case chatSubmittedMsg:
		var submitCmds []tea.Cmd
		m, submitCmds = m.handleChatSubmitted(taskDTO(msg))
		cmds = append(cmds, submitCmds...)

	case chatUpdatedMsg:
		var updateCmds []tea.Cmd
		m, updateCmds = m.handleChatUpdated(taskDTO(msg))
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
func (m tuiModel) cycleSection(direction int) tuiModel {
	nextSectionIndex := (int(m.section.Index) + direction + len(sectionNames)) % len(sectionNames)
	m.section = navSections[nextSectionIndex]
	m.input.Blur()
	m.chatInput.Blur()
	m.showAgentIn = false
	m.agentInput.Blur()
	m.agentInput.SetValue("")
	switch m.section.Index {
	case sectionTasks:
		m.input.Focus()
	case sectionChat:
		m.chatInput.Focus()
	}
	return m
}

func (m tuiModel) toggleAgentInput() tuiModel {
	m.showAgentIn = !m.showAgentIn
	if m.showAgentIn {
		m.input.Blur()
		m.chatInput.Blur()
		m.agentInput.Focus()
	} else {
		m.agentInput.Blur()
		m.agentInput.SetValue("")
		m.focusActiveInput()
	}
	return m
}

func (m tuiModel) closeAgentInput() tuiModel {
	m.showAgentIn = false
	m.agentInput.Blur()
	m.agentInput.SetValue("")
	m.focusActiveInput()
	return m
}

func (m *tuiModel) focusActiveInput() {
	switch m.section.Index {
	case sectionTasks:
		m.input.Focus()
	case sectionChat:
		m.chatInput.Focus()
	}
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

// routeInputUpdate forwards msg to the active text input for the current section.
func (m tuiModel) routeInputUpdate(msg tea.Msg) (tuiModel, tea.Cmd) {
	var cmd tea.Cmd
	if m.showAgentIn && m.agentInput.Focused() {
		m.agentInput, cmd = m.agentInput.Update(msg)
		return m, cmd
	}
	switch m.section.Index {
	case sectionTasks:
		m.input, cmd = m.input.Update(msg)
	case sectionChat:
		m.chatInput, cmd = m.chatInput.Update(msg)
	}
	return m, cmd
}

// ── chat handlers ─────────────────────────────────────────────────────────────

func (m tuiModel) handleChatEnter() (tuiModel, []tea.Cmd) {
	if m.showAgentIn && m.agentInput.Focused() {
		m.chatAgent = strings.TrimSpace(m.agentInput.Value())
		m.agentInput.Blur()
		m.agentInput.SetValue("")
		m.showAgentIn = false
		m.chatInput.Focus()
		return m, nil
	}
	if m.chatLoading {
		return m, nil
	}
	text := strings.TrimSpace(m.chatInput.Value())
	if text == "" {
		return m, nil
	}
	m.chatInput.SetValue("")
	m.chatMessages = append(m.chatMessages, chatMessage{role: "user", content: text})
	m.chatLoading = true
	m = m.updateChatVP()
	return m, []tea.Cmd{submitChatMsg(m.client, m.ctx, text, m.chatAgent)}
}

func (m tuiModel) handleChatSubmitted(dto taskDTO) (tuiModel, []tea.Cmd) {
	return m, []tea.Cmd{pollChatMsg(m.client, m.ctx, dto.ID)}
}

func (m tuiModel) handleChatUpdated(dto taskDTO) (tuiModel, []tea.Cmd) {
	if dto.Status != "done" && dto.Status != "failed" {
		return m, []tea.Cmd{pollChatMsg(m.client, m.ctx, dto.ID)}
	}
	content := dto.Result
	if dto.Status == "failed" {
		content = "error: " + dto.Error
	}
	name := m.chatAgent
	if name == "" {
		name = "mango"
	}
	m.chatMessages = append(m.chatMessages, chatMessage{role: "agent", content: content, agentName: name})
	m.chatLoading = false
	m = m.updateChatVP()
	return m, nil
}

func (m tuiModel) updateChatVP() tuiModel {
	w := m.contentWidth() - 4
	if w < 20 {
		w = 20
	}
	m.chatVP.SetContent(m.renderChatContent(w))
	m.chatVP.GotoBottom()
	return m
}

func (m tuiModel) chatVPHeight() int {
	h := m.contentHeight() - 7 // agent bar(2) + thinking/spacer(1) + input(3) + hints(1)
	if m.showAgentIn {
		h -= 3
	}
	if h < 1 {
		return 1
	}
	return h
}
