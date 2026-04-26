package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}
	cmd.AddCommand(
		newConfigShowCmd(),
		newConfigSetCmd(),
		newConfigAgentCmd(),
		newConfigBindingCmd(),
	)
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := loadRawViper(configPath)
			if err != nil {
				return err
			}
			var cfg Config
			if err := v.Unmarshal(&cfg); err != nil {
				return err
			}
			out, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		},
	}
}

func writeViperConfig(v *viper.Viper) error {
	if v.ConfigFileUsed() == "" {
		path := configPath
		if path == "" {
			path = defaultConfigPath()
		}
		return v.WriteConfigAs(path)
	}
	return v.WriteConfig()
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := loadRawViper(configPath)
			if err != nil {
				return err
			}
			v.Set(args[0], args[1])
			return writeViperConfig(v)
		},
	}
}

func newConfigAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents in configuration",
	}
	cmd.AddCommand(newConfigAgentAddCmd(), newConfigAgentEditCmd(), newConfigAgentRemoveCmd())
	return cmd
}

type agentFlags struct {
	role      string
	skills    string
	provider  string
	model     string
	apiKey    string
	baseURL   string
	authCreds string
}

func parseSkills(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseAuthCreds(s string) map[string]string {
	if s == "" {
		return nil
	}
	res := make(map[string]string)
	pairs := strings.Split(s, ",")
	for _, p := range pairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			res[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return res
}

func newConfigAgentAddCmd() *cobra.Command {
	var f agentFlags
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new agent to configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
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
				Name:   name,
				Role:   f.role,
				Skills: parseSkills(f.skills),
				LLM: LLMConfig{
					Provider: f.provider,
					Model:    f.model,
					APIKey:   f.apiKey,
					BaseURL:  f.baseURL,
				},
				AuthCreds: parseAuthCreds(f.authCreds),
			}
			agents = append(agents, newAgent)
			v.Set("agents", agents)
			if err := writeViperConfig(v); err != nil {
				return err
			}
			fmt.Printf("\nAgent %q added. Create its definition file with:\n  mango agent create %s\n",
				name, name)
			return nil
		},
	}

	addAgentFlags(cmd, &f)
	return cmd
}

func newConfigAgentEditCmd() *cobra.Command {
	var f agentFlags
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit an existing agent in configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			v, err := loadRawViper(configPath)
			if err != nil {
				return err
			}

			var agents []AgentConfig
			if err := v.UnmarshalKey("agents", &agents); err != nil {
				return err
			}

			found := false
			for i := range agents {
				if agents[i].Name == name {
					found = true
					if cmd.Flags().Changed("role") {
						agents[i].Role = f.role
					}
					if cmd.Flags().Changed("skills") {
						agents[i].Skills = parseSkills(f.skills)
					}
					if cmd.Flags().Changed("provider") {
						agents[i].LLM.Provider = f.provider
					}
					if cmd.Flags().Changed("model") {
						agents[i].LLM.Model = f.model
					}
					if cmd.Flags().Changed("api-key") {
						agents[i].LLM.APIKey = f.apiKey
					}
					if cmd.Flags().Changed("base-url") {
						agents[i].LLM.BaseURL = f.baseURL
					}
					if cmd.Flags().Changed("auth-creds") {
						agents[i].AuthCreds = parseAuthCreds(f.authCreds)
					}
					break
				}
			}

			if !found {
				return fmt.Errorf("agent %q not found", name)
			}

			v.Set("agents", agents)
			return v.WriteConfig()
		},
	}

	addAgentFlags(cmd, &f)
	return cmd
}

func addAgentFlags(cmd *cobra.Command, f *agentFlags) {
	cmd.Flags().StringVar(&f.role, "role", "", "agent role")
	cmd.Flags().StringVar(&f.skills, "skills", "", "comma-separated list of skills")
	cmd.Flags().StringVar(&f.provider, "provider", "", "LLM provider")
	cmd.Flags().StringVar(&f.model, "model", "", "LLM model")
	cmd.Flags().StringVar(&f.apiKey, "api-key", "", "API key")
	cmd.Flags().StringVar(&f.baseURL, "base-url", "", "Base URL for the LLM provider")
	cmd.Flags().StringVar(&f.authCreds, "auth-creds", "", "comma-separated list of key=value pairs for auth credentials")
}

func newConfigAgentRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an agent from configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			v, err := loadRawViper(configPath)
			if err != nil {
				return err
			}

			var agents []AgentConfig
			if err := v.UnmarshalKey("agents", &agents); err != nil {
				return err
			}

			found := false
			newAgents := make([]AgentConfig, 0, len(agents))
			for _, a := range agents {
				if a.Name == name {
					found = true
					continue
				}
				newAgents = append(newAgents, a)
			}

			if !found {
				return fmt.Errorf("agent %q not found", name)
			}

			v.Set("agents", newAgents)
			return v.WriteConfig()
		},
	}
}

func newConfigBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "binding",
		Short: "Manage channel bindings in configuration",
	}
	cmd.AddCommand(newConfigBindingAddCmd(), newConfigBindingRemoveCmd())
	return cmd
}

func newConfigBindingAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <channel-id> <agent-name>",
		Short: "Add a channel binding to configuration",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			channelID := args[0]
			agentName := args[1]
			v, err := loadRawViper(configPath)
			if err != nil {
				return err
			}

			var bindings []BindingConfig
			if err := v.UnmarshalKey("bindings", &bindings); err != nil {
				return err
			}

			for _, b := range bindings {
				if b.ChannelID == channelID {
					return fmt.Errorf("binding for channel %q already exists", channelID)
				}
			}

			newBinding := BindingConfig{
				ChannelID: channelID,
				Agent:     agentName,
			}
			bindings = append(bindings, newBinding)
			v.Set("bindings", bindings)

			return v.WriteConfig()
		},
	}
}

func newConfigBindingRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <channel-id>",
		Short: "Remove a channel binding from configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channelID := args[0]
			v, err := loadRawViper(configPath)
			if err != nil {
				return err
			}

			var bindings []BindingConfig
			if err := v.UnmarshalKey("bindings", &bindings); err != nil {
				return err
			}

			found := false
			newBindings := make([]BindingConfig, 0, len(bindings))
			for _, b := range bindings {
				if b.ChannelID == channelID {
					found = true
					continue
				}
				newBindings = append(newBindings, b)
			}

			if !found {
				return fmt.Errorf("binding for channel %q not found", channelID)
			}

			v.Set("bindings", newBindings)
			return v.WriteConfig()
		},
	}
}
