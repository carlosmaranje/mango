package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/carlosmaranje/goclaw/internal/agent"
	"github.com/carlosmaranje/goclaw/internal/constants"
	"github.com/carlosmaranje/goclaw/internal/discord"
	"github.com/carlosmaranje/goclaw/internal/gateway"
	"github.com/carlosmaranje/goclaw/internal/llm"
	"github.com/carlosmaranje/goclaw/internal/memory"
	"github.com/carlosmaranje/goclaw/internal/orchestrator"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the gateway in the foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			return runServe(cmd.Context(), cfg)
		},
	}
}

func runServe(parent context.Context, cfg *Config) error {
	ctx, cancel := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	registry := agent.NewRegistry()
	runners := map[string]*agent.Runner{}
	closers := []func() error{}

	var plannerAgent *agent.Agent

	for _, ac := range cfg.Agents {
		if ac.LLM.Provider == "" {
			log.Printf("warn: agent %q has no LLM provider configured — skipping. Edit config and restart.", ac.Name)
			continue
		}

		llmClient, err := llm.NewClient(llm.ProviderConfig{
			Provider: ac.LLM.Provider,
			Model:    ac.LLM.Model,
			APIKey:   ac.LLM.APIKey,
			BaseURL:  ac.LLM.BaseURL,
		})
		if err != nil {
			return fmt.Errorf("agent %q: %w", ac.Name, err)
		}

		workDir := ac.WorkDir
		if workDir == "" {
			workDir = filepath.Join(os.TempDir(), constants.AppName, ac.Name)
		}
		mem, err := memory.Open(workDir)
		if err != nil {
			return fmt.Errorf("agent %q memory: %w", ac.Name, err)
		}
		closers = append(closers, mem.Close)

		promptPath := AgentPromptPath(cfg.ConfigDir, ac.Name)
		promptBytes, err := os.ReadFile(promptPath)
		if err != nil {
			return fmt.Errorf("agent %q: read %s: %w", ac.Name, promptPath, err)
		}
		systemPrompt := strings.TrimSpace(string(promptBytes))
		if systemPrompt == "" {
			return fmt.Errorf("agent %q: %s is empty", ac.Name, promptPath)
		}

		a := &agent.Agent{
			Name:         ac.Name,
			WorkDir:      workDir,
			Model:        ac.LLM.Model,
			Role:         ac.Role,
			Capabilities: ac.Capabilities,
			LLM:          llmClient,
			Memory:       mem,
			Session:      agent.NewSessionStore(),
			AuthCreds:    ac.AuthCreds,
			SystemPrompt: systemPrompt,
		}
		if err := registry.Register(a); err != nil {
			return err
		}
		runner := agent.NewRunner(a, 0)
		runners[a.Name] = runner
		if err := runner.Start(ctx); err != nil {
			return err
		}
		if a.Role == "orchestrator" {
			plannerAgent = a
		}
	}

	if len(runners) == 0 {
		log.Printf("warn: no agents configured — tasks will fail. Run 'mango agent create' or edit configuration.")
	}

	var planner *orchestrator.Planner
	if plannerAgent != nil {
		planner = orchestrator.NewPlanner(plannerAgent, registry)
	}
	dispatcher := orchestrator.NewDispatcher(registry, runners, planner)

	gw := gateway.NewServer(cfg.SocketPath, registry, runners, dispatcher)
	if err := gw.Start(ctx); err != nil {
		return err
	}
	log.Printf("gateway: listening on %s", cfg.SocketPath)

	if cfg.Discord.Token != "" {
		bindings := make([]discord.ChannelBinding, 0, len(cfg.Bindings))
		for _, b := range cfg.Bindings {
			bindings = append(bindings, discord.ChannelBinding{ChannelID: b.ChannelID, AgentName: b.Agent})
		}
		router := discord.NewRouter(bindings)
		history := discord.NewChannelHistory(discord.DefaultHistorySize)
		bot, err := discord.NewBot(cfg.Discord.Token, router, history, dispatcher, cfg.Discord.Global)

		defer func(bot *discord.Bot) {
			err := bot.Close()
			if err != nil {

			}
		}(bot)

		if err != nil {
			return err
		}

		if err := bot.Start(ctx); err != nil {
			return err
		}
	} else {
		log.Printf("discord: no token configured, skipping")
	}

	<-ctx.Done()
	log.Printf("shutdown: stopping runners")
	for _, r := range runners {
		r.Stop()
	}
	for _, c := range closers {
		_ = c()
	}
	return nil
}
