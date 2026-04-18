<#
.SYNOPSIS
  Install media115 CLI binaries and/or agent skills on Windows.
.EXAMPLE
  # Install CLI binaries only
  .\install.ps1
.EXAMPLE
  # Install agent skills for all agents
  .\install.ps1 -Skill -Agent all
#>
param(
    [switch]$Skill,
    [ValidateSet("claude", "agents", "all")]
    [string]$Agent,
    [string]$Version,
    [switch]$Help
)

Set-PSDebug -Trace 1
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$Repo = "newcoderlife/media115"
$Branch = "master"
$RawBase = "https://raw.githubusercontent.com/$Repo/$Branch"
$Skills = @("auth", "dedup", "doctor", "organize", "scan", "scrape", "scrape-fix", "sync")

function Show-Usage {
    Write-Host @"
Install media115 CLI binaries and/or agent skills

Usage: .\install.ps1 [options]

Options:
  -Skill              Install agent skills (requires -Agent)
  -Agent AGENT        Target agent: claude, agents (Cursor/Codex/OpenCode), or all
  -Version VERSION    Pin binary version (default: latest)
  -Help               Show this help

Examples:
  .\install.ps1                          # Install CLI binaries only
  .\install.ps1 -Skill -Agent claude     # Install skills for Claude Code
  .\install.ps1 -Skill -Agent agents     # Install skills for Cursor/Codex/OpenCode
  .\install.ps1 -Skill -Agent all        # Install skills for all agents
"@
}

# ── Binary install ────────────────────────────────────────────────────────────

function Install-Binaries {
    if (-not $Version) {
        $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
        $Version = $release.tag_name
        if (-not $Version) { throw "Failed to fetch latest version" }
    }

    $installDir = "$env:LOCALAPPDATA\Microsoft\WindowsApps"

    foreach ($name in @("cloud115", "media115")) {
        $url = "https://github.com/$Repo/releases/download/$Version/$name-windows-amd64.exe"
        $dest = Join-Path $installDir "$name.exe"
        Write-Host "Downloading $name $Version..."
        Invoke-WebRequest $url -OutFile $dest
    }

    Write-Host ""
    Write-Host "Installed to $installDir`:"
    Write-Host "  cloud115 $Version"
    Write-Host "  media115 $Version"
}

# ── Skill install ─────────────────────────────────────────────────────────────

function Install-Skills([string]$Dest) {
    Write-Host "Installing skills -> $Dest"
    New-Item -ItemType Directory -Path $Dest -Force | Out-Null
    Invoke-WebRequest "$RawBase/skills/SKILL.md" -OutFile (Join-Path $Dest "SKILL.md")
    foreach ($skill in $Skills) {
        $dir = Join-Path $Dest $skill
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Invoke-WebRequest "$RawBase/skills/$skill/SKILL.md" -OutFile (Join-Path $dir "SKILL.md")
    }
    Write-Host "  $($Skills.Count) skills installed"
}

# ── Main ──────────────────────────────────────────────────────────────────────

# Support env vars for piped execution (irm ... | iex)
if (-not $Skill -and $env:SKILL) { $Skill = $true }
if (-not $Agent -and $env:AGENT) { $Agent = $env:AGENT }
if (-not $Version -and $env:VERSION) { $Version = $env:VERSION }

if ($Help) { Show-Usage; return }

if ($Skill) {
    if (-not $Agent) { throw "-Skill requires -Agent (claude|agents|all)" }
    switch ($Agent) {
        "claude" { Install-Skills "$env:USERPROFILE\.claude\skills\media115" }
        "agents" { Install-Skills "$env:USERPROFILE\.agents\skills\media115" }
        "all" {
            Install-Skills "$env:USERPROFILE\.claude\skills\media115"
            Install-Skills "$env:USERPROFILE\.agents\skills\media115"
        }
    }
} else {
    Install-Binaries
}
