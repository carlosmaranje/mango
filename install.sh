#!/bin/bash
set -e

REPO_URL="https://github.com/carlosmaranje/mango.git"

# Support for uninstallation even if running remotely
if [[ "$1" == "uninstall" ]]; then
	echo "Delegating to uninstall script..."
	if [ ! -f "uninstall.sh" ]; then
		echo "Uninstall script not found locally. Cloning from $REPO_URL..."
		TMP_DIR=$(mktemp -d)
		git clone --depth 1 "$REPO_URL" "$TMP_DIR"
		cd "$TMP_DIR"
	fi
	chmod +x uninstall.sh
	exec ./uninstall.sh
fi

# Support for remote execution (curl -sSL ... | bash)
if [ ! -d "cmd/app" ] || [ ! -f "go.mod" ]; then
	echo "Mango source not found in current directory. Cloning from $REPO_URL..."
	TMP_DIR=$(mktemp -d)
	git clone --depth 1 "$REPO_URL" "$TMP_DIR"
	cd "$TMP_DIR"
fi

echo "Mango Agent Gateway Installer"
echo "=============================="
echo ""

if [[ "$OSTYPE" != "linux-gnu"* ]]; then
	echo "Error: This script is for Linux only"
	exit 1
fi

if ! command -v sudo &> /dev/null; then
	echo "Error: sudo is required"
	exit 1
fi

echo "This script will:"
echo "  1. Build the mango binary"
echo "  2. Install it to /usr/local/bin/mango"
echo "  3. Create a 'mango' system user"
echo "  4. Install systemd service file"
echo "  5. Copy config to ~/.mango/ (resolved from the mango system user's home)"
echo ""
echo "You will be prompted for your sudo password."
echo ""
read -p "Continue? (y/n) " -n 1 -r </dev/tty
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
	echo "Aborted."
	exit 1
fi

# Build binary
echo "Building mango..."
go build -o mango ./cmd/app
if [ ! -f mango ]; then
	echo "Error: Failed to build mango"
	exit 1
fi

# Install binary
echo "Installing binary to /usr/local/bin/mango..."
sudo mv mango /usr/local/bin/mango
sudo chmod +x /usr/local/bin/mango

# Create system user
echo "Creating 'mango' system user..."
if ! id -u mango &> /dev/null; then
	sudo useradd --system --create-home --home-dir /var/lib/mango --shell /usr/sbin/nologin mango
	echo "  Created user: mango"
else
	echo "  User 'mango' already exists"
fi

# Install service file
echo "Installing systemd service file..."
sudo cp deploy/mango.service /etc/systemd/system/mango.service
sudo systemctl daemon-reload

# Resolve the mango data directory: honour MANGO_DIR if the caller set it,
# otherwise derive ~/.mango from the mango system user's home.
if [ -z "$MANGO_DIR" ]; then
	MANGO_HOME=$(getent passwd mango | cut -d: -f6)
	MANGO_DIR="$MANGO_HOME/.mango"
fi
export MANGO_DIR

# Create config directory and install default config
echo "Setting up configuration..."
sudo -u mango mkdir -p "$MANGO_DIR/agents" "$MANGO_DIR/skills"
if [ -f "$MANGO_DIR/config.yaml" ]; then
	echo "  $MANGO_DIR/config.yaml already exists"
	read -p "  Replace with default config? (y/N) " -n 1 -r </dev/tty
	echo
	if [[ $REPLY =~ ^[Yy]$ ]]; then
		if [ -f config/config.default.yaml ]; then
			sudo -u mango cp config/config.default.yaml "$MANGO_DIR/config.yaml"
			echo "  Installed default config with orchestrator + worker agents"
		else
			echo "  Warning: config/config.default.yaml not found; keeping existing config"
		fi
	else
		echo "  Keeping existing config"
	fi
elif [ -f config/config.default.yaml ]; then
	sudo -u mango cp config/config.default.yaml "$MANGO_DIR/config.yaml"
	echo "  Installed default config with orchestrator + worker agents"
else
	echo "  Warning: config/config.default.yaml not found; skipping config install"
fi

# Install default agent definitions
for agent in ORCHESTRATOR WORKER; do
	agent_file="config/agents/${agent}.md"
	if [ -f "$agent_file" ]; then
		target="$MANGO_DIR/agents/${agent}.md"
		if [ -f "$target" ]; then
			echo "  $target already exists"
			read -p "    Replace? (y/N) " -n 1 -r </dev/tty
			echo
			if [[ $REPLY =~ ^[Yy]$ ]]; then
				sudo -u mango cp "$agent_file" "$target"
				echo "    Updated $agent.md"
			fi
		else
			sudo -u mango cp "$agent_file" "$target"
			echo "    Installed $agent.md"
		fi
	fi
done

echo "  Run 'mango add agent <name>' to scaffold a new agent definition"

sudo chown mango:mango /usr/local/bin/mango

# Optional interactive Discord setup
DISCORD_CONFIGURED=0
echo ""
read -p "Configure Discord bot now? (y/N) " -n 1 -r </dev/tty
echo
if [[ $REPLY =~ ^[Yy]$ ]] && [ -f "$MANGO_DIR/config.yaml" ]; then
	read -p "  Discord bot token: " discord_token </dev/tty
	if [ -n "$discord_token" ]; then
		echo "  Bind the bot to:"
		echo "    [g] all channels (global)"
		echo "    [c] a specific list of channel IDs"
		read -p "  Choose [g/c]: " -n 1 -r bind_mode </dev/tty
		echo

		discord_global="false"
		channels_csv=""
		bind_agent=""
		if [[ $bind_mode =~ ^[Gg]$ ]]; then
			discord_global="true"
		else
			read -p "  Channel IDs (comma-separated): " channels_csv </dev/tty
			if [ -n "$channels_csv" ]; then
				read -p "  Bind channels to which agent? [worker]: " bind_agent </dev/tty
				bind_agent="${bind_agent:-worker}"
			fi
		fi

		tmpfile=$(mktemp)
		{
			echo "discord:"
			echo "  token: \"$discord_token\""
			if [ "$discord_global" = "true" ]; then
				echo "  global: true"
			fi
			echo ""
			if [ -n "$channels_csv" ]; then
				echo "bindings:"
				IFS=',' read -ra CHANS <<< "$channels_csv"
				for ch in "${CHANS[@]}"; do
					ch_trimmed=$(echo "$ch" | xargs)
					[ -z "$ch_trimmed" ] && continue
					echo "  - channel_id: \"$ch_trimmed\""
					echo "    agent: $bind_agent"
				done
				echo ""
			fi
		} > "$tmpfile"
		sudo -u mango cat "$MANGO_DIR/config.yaml" >> "$tmpfile"
		sudo -u mango mv "$tmpfile" "$MANGO_DIR/config.yaml"
		DISCORD_CONFIGURED=1
		echo "  Discord configured"
	else
		echo "  No token provided; skipping Discord setup"
	fi
fi

# Optional interactive LLM setup
configure_agent() {
	local agent_name="$1"
	echo ""
	echo "--- Configure agent: $agent_name ---"
	read -p "  provider (anthropic/openai/ollama, leave blank to skip): " provider </dev/tty
	if [ -z "$provider" ]; then
		echo "  skipped $agent_name"
		return
	fi
	read -p "  model: " model </dev/tty
	read -p "  api_key (or \${ENV_VAR}, leave blank for ollama): " api_key </dev/tty
	read -p "  base_url (leave blank for default): " base_url </dev/tty

	local args=(config agent edit "$agent_name" --provider "$provider" --model "$model")
	if [ -n "$api_key" ]; then
		args+=(--api-key "$api_key")
	fi
	if [ -n "$base_url" ]; then
		args+=(--base-url "$base_url")
	fi
	sudo -u mango env MANGO_DIR="$MANGO_DIR" /usr/local/bin/mango "${args[@]}"
	echo "  $agent_name configured"
}

echo ""
read -p "Configure LLM providers now? (y/N) " -n 1 -r </dev/tty
echo
CONFIGURED=0
if [[ $REPLY =~ ^[Yy]$ ]]; then
	configure_agent orchestrator
	configure_agent worker
	CONFIGURED=1
fi

echo ""
echo "Installation complete!"
echo ""
if [ "$CONFIGURED" -eq 0 ] || [ "$DISCORD_CONFIGURED" -eq 0 ]; then
	echo "=== ACTION REQUIRED ==="
	echo ""
	if [ "$CONFIGURED" -eq 0 ]; then
		echo "LLM providers were not configured. Fill in provider, model, and api_key"
		echo "for the orchestrator and worker agents in $MANGO_DIR/config.yaml."
		echo ""
		echo "Supported providers:"
		echo "  - anthropic: Requires ANTHROPIC_API_KEY"
		echo "  - openai:    Requires base_url and OPENAI_API_KEY"
		echo "  - ollama:    Local, no api_key needed (http://localhost:11434)"
		echo ""
	fi
	if [ "$DISCORD_CONFIGURED" -eq 0 ]; then
		echo "Discord was not configured. To enable, add a discord block (and optional"
		echo "bindings) to $MANGO_DIR/config.yaml."
		echo ""
	fi
	echo "After editing the config, reload systemd to apply:"
	echo ""
	echo "  sudo -u mango nano $MANGO_DIR/config.yaml"
	echo "  sudo systemctl daemon-reload"
	echo "  sudo systemctl restart mango"
	echo ""
fi
echo "=== Next steps ==="
echo "  1. Enable:     sudo systemctl enable mango"
echo "  2. Start:      sudo systemctl start mango"
echo "  3. Check logs: journalctl -u mango -f"
echo ""
echo "Once running:"
echo "  mango status"
echo "  mango agent list"
echo "  mango task submit 'Say hello' --wait"
