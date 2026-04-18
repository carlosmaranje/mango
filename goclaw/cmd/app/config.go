package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

type LLMConfig struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`
	APIKey   string `mapstructure:"api_key"`
	BaseURL  string `mapstructure:"base_url"`
}

type AgentConfig struct {
	Name         string            `mapstructure:"name"`
	WorkDir      string            `mapstructure:"work_dir"`
	Role         string            `mapstructure:"role"`
	Capabilities []string          `mapstructure:"capabilities"`
	LLM          LLMConfig         `mapstructure:"llm"`
	AuthCreds    map[string]string `mapstructure:"auth_creds"`
}

type BindingConfig struct {
	ChannelID string `mapstructure:"channel_id"`
	Agent     string `mapstructure:"agent"`
}

type DiscordConfig struct {
	Token string `mapstructure:"token"`
}

type Config struct {
	SocketPath string          `mapstructure:"socket_path"`
	Discord    DiscordConfig   `mapstructure:"discord"`
	Agents     []AgentConfig   `mapstructure:"agents"`
	Bindings   []BindingConfig `mapstructure:"bindings"`
}

func defaultSocketPath() string {
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".myapp", "myapp.sock")
		}
	}
	return "/var/run/myapp/myapp.sock"
}

func loadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetDefault("socket_path", defaultSocketPath())
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/myapp")
		_ = v.ReadInConfig()
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	expandConfig(&cfg)
	if cfg.SocketPath == "" {
		cfg.SocketPath = defaultSocketPath()
	}
	return &cfg, nil
}

func expandConfig(cfg *Config) {
	cfg.SocketPath = os.ExpandEnv(cfg.SocketPath)
	cfg.Discord.Token = os.ExpandEnv(cfg.Discord.Token)
	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		a.WorkDir = os.ExpandEnv(a.WorkDir)
		a.LLM.APIKey = os.ExpandEnv(a.LLM.APIKey)
		a.LLM.BaseURL = os.ExpandEnv(a.LLM.BaseURL)
		for k, v := range a.AuthCreds {
			a.AuthCreds[k] = os.ExpandEnv(v)
		}
	}
}
