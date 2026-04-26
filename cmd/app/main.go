package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/carlosmaranje/mango/internal/constants"
)

var configPath string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer cancel()

	root := &cobra.Command{
		Use:           constants.AppName,
		Short:         "Agent Gateway — multi-agent orchestration with Discord and a CLI control plane",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			return runTUI(cfg)
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", fmt.Sprintf("path to config.yaml (default: MANGO_DIR/config.yaml, where MANGO_DIR defaults to ~/.%s)", constants.AppName))

	root.AddCommand(
		newServeCmd(),
		newStatusCmd(),
		newAddCmd(),
		newAgentCmd(),
		newTaskCmd(),
		newConfigCmd(),
	)

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
