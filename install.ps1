Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Set-PSDebug -Trace 1

$Repo = "newcoderlife/media115"
$Skills = @("cloud115-auth", "cloud115-dedup", "cloud115-doctor", "cloud115-sync", "media115-organize", "media115-scan", "media115-scrape", "media115-scrape-fix", "media115-subscribe", "media115-wishlist")

function Install-Skills([string]$Dest) {
    foreach ($skill in $Skills) {
        $dir = Join-Path $Dest $skill
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Invoke-WebRequest "https://raw.githubusercontent.com/$Repo/master/skills/$skill/SKILL.md" -OutFile (Join-Path $dir "SKILL.md")
    }
}

# ── Binaries ──────────────────────────────────────────────────────────────────

$Version = (Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest").tag_name
$installDir = "$env:LOCALAPPDATA\Microsoft\WindowsApps"
foreach ($name in @("cloud115", "media115")) {
    Invoke-WebRequest "https://github.com/$Repo/releases/download/$Version/$name-windows-amd64.exe" -OutFile (Join-Path $installDir "$name.exe")
}

# ── Config ────────────────────────────────────────────────────────────────────

$configDir = Join-Path $env:APPDATA "media115"
New-Item -ItemType Directory -Path $configDir -Force | Out-Null
$configFile = Join-Path $configDir "config.toml"
if (-not (Test-Path $configFile)) {
    Invoke-WebRequest "https://raw.githubusercontent.com/$Repo/master/config.example.toml" -OutFile $configFile
}

# ── Skills ────────────────────────────────────────────────────────────────────

Install-Skills "$env:USERPROFILE\.agents\skills"
if (Test-Path "$env:USERPROFILE\.claude") { Install-Skills "$env:USERPROFILE\.claude\skills" }
if (Test-Path "$env:USERPROFILE\.kiro")   { Install-Skills "$env:USERPROFILE\.kiro\skills" }

Write-Host "Done: cloud115 & media115 $Version installed"
