<#
.SYNOPSIS
Sets up a SECOND player's PC to join a Cypress CFB27 private server.

.DESCRIPTION
Run this on the joining player's machine (not the host). It installs the Cypress
bridge into the game directory and points it at the host's address, so the
game's traffic goes to the host's private server instead of EA.

Requires: the player's own legitimate CFB27 install, and both machines on the
same Tailscale/ZeroTier network (or the same LAN).

Copy this script and cypress_CFB27.dll to the client machine, then:

    .\Setup-CFB27Client.ps1 -HostAddress 100.110.136.90

Run it elevated: it writes into the game directory under Program Files.

The bridge only arms while the launch lease is valid, so re-run this (or pass
-LeaseHours) before a session rather than leaving it permanently armed.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$HostAddress,
    # The name other players see. Defaults to the Cypress launcher sign-in when
    # there is one; pass it explicitly on a machine without the launcher, which
    # is otherwise the only way this player ends up correctly named.
    [string]$PlayerName = "",
    [string]$GameDir = "C:\Program Files\EA Games\EA SPORTS College Football 27",
    [int]$BlazePort = 27920,
    [string]$BridgeDll = "",
    [int]$LeaseHours = 8
)

$ErrorActionPreference = "Stop"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
if (-not (New-Object Security.Principal.WindowsPrincipal($identity)).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this from an elevated PowerShell (it writes into the game directory)."
}

if ([string]::IsNullOrWhiteSpace($BridgeDll)) {
    $BridgeDll = Join-Path $PSScriptRoot "cypress_CFB27.dll"
}
if (-not (Test-Path -LiteralPath $BridgeDll)) {
    throw "Bridge DLL not found: $BridgeDll (copy cypress_CFB27.dll next to this script)."
}
if (-not (Test-Path -LiteralPath $GameDir)) {
    throw "CFB27 not found at $GameDir. Pass -GameDir with the correct path."
}

# Reachability check first: a wrong address is the most likely mistake, and it is
# far easier to diagnose here than as a silent failure in-game.
Write-Host "checking $HostAddress`:$BlazePort ..."
$probe = Test-NetConnection -ComputerName $HostAddress -Port $BlazePort -WarningAction SilentlyContinue
if (-not $probe.TcpTestSucceeded) {
    Write-Host "WARNING: could not reach ${HostAddress}:$BlazePort" -ForegroundColor Yellow
    Write-Host "  - is the host's server running with -BindAddress $HostAddress ?" -ForegroundColor Yellow
    Write-Host "  - are both machines connected to the same Tailscale network?" -ForegroundColor Yellow
    Write-Host "  - is the host's firewall allowing port $BlazePort ?" -ForegroundColor Yellow
    Write-Host "Continuing setup anyway; fix the above before launching." -ForegroundColor Yellow
} else {
    Write-Host "host is reachable" -ForegroundColor Green
}

# The game must not be running: the DLL cannot be replaced while it is loaded.
foreach ($name in @("CollegeFB27", "CollegeFB27_Trial")) {
    $running = Get-Process -Name $name -ErrorAction SilentlyContinue
    if ($running) {
        Write-Host "closing running game process $name"
        Stop-Process -InputObject $running -Force
        Start-Sleep -Seconds 2
    }
}

# Preserve any pre-existing dinput8.dll that is not ours.
$installed = Join-Path $GameDir "dinput8.dll"
if ((Test-Path -LiteralPath $installed) -and
    -not (Test-Path -LiteralPath "$installed.cypress-backup")) {
    $existing = Get-Item -LiteralPath $installed
    if ($existing.Length -ne (Get-Item -LiteralPath $BridgeDll).Length) {
        Copy-Item -LiteralPath $installed -Destination "$installed.cypress-backup"
        Write-Host "backed up existing dinput8.dll"
    }
}
Copy-Item -LiteralPath $BridgeDll -Destination $installed -Force
Write-Host "installed bridge into $GameDir"

$privateRoot = Join-Path $env:APPDATA "Cypress\CFB27\Private"
New-Item -ItemType Directory -Force -Path $privateRoot | Out-Null
$expires = [DateTimeOffset]::UtcNow.AddHours($LeaseHours).ToUnixTimeSeconds()
@(
    "# Written by Setup-CFB27Client.ps1",
    "blazeHost=$HostAddress",
    "blazePort=$BlazePort",
    "privateLaunchExpiresUtc=$expires",
    "externalPassThrough=false",
    "enableBearSslBypass=true",
    "runDirectory=$privateRoot"
) | Set-Content -LiteralPath (Join-Path $privateRoot "cfb27-bridge.ini") -Encoding ASCII

# A stale capture marker would divert this client to a capture proxy instead of
# the host, so make sure it is not present.
Remove-Item -LiteralPath (Join-Path $privateRoot "capture-mode") -Force -ErrorAction SilentlyContinue

# Tell the host who this player is. The Cypress launcher has already signed the
# user in and stored their username here, so this machine is the authoritative
# source — better than a list maintained on the host. Without a name the host can
# only invent one, and players cannot pick each other out of a friends list.
$launcherData = Join-Path $env:APPDATA "Cypress\launcherdata.json"
$playerName = $PlayerName.Trim()
if (-not [string]::IsNullOrWhiteSpace($playerName)) {
    Write-Host "using the name you supplied: '$playerName'"
} elseif (Test-Path -LiteralPath $launcherData) {
    try {
        $data = Get-Content -LiteralPath $launcherData -Raw | ConvertFrom-Json
        if ($data.Identity -and $data.Identity.Username) { $playerName = [string]$data.Identity.Username }
        elseif ($data.Username) { $playerName = [string]$data.Username }
    } catch {
        Write-Host "WARNING could not read $launcherData ($_)" -ForegroundColor Yellow
    }
}
if ([string]::IsNullOrWhiteSpace($playerName)) {
    Write-Host "No name given and no Cypress launcher sign-in found." -ForegroundColor Yellow
    Write-Host "  Re-run with -PlayerName 'YourName' so other players see you correctly." -ForegroundColor Yellow
} else {
    $registerUrl = "http://${HostAddress}:$BlazePort/cypress/register?name=$([uri]::EscapeDataString($playerName))"
    try {
        Invoke-RestMethod -Uri $registerUrl -TimeoutSec 10 -ErrorAction Stop | Out-Null
        Write-Host "registered with the host as '$playerName'" -ForegroundColor Green
    } catch {
        Write-Host "WARNING could not register with the host: $_" -ForegroundColor Yellow
        Write-Host "  You will still connect, but under a generated name." -ForegroundColor Yellow
    }
}

Write-Host ""
Write-Host "CLIENT READY." -ForegroundColor Green
Write-Host "  host      : ${HostAddress}:$BlazePort"
Write-Host "  lease ends: $([DateTimeOffset]::FromUnixTimeSeconds($expires).ToLocalTime())"
Write-Host "  log       : $privateRoot\cfb27-bridge.log"
Write-Host ""
Write-Host "Now launch CFB27 the same way the host does:" -ForegroundColor Cyan
Write-Host "  1. Start the game normally"
Write-Host "  2. At the press-start screen, take the anticheat down"
Write-Host "  3. Both players: Play Now -> Play a Friend"
Write-Host ""
Write-Host "If it does not connect, send the host cfb27-bridge.log from the path above."
