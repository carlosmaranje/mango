#!/bin/bash
set -e

MANGO_DIR="$HOME/.mango"
CONFIG_FILE="$MANGO_DIR/config.yaml"
LOG_DIR="$HOME/Library/Logs/mango"
PLIST_LABEL="com.mango.gateway"
PLIST_DEST="$HOME/Library/LaunchAgents/$PLIST_LABEL.plist"
BINARY_DEST="/usr/local/bin/mango"

if [[ "$1" == "uninstall" ]]; then
	echo "Uninstalling Mango..."
	launchctl unload "$PLIST_DEST" 2>/dev/null || true
	rm -f "$PLIST_DEST"
	rm -f "$BINARY_DEST"
	echo "Removed binary and launch agent."
	echo "Config and data at $MANGO_DIR were left untouched."
	echo "Remove manually if you no longer need them:"
	echo "  rm -rf $MANGO_DIR"
	exit 0
fi

if [[ "$OSTYPE" != "darwin"* ]]; then
	echo "Error: This script is for macOS only. Use install.sh on Linux."
	exit 1
fi

if ! command -v go &> /dev/null; then
	echo "Error: Go is not installed. Install it from https://go.dev/dl/"
	exit 1
fi

echo "Mango Agent Gateway Installer (macOS)"
echo "======================================"
echo ""
echo "This script will:"
echo "  1. Build the mango binary"
echo "  2. Install it to $BINARY_DEST"
echo "  3. Set up config at $CONFIG_FILE"
echo "  4. Install a launchd agent (auto-start on login)"
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
	echo "Error: Build failed"
	exit 1
fi

# Install binary
echo "Installing binary to $BINARY_DEST..."
mkdir -p "$(dirname "$BINARY_DEST")"
mv mango "$BINARY_DEST"
chmod +x "$BINARY_DEST"

# Set up config directory
echo "Setting up config at $MANGO_DIR..."
mkdir -p "$MANGO_DIR"

if [ -f "$CONFIG_FILE" ]; then
	echo "  $CONFIG_FILE already exists — leaving it untouched"
elif [ -f config/config.default.yaml ]; then
	cp config/config.default.yaml "$CONFIG_FILE"
	echo "  Installed default config"
else
	echo "  Warning: config/config.default.yaml not found; skipping"
fi

# Copy PULSE.md files for each agent
if [ -d config/agents ]; then
	for agent_dir in config/agents/*/; do
		[ -d "$agent_dir" ] || continue
		agent_name=$(basename "$agent_dir")
		target="$MANGO_DIR/agents/$agent_name/PULSE.md"
		if [ -f "$target" ]; then
			echo "  $target already exists — leaving it untouched"
		else
			mkdir -p "$MANGO_DIR/agents/$agent_name"
			cp "$agent_dir/PULSE.md" "$target"
			echo "  Installed PULSE.md for agent '$agent_name'"
		fi
	done
fi

# Set up log directory
mkdir -p "$LOG_DIR"

# Install launchd plist
echo "Installing launchd agent..."
mkdir -p "$(dirname "$PLIST_DEST")"
sed \
	-e "s|MANGO_CONFIG_PATH|$CONFIG_FILE|g" \
	-e "s|MANGO_HOME|$HOME|g" \
	-e "s|MANGO_LOG_DIR|$LOG_DIR|g" \
	deploy/mango.plist > "$PLIST_DEST"

# Optional interactive Discord setup
DISCORD_CONFIGURED=0
echo ""
read -p "Configure Discord bot now? (y/N) " -n 1 -r </dev/tty
echo
if [[ $REPLY =~ ^[Yy]$ ]] && [ -f "$CONFIG_FILE" ]; then
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
		cat "$CONFIG_FILE" >> "$tmpfile"
		mv "$tmpfile" "$CONFIG_FILE"
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

	local args=(--config "$CONFIG_FILE" config agent edit "$agent_name" --provider "$provider" --model "$model")
	if [ -n "$api_key" ]; then
		args+=(--api-key "$api_key")
	fi
	if [ -n "$base_url" ]; then
		args+=(--base-url "$base_url")
	fi
	"$BINARY_DEST" "${args[@]}"
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

# Load the launch agent
echo ""
echo "Loading launch agent..."
launchctl unload "$PLIST_DEST" 2>/dev/null || true
launchctl load "$PLIST_DEST"

echo ""
echo "Installation complete!"
echo ""
if [ "$CONFIGURED" -eq 0 ] || [ "$DISCORD_CONFIGURED" -eq 0 ]; then
	echo "=== ACTION REQUIRED ==="
	echo ""
	if [ "$CONFIGURED" -eq 0 ]; then
		echo "LLM providers were not configured. Edit $CONFIG_FILE"
		echo "and fill in provider, model, and api_key for each agent."
		echo ""
		echo "Supported providers:"
		echo "  - anthropic: Requires api_key"
		echo "  - openai:    Requires base_url and api_key"
		echo "  - ollama:    Local, no api_key needed (http://localhost:11434)"
		echo ""
	fi
	if [ "$DISCORD_CONFIGURED" -eq 0 ]; then
		echo "Discord was not configured. Add a discord block to $CONFIG_FILE"
		echo "if you want bot support."
		echo ""
	fi
	echo "After editing the config, restart the agent:"
	echo ""
	echo "  nano $CONFIG_FILE"
	echo "  launchctl unload $PLIST_DEST"
	echo "  launchctl load   $PLIST_DEST"
	echo ""
fi
echo "=== Next steps ==="
echo "  Check status: mango status"
echo "  List agents:  mango agent list"
echo "  Test:         mango task submit 'Say hello' --wait"
echo ""
echo "Logs: tail -f $LOG_DIR/mango.log"
