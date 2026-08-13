# 🤖 Discord Bot Setup for Mango

To connect **Mango** to your Discord server, follow these steps to create a bot, obtain its token, and configure your agents to specific channels.

## 1. Create your Discord Bot

1.  Navigate to the **[Discord Developer Portal](https://discord.com/developers/applications)**.
2.  Click **"New Application"** and give it a name (e.g., `Mango Orchestrator`).
3.  Go to the **"Bot"** tab on the left sidebar.
4.  Click **"Reset Token"** (or **"Copy Token"**) to reveal your bot's secret token.
    -   ⚠️ **IMPORTANT**: Store this safely! This token grants full control over your bot.

## 2. Enable Required Intents

For Mango to read and respond to messages, you **must** enable specific privileged intents:

1.  While still in the **"Bot"** tab, scroll down to the **"Privileged Gateway Intents"** section.
2.  Switch on **"Message Content Intent"**. (This is mandatory for Mango to read messages).
3.  Enable **"Presence Intent"**. (Required for the bot to show its online status and activity).
4.  (Optional) Enable **"Server Members Intent"**.
5.  Click **"Save Changes"**.

## 3. Configure Mango via CLI

Now that you have your token, you can configure Mango without manually editing the YAML file.

### Set your Discord Token
Run the following command in your terminal:
```bash
./mango config set discord.token "YOUR_DISCORD_BOT_TOKEN_HERE"
```

### Bind Channels to Agents
You need to tell Mango which agent should respond in which Discord channel. To get a **Channel ID**, enable "Developer Mode" in Discord (*Settings > Advanced > Developer Mode*), right-click a channel, and select **"Copy ID"**.

To bind a channel to an agent (e.g., `researcher`):
```bash
./mango config binding add "123456789012345678" "researcher"
```

You can repeat this for multiple channels and agents.

### Direct Messages (DMs)
Mango automatically responds to Direct Messages. If you message the bot privately, it will use the **Orchestrator** to handle your request, as DMs are not bound to specific agents by default.

### Mentions and "Global" Listening
By default, Mango only responds in:
1.  **Bound Channels**: Where a specific agent is assigned.
2.  **Mentions**: Any channel where the bot is mentioned (it will use the Orchestrator).
3.  **Direct Messages**: Private conversations with the bot.

If you want the bot to listen and respond to **every message in every channel** it has access to (without needing a mention), you can enable `global` mode:

```bash
./mango config set discord.global true
```
*Note: This can be noisy in large servers, so use it with caution.*

### Routing and Conversation History

- A bound channel routes directly to its configured agent.
- DMs, mentions in unbound channels, and unbound messages accepted by global mode route through the orchestrator.
- The Discord channel ID is used as Mango's `session_id`, so follow-up messages in the same channel share context on both routing paths.
- Session history is held by the core dispatcher, retains the latest 200 user/assistant messages, and is lost when Mango restarts. The older `internal/discord/context.go` buffer is not part of the live bot path.

While a task is running, Mango refreshes Discord's typing indicator every eight seconds. A task is polled for up to five minutes before the reply loop gives up.

## 4. Invite the Bot to your Server

1.  Go to the **"OAuth2"** tab in the Developer Portal, then select **"URL Generator"**.
2.  Under **Scopes**, select `bot` and `applications.commands`.
3.  Under **Bot Permissions**, select:
    -   `Read Messages/View Channels`
    -   `Send Messages`
    -   `Read Message History`
4.  Copy the generated URL and open it in your browser.
5.  Select your server and authorize the bot.

## 5. Start the Gateway

With the token and bindings configured, start the Mango gateway:

```bash
./mango serve
```

Look for the log message: `discord: bot ready as [Username]`. The bot will appear **Online** in your Discord server with the status **"Watching for tasks"**. You can now send messages in your bound Discord channels, and the assigned agent will respond!
