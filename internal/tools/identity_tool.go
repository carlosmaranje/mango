package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"
)

type IdentityTool struct {
	agentName  string
	socketPath string
	configPath string
	startedAt  time.Time
}

func NewIdentityTool(agentName, socketPath, configPath string) *IdentityTool {
	return &IdentityTool{
		agentName:  agentName,
		socketPath: socketPath,
		configPath: configPath,
		startedAt:  time.Now(),
	}
}

func (t *IdentityTool) Name() string { return "identity" }

func (t *IdentityTool) Description() string {
	return "Returns information about this running instance: hostname, agent name, socket path, config path, working directory, OS, and uptime. Use when asked 'where are you running', 'what instance is this', or similar self-identification questions."
}

func (t *IdentityTool) Parameters() []Parameter { return nil }

type IdentityResult struct {
	AgentName  string `json:"agent_name"`
	Hostname   string `json:"hostname"`
	OS         string `json:"os"`
	WorkDir    string `json:"work_dir"`
	SocketPath string `json:"socket_path"`
	ConfigPath string `json:"config_path"`
	UptimeMs   int64  `json:"uptime_ms"`
	StartedAt  string `json:"started_at"`
}

func (t *IdentityTool) Returns() string {
	return DescribeReturnType(IdentityResult{})
}

func (t *IdentityTool) Execute(_ context.Context, _ string) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	workDir, err := os.Getwd()
	if err != nil {
		workDir = "unknown"
	}

	result := IdentityResult{
		AgentName:  t.agentName,
		Hostname:   hostname,
		OS:         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		WorkDir:    workDir,
		SocketPath: t.socketPath,
		ConfigPath: t.configPath,
		UptimeMs:   time.Since(t.startedAt).Milliseconds(),
		StartedAt:  t.startedAt.UTC().Format(time.RFC3339),
	}

	out, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal identity: %w", err)
	}
	return string(out), nil
}
