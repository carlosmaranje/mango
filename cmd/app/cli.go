package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check if the gateway is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			client := newGatewayClient(cfg.SocketPath)
			var out map[string]any
			if err := client.request(cmd.Context(), "GET", "/health", nil, &out); err != nil {
				return err
			}
			fmt.Printf("gateway: ok (socket=%s)\n", cfg.SocketPath)
			return nil
		},
	}
}

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Manage agents"}
	cmd.AddCommand(newAgentListCmd(), newAgentStartCmd(), newAgentStopCmd(), newAgentCreateCmd())
	return cmd
}

type agentStatusDTO struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	Role         string   `json:"role,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

func newAgentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered agents and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			client := newGatewayClient(cfg.SocketPath)
			var out []agentStatusDTO
			if err := client.request(cmd.Context(), "GET", "/agents", nil, &out); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSTATUS\tROLE\tCAPABILITIES")
			for _, a := range out {
				caps := ""
				if len(a.Capabilities) > 0 {
					b, _ := json.Marshal(a.Capabilities)
					caps = string(b)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Name, a.Status, a.Role, caps)
			}
			return tw.Flush()
		},
	}
}

func newAgentStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start the runner loop for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			client := newGatewayClient(cfg.SocketPath)
			var out map[string]any
			if err := client.request(cmd.Context(), "POST", "/agents/start", map[string]string{"name": args[0]}, &out); err != nil {
				return err
			}
			fmt.Printf("agent %s: %s\n", args[0], out["status"])
			return nil
		},
	}
}

func newAgentStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop the runner loop for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			client := newGatewayClient(cfg.SocketPath)
			var out map[string]any
			if err := client.request(cmd.Context(), "POST", "/agents/stop", map[string]string{"name": args[0]}, &out); err != nil {
				return err
			}
			fmt.Printf("agent %s: %s\n", args[0], out["status"])
			return nil
		},
	}
}

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "Submit and inspect tasks"}
	cmd.AddCommand(newTaskSubmitCmd(), newTaskStatusCmd())
	return cmd
}

type taskDTO struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func newTaskSubmitCmd() *cobra.Command {
	var agentName string
	var wait bool
	cmd := &cobra.Command{
		Use:   "submit <goal>",
		Short: "Submit a goal to the orchestrator",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			client := newGatewayClient(cfg.SocketPath)
			body := map[string]string{"goal": args[0]}
			if agentName != "" {
				body["agent"] = agentName
			}
			var out taskDTO
			if err := client.request(cmd.Context(), "POST", "/tasks", body, &out); err != nil {
				return err
			}
			fmt.Println(out.ID)
			if wait {
				return pollTask(cmd, client, out.ID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&agentName, "agent", "", "route directly to a specific agent (skip orchestrator)")
	cmd.Flags().BoolVar(&wait, "wait", false, "poll until the task completes")
	return cmd
}

func newTaskStatusCmd() *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Show task status (and result if done)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			client := newGatewayClient(cfg.SocketPath)
			if wait {
				return pollTask(cmd, client, args[0])
			}
			return printTask(cmd, client, args[0])
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "poll until the task completes")
	return cmd
}

func printTask(cmd *cobra.Command, client *gatewayClient, id string) error {
	var out taskDTO
	if err := client.request(cmd.Context(), "GET", "/tasks/"+id, nil, &out); err != nil {
		return err
	}
	fmt.Printf("id: %s\nstatus: %s\n", out.ID, out.Status)
	if out.Result != "" {
		fmt.Printf("result: %s\n", out.Result)
	}
	if out.Error != "" {
		fmt.Printf("error: %s\n", out.Error)
	}
	return nil
}

func pollTask(cmd *cobra.Command, client *gatewayClient, id string) error {
	for {
		var out taskDTO
		if err := client.request(cmd.Context(), "GET", "/tasks/"+id, nil, &out); err != nil {
			return err
		}
		if out.Status == "done" || out.Status == "failed" {
			if out.Result != "" {
				fmt.Println(out.Result)
			}
			if out.Error != "" {
				return fmt.Errorf("task failed: %s", out.Error)
			}
			return nil
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(time.Second):
		}
	}
}

func newAgentCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create a new agent interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			scanner := bufio.NewScanner(os.Stdin)

			fmt.Print("Agent name: ")
			if !scanner.Scan() {
				return fmt.Errorf("failed to read agent name")
			}
			name := strings.TrimSpace(scanner.Text())
			if name == "" {
				return fmt.Errorf("agent name cannot be empty")
			}

			fmt.Print("LLM provider [anthropic/openai/ollama]: ")
			if !scanner.Scan() {
				return fmt.Errorf("failed to read LLM provider")
			}
			provider := strings.TrimSpace(scanner.Text())
			if provider == "" {
				return fmt.Errorf("LLM provider cannot be empty")
			}

			fmt.Print("Model: ")
			if !scanner.Scan() {
				return fmt.Errorf("failed to read model")
			}
			model := strings.TrimSpace(scanner.Text())
			if model == "" {
				return fmt.Errorf("model cannot be empty")
			}

			fmt.Print("API key (or ${ENV_VAR}, leave blank for ollama): ")
			if !scanner.Scan() {
				return fmt.Errorf("failed to read API key")
			}
			apiKey := strings.TrimSpace(scanner.Text())

			fmt.Print("Base URL (leave blank for defaults): ")
			if !scanner.Scan() {
				return fmt.Errorf("failed to read base URL")
			}
			baseURL := strings.TrimSpace(scanner.Text())

			fmt.Print("Role (leave blank for none): ")
			if !scanner.Scan() {
				return fmt.Errorf("failed to read role")
			}
			role := strings.TrimSpace(scanner.Text())

			fmt.Print("Capabilities (comma-separated, leave blank for none): ")
			if !scanner.Scan() {
				return fmt.Errorf("failed to read capabilities")
			}
			capsStr := strings.TrimSpace(scanner.Text())
			capabilities := parseCapabilities(capsStr)

			fmt.Print("Work directory (leave blank for default): ")
			if !scanner.Scan() {
				return fmt.Errorf("failed to read work directory")
			}
			workDir := strings.TrimSpace(scanner.Text())

			v, err := loadRawViper(configPath)
			if err != nil {
				return err
			}

			var agents []AgentConfig
			if err := v.UnmarshalKey("agents", &agents); err != nil {
				return err
			}

			for _, a := range agents {
				if a.Name == name {
					return fmt.Errorf("agent %q already exists", name)
				}
			}

			newAgent := AgentConfig{
				Name:         name,
				WorkDir:      workDir,
				Role:         role,
				Capabilities: capabilities,
				LLM: LLMConfig{
					Provider: provider,
					Model:    model,
					APIKey:   apiKey,
					BaseURL:  baseURL,
				},
			}
			agents = append(agents, newAgent)
			v.Set("agents", agents)

			if v.ConfigFileUsed() == "" {
				path := configPath
				if path == "" {
					path = defaultConfigPath()
				}
				if err := v.WriteConfigAs(path); err != nil {
					return err
				}
			} else {
				if err := v.WriteConfig(); err != nil {
					return err
				}
			}

			fmt.Printf("Agent %q added to %s. Restart 'mango serve' to apply.\n", name, v.ConfigFileUsed())
			return nil
		},
	}
}
