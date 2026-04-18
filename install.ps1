Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Set-PSDebug -Trace 1

$Repo = "newcoderlife/media115"
$Skills = @("cloud115-auth", "cloud115-dedup", "cloud115-doctor", "cloud115-sync", "media115-organize", "media115-scan", "media115-scrape", "media115-scrape-fix", "media115-subscribe", "media115-wishlist")

$Version = if ($env:VERSION) { $env:VERSION } else { $null }

function Install-Skills([string]$Dest) {
    Write-Host "Installing skills -> $Dest"
    foreach ($skill in $Skills) {
        $dir = Join-Path $Dest $skill
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Invoke-WebRequest "https://raw.githubusercontent.com/$Repo/master/skills/$skill/SKILL.md" -OutFile (Join-Path $dir "SKILL.md")
    }
    Write-Host "  $($Skills.Count) skills installed"
}

# ── Install binaries ──────────────────────────────────────────────────────────

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

Write-Host "Installed cloud115 & media115 $Version -> $installDir"

# ── Install skills ────────────────────────────────────────────────────────────

Install-Skills "$env:USERPROFILE\.agents\skills"
if (Test-Path "$env:USERPROFILE\.claude") {
    Install-Skills "$env:USERPROFILE\.claude\skills"
}
if (Test-Path "$env:USERPROFILE\.kiro") {
    Install-Skills "$env:USERPROFILE\.kiro\skills"
}
