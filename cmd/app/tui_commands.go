package main

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// tick fires a tickMsg every 5 seconds to refresh health and agent status.
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

func submitChatMsg(c *gatewayClient, ctx context.Context, text, agentName string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]string{"goal": text}
		if agentName != "" {
			body["agent"] = agentName
		}
		var out taskDTO
		if err := c.request(ctx, "POST", "/tasks", body, &out); err != nil {
			return errMsg{err}
		}
		return chatSubmittedMsg(out)
	}
}

func pollChatMsg(c *gatewayClient, ctx context.Context, id string) tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(_ time.Time) tea.Msg {
		var out taskDTO
		if err := c.request(ctx, "GET", "/tasks/"+id, nil, &out); err != nil {
			return errMsg{err}
		}
		return chatUpdatedMsg(out)
	})
}
