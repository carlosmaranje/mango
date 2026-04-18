package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var configPath string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	root := &cobra.Command{
		Use:           "myapp",
		Short:         "Agent Gateway — multi-agent orchestration with Discord and a CLI control plane",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to config.yaml (default: ./config.yaml or /etc/myapp/config.yaml)")

	root.AddCommand(
		newServeCmd(),
		newStatusCmd(),
		newAgentCmd(),
		newTaskCmd(),
	)

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
