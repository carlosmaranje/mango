package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/carlosmaranje/mango/core"
	"github.com/carlosmaranje/mango/core/llm"
	coretools "github.com/carlosmaranje/mango/core/tools"
	"github.com/carlosmaranje/mango/internal/agentdef"
	"github.com/carlosmaranje/mango/internal/discord"
	"github.com/carlosmaranje/mango/internal/gateway"
	"github.com/carlosmaranje/mango/internal/skill"
	"github.com/carlosmaranje/mango/internal/tools"
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
			return runServe(cmd.Context(), cfg, configPath)
		},
	}
}

func runServe(parent context.Context, cfg *Config, cfgPath string) error {
	ctx, cancel := signal.NotifyContext(parent, shutdownSignals...)
	defer cancel()

	agentsDir := agentdef.ResolveAgentsDir()
	skillsDir := skill.ResolveSkillsDir()
	socketDir := filepath.Dir(cfg.SocketPath)

	for _, dir := range []string{agentsDir, skillsDir, socketDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	skillLoader := skill.NewLoader(skillsDir)

	// mango plugs its concrete, app-specific tools into the reusable core engine.
	opts := core.Options{
		Tools: []coretools.Tool{
			tools.NewGoSolarTool(),
			tools.NewDateTimeTool(),
		},
	}

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

		systemPrompt, err := agentdef.ComposeSystemPrompt(agentsDir, ac.Name, ac.Skills, skillLoader)
		if err != nil {
			return fmt.Errorf("agent %q: %w", ac.Name, err)
		}
		if len(ac.Skills) > 0 {
			log.Printf("agent %q: loaded skills %v from %s", ac.Name, ac.Skills, skillsDir)
		}

		opts.Agents = append(opts.Agents, core.AgentSpec{
			Name:         ac.Name,
			Role:         ac.Role,
			SystemPrompt: systemPrompt,
			LLM:          llmClient,
			Skills:       ac.Skills,
			MaxTokens:    ac.MaxTokens,
			AuthCreds:    ac.AuthCreds,
			WorkDir:      filepath.Join(agentsDir, ac.Name),
			Tools: []coretools.Tool{
				tools.NewIdentityTool(ac.Name, cfg.SocketPath, cfgPath),
			},
		})
	}

	if len(opts.Agents) == 0 {
		log.Printf("warn: no agents configured — tasks will fail. Run 'mango agent create' or edit configuration.")
	}

	engine, err := core.New(opts)
	if err != nil {
		return err
	}
	if err := engine.Start(ctx); err != nil {
		return err
	}

	// This is where the gateway starts
	gw := gateway.NewServer(cfg.SocketPath, cfg.HTTPAddr, engine)
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
		bot, err := discord.NewBot(cfg.Discord.Token, router, engine, cfg.Discord.Global)
		if err != nil {
			return err
		}
		defer func() {
			if err := bot.Close(); err != nil {
				log.Printf("discord: close: %v", err)
			}
		}()

		if err := bot.Start(ctx); err != nil {
			return err
		}
	} else {
		log.Printf("discord: no token configured, skipping")
	}

	<-ctx.Done()
	log.Printf("shutdown: stopping engine")
	engine.Stop()
	return nil
}
