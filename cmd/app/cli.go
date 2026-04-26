package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/carlosmaranje/mango/internal/agent"
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
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Skills []string `json:"skills,omitempty"`
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
			fmt.Fprintln(tw, "NAME\tSTATUS\tSKILLS")
			for _, a := range out {
				skills := ""
				if len(a.Skills) > 0 {
					b, _ := json.Marshal(a.Skills)
					skills = string(b)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", a.Name, a.Status, skills)
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
		Use:        "create <name>",
		Short:      "Scaffold a new agent definition file (deprecated; use 'mango add agent')",
		Args:       cobra.ExactArgs(1),
		Deprecated: `use "mango add agent" instead`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(),
				`Warning: "agent create" is deprecated and will be removed in a future version. Use "mango add agent" instead.`)

			name := args[0]
			if name == "" {
				return fmt.Errorf("agent name cannot be empty")
			}

			dir := agent.ResolveAgentsDir()
			path := agent.AgentDefinitionPath(dir, name)

			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("agent %q already exists at %s", name, path)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("stat %s: %w", path, err)
			}

			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
			}

			template := fmt.Sprintf("# %s\n\nDescribe this agent's role, persona, and constraints here.\n",
				filepath.Base(path[:len(path)-len(filepath.Ext(path))]))
			if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}

			fmt.Printf("Created agent definition at %s\n", path)
			return nil
		},
	}
}
