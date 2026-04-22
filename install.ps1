#Requires -Version 5.1
<#
.SYNOPSIS
    Installs the Mango Agent Gateway on Windows.

.DESCRIPTION
    Builds the mango binary, installs it to %LocalAppData%\mango\,
    copies the default configuration to %AppData%\mango\config.yaml,
    and registers a Windows Scheduled Task that starts the gateway at boot.

.PARAMETER Uninstall
    Remove the Scheduled Task, binary, and (optionally) configuration.

.EXAMPLE
    .\install.ps1
    .\install.ps1 -Uninstall
#>
[CmdletBinding()]
param(
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"

# Check for Administrator privileges
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host ""
    Write-Error "Administrative privileges are required to register a Windows Scheduled Task."
    Write-Host "Please restart PowerShell as Administrator and run the script again." -ForegroundColor Yellow
    Write-Host ""
    return
}

$AppName   = "mango"
$TaskName  = "Mango Agent Gateway"
$BinDir    = Join-Path $env:LOCALAPPDATA $AppName
$CfgDir    = Join-Path $env:APPDATA      $AppName
$BinPath   = Join-Path $BinDir "$AppName.exe"
$CfgPath   = Join-Path $CfgDir "config.yaml"

# ── Uninstall ──────────────────────────────────────────────────────────────────
if ($Uninstall) {
    Write-Host "Uninstalling Mango Agent Gateway..." -ForegroundColor Cyan

    # Stop and remove the scheduled task if it exists
    if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
        Stop-ScheduledTask  -TaskName $TaskName -ErrorAction SilentlyContinue
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
        Write-Host "  Removed Scheduled Task: $TaskName"
    } else {
        Write-Host "  Scheduled Task not found, skipping"
    }

    # Remove binary directory
    if (Test-Path $BinDir) {
        Remove-Item -Recurse -Force $BinDir
        Write-Host "  Removed binary directory: $BinDir"
    }

    # Prompt before removing configuration
    $removeCfg = Read-Host "Remove configuration directory $CfgDir? [y/N]"
    if ($removeCfg -match '^[Yy]$') {
        if (Test-Path $CfgDir) {
            Remove-Item -Recurse -Force $CfgDir
            Write-Host "  Removed config directory: $CfgDir"
        }
    } else {
        Write-Host "  Configuration kept at: $CfgDir"
    }

    # Remove binary from PATH if it was added
    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($userPath -split ";" | Where-Object { $_ -eq $BinDir }) {
        $newPath = ($userPath -split ";" | Where-Object { $_ -ne $BinDir }) -join ";"
        [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
        Write-Host "  Removed $BinDir from user PATH"
    }

    Write-Host ""
    Write-Host "Uninstall complete." -ForegroundColor Green
    return
}

# ── Install ────────────────────────────────────────────────────────────────────
Write-Host "Mango Agent Gateway Installer (Windows)" -ForegroundColor Cyan
Write-Host "========================================"
Write-Host ""
Write-Host "This script will:"
Write-Host "  1. Build the mango binary"
Write-Host "  2. Install it to $BinDir"
Write-Host "  3. Copy default configuration to $CfgDir"
Write-Host "  4. Register a Scheduled Task to auto-start the gateway at boot"
Write-Host ""

$confirm = Read-Host "Continue? [y/N]"
if ($confirm -notmatch '^[Yy]$') {
    Write-Host "Aborted." -ForegroundColor Yellow
    return
}

# Verify we are in the repository root
if (-not (Test-Path "cmd\app") -or -not (Test-Path "go.mod")) {
    Write-Error "Run this script from the root of the mango repository."
    return
}

# Build binary
Write-Host ""
Write-Host "Building mango.exe..." -ForegroundColor Cyan
$env:CGO_ENABLED = "1"
go build -o mango.exe .\cmd\app
if (-not (Test-Path "mango.exe")) {
    Write-Error "Build failed: mango.exe not found."
    return
}

# Create directories
Write-Host "Creating directories..."
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
New-Item -ItemType Directory -Force -Path $CfgDir | Out-Null

# Install binary
Write-Host "Installing binary to $BinPath..."
Move-Item -Force mango.exe $BinPath

# Add binary directory to user PATH (if not already present)
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
$pathEntries = $userPath -split ";"
if ($pathEntries -notcontains $BinDir) {
    [Environment]::SetEnvironmentVariable("PATH", "$userPath;$BinDir", "User")
    Write-Host "Added $BinDir to user PATH"
    Write-Host "  (Restart your terminal for PATH changes to take effect)"
}

# Copy default configuration
Write-Host "Setting up configuration..."
if (Test-Path $CfgPath) {
    Write-Host "  $CfgPath already exists — leaving it untouched"
} elseif (Test-Path "config\config.default.yaml") {
    Copy-Item "config\config.default.yaml" $CfgPath
    Write-Host "  Installed default config to $CfgPath"
} else {
    Write-Warning "config\config.default.yaml not found; skipping config install"
}

# Copy agent PULSE.md files
$agentsSrcDir = "config\agents"
if (Test-Path $agentsSrcDir) {
    $agentsDestDir = Join-Path $CfgDir "agents"
    Get-ChildItem $agentsSrcDir -Directory | ForEach-Object {
        $agentName = $_.Name
        $destDir   = Join-Path $agentsDestDir $agentName
        $destFile  = Join-Path $destDir "PULSE.md"
        $srcFile   = Join-Path $_.FullName "PULSE.md"
        if (-not (Test-Path $destFile)) {
            New-Item -ItemType Directory -Force -Path $destDir | Out-Null
            Copy-Item $srcFile $destFile
            Write-Host "  Installed PULSE.md for agent '$agentName'"
        } else {
            Write-Host "  $destFile already exists — leaving it untouched"
        }
    }
}

# Register Scheduled Task (runs as current user at logon, restarts on failure)
Write-Host ""
Write-Host "Registering Scheduled Task '$TaskName'..."

$action   = New-ScheduledTaskAction -Execute $BinPath -Argument "serve --config `"$CfgPath`""
$trigger  = New-ScheduledTaskTrigger -AtLogon
$settings = New-ScheduledTaskSettingsSet `
    -ExecutionTimeLimit    ([TimeSpan]::Zero) `
    -RestartCount          3 `
    -RestartInterval       (New-TimeSpan -Minutes 1) `
    -StartWhenAvailable `
    -MultipleInstances     IgnoreNew
$principal = New-ScheduledTaskPrincipal `
    -UserId    $env:USERNAME `
    -LogonType Interactive

try {
    Register-ScheduledTask `
        -TaskName  $TaskName `
        -Action    $action `
        -Trigger   $trigger `
        -Settings  $settings `
        -Principal $principal `
        -Force | Out-Null
} catch {
    Write-Error "Failed to register Scheduled Task: $_"
    Write-Host "  Ensure you are running PowerShell as Administrator." -ForegroundColor Yellow
    return
}

Write-Host "  Scheduled Task registered (starts at next logon, or start manually below)"

# Optional interactive Discord setup
Write-Host ""
$discordConfigured = $false
$configureDiscord = Read-Host "Configure Discord bot now? [y/N]"
if ($configureDiscord -match '^[Yy]$') {
    $token = Read-Host "  Discord bot token"
    if ($token) {
        $bindMode = Read-Host "  Bind to [g]lobal (all channels) or [c]hannel list? [g/c]"
        $globalBind = $false
        $channelsBlock = ""
        if ($bindMode -match '^[Gg]$') {
            $globalBind = $true
        } else {
            $channelsCsv = Read-Host "  Channel IDs (comma-separated)"
            if ($channelsCsv) {
                $bindAgent = Read-Host "  Bind channels to which agent? [worker]"
                if (-not $bindAgent) { $bindAgent = "worker" }
                $channelsBlock = "bindings:`n"
                foreach ($ch in ($channelsCsv -split ",")) {
                    $ch = $ch.Trim()
                    if ($ch) {
                        $channelsBlock += "  - channel_id: `"$ch`"`n"
                        $channelsBlock += "    agent: $bindAgent`n"
                    }
                }
            }
        }

        $discordBlock  = "discord:`n  token: `"$token`"`n"
        if ($globalBind) { $discordBlock += "  global: true`n" }
        $discordBlock += "`n"
        if ($channelsBlock) { $discordBlock += "$channelsBlock`n" }

        # Update or add discord block in config
        $configContent = if (Test-Path $CfgPath) { Get-Content $CfgPath -Raw } else { "" }
        if ($configContent -match "(?ms)^discord:.*?(?=\n\w+:|\Z)") {
            $configContent = $configContent -replace "(?ms)^discord:.*?(?=\n\w+:|\Z)", $discordBlock.TrimEnd()
        } else {
            $configContent = $discordBlock + $configContent
        }
        Set-Content -Path $CfgPath -Value $configContent
        $discordConfigured = $true
        Write-Host "  Discord configured"
    } else {
        Write-Host "  No token provided; skipping Discord setup"
    }
}

# Optional interactive LLM setup
function Configure-Agent {
    param([string]$AgentName)
    Write-Host ""
    Write-Host "--- Configure agent: $AgentName ---"
    $provider = Read-Host "  provider (anthropic/openai/ollama, leave blank to skip)"
    if (-not $provider) {
        Write-Host "  Skipped $AgentName"
        return
    }
    $model   = Read-Host "  model"
    $apiKey  = Read-Host "  api_key (or `${ENV_VAR}, leave blank for ollama)"
    $baseUrl = Read-Host "  base_url (leave blank for default)"

    $argList = @("--config", $CfgPath, "config", "agent", "edit", $AgentName, "--provider", $provider, "--model", $model)
    if ($apiKey)  { $argList += @("--api-key", $apiKey) }
    if ($baseUrl) { $argList += @("--base-url", $baseUrl) }

    & $BinPath @argList
    Write-Host "  $AgentName configured"
}

Write-Host ""
$llmAnswer = Read-Host "Configure LLM providers now? [y/N]"
$llmConfigured = $false
if ($llmAnswer -match '^[Yy]$') {
    Configure-Agent "orchestrator"
    Configure-Agent "worker"
    $llmConfigured = $true
}

# Summary
Write-Host ""
Write-Host "Installation complete!" -ForegroundColor Green
Write-Host ""

if (-not $llmConfigured -or -not $discordConfigured) {
    Write-Host "=== ACTION REQUIRED ===" -ForegroundColor Yellow
    Write-Host ""
    if (-not $llmConfigured) {
        Write-Host "LLM providers were not configured. Fill in provider, model, and api_key"
        Write-Host "for the orchestrator and worker agents in $CfgPath"
        Write-Host ""
        Write-Host "Supported providers:"
        Write-Host "  - anthropic: Requires ANTHROPIC_API_KEY"
        Write-Host "  - openai:    Requires base_url and OPENAI_API_KEY"
        Write-Host "  - ollama:    Local, no api_key needed (http://localhost:11434)"
        Write-Host ""
    }
    if (-not $discordConfigured) {
        Write-Host "Discord was not configured. To enable, add a discord block (and optional"
        Write-Host "bindings) to $CfgPath"
        Write-Host ""
    }
    Write-Host "After editing the config, restart the gateway (Requires Admin):"
    Write-Host "  notepad $CfgPath"
    Write-Host "  Stop-ScheduledTask  -TaskName '$TaskName'"
    Write-Host "  Start-ScheduledTask -TaskName '$TaskName'"
    Write-Host ""
}

Write-Host "=== Next steps ===" -ForegroundColor Cyan
Write-Host "  Start now:    Start-ScheduledTask -TaskName '$TaskName' (Requires Admin)"
Write-Host "  Stop:         Stop-ScheduledTask  -TaskName '$TaskName' (Requires Admin)"
Write-Host "  Check status: mango status"
Write-Host "  List agents:  mango agent list"
Write-Host "  Submit task:  mango task submit 'Say hello' --wait"
Write-Host ""
Write-Host "The gateway will start automatically at next boot/logon."
