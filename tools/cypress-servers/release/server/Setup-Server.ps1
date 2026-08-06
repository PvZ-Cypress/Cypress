[CmdletBinding()]
param(
    [string]$PackageRoot = $PSScriptRoot,
    [Parameter(Mandatory = $true)][string]$VpnBindAddress,
    [string]$VpnRemoteAddress = "100.64.0.0/10",
    [Parameter(Mandatory = $true)][string]$DynastySeed,
    [string]$Profile = "CFB27Private",
    [ValidateRange(0, 99)][int]$AssetSlot = 0,
    [ValidateRange(1, 65535)][int]$BlazePort = 27920,
    [ValidateRange(1, 65535)][int]$DiagnosticsPort = 27921,
    [ValidateRange(1, 65535)][int]$DynastyPort = 27910,
    [switch]$InstallFirewallRule
)

$ErrorActionPreference = "Stop"
$modulePath = Join-Path $PSScriptRoot "Release.Common.psm1"
if (-not (Test-Path -LiteralPath $modulePath)) { $modulePath = Join-Path (Split-Path -Parent $PSScriptRoot) "Release.Common.psm1" }
Import-Module $modulePath -Force

if (-not [Net.IPAddress]::TryParse($VpnBindAddress, [ref]([Net.IPAddress]$null))) {
    throw "VPN bind address is not a valid IP address: $VpnBindAddress"
}
if ([string]::IsNullOrWhiteSpace($Profile)) { throw "Profile must not be empty." }
if (-not (Test-CFBChunksFile -Path $DynastySeed)) { throw "Dynasty seed is not an FBCHUNKS save: $DynastySeed" }

$root = [IO.Path]::GetFullPath($PackageRoot)
$slotRoot = Join-Path $root ("assets\Dynasty_Assets\{0}" -f $AssetSlot)
Assert-CypressFile -Path (Join-Path $slotRoot "franchise-schemas.FTX") -Label "main Dynasty schema" | Out-Null
Assert-CypressFile -Path (Join-Path $slotRoot "dynasty-dynasty-binary.FTC") -Label "Dynasty template" | Out-Null

$configDir = Join-Path $root "config"
$seedDir = Join-Path $root "data\seed"
New-Item -ItemType Directory -Force -Path $configDir, $seedDir | Out-Null
$seedDestination = Join-Path $seedDir "DYNASTY-SEED"
Copy-Item -LiteralPath $DynastySeed -Destination $seedDestination -Force

$config = [ordered]@{
    schemaVersion = 1
    vpnBindAddress = $VpnBindAddress
    vpnRemoteAddress = $VpnRemoteAddress
    blazePort = $BlazePort
    diagnosticsPort = $DiagnosticsPort
    dynastyPort = $DynastyPort
    profile = $Profile
    assetSlot = $AssetSlot
    dynastySeed = "data/seed/DYNASTY-SEED"
}
$configPath = Join-Path $configDir "server.json"
[IO.File]::WriteAllText($configPath, (($config | ConvertTo-Json -Depth 4) + [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))

if ($InstallFirewallRule) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw "Installing the firewall rule requires an elevated PowerShell window."
    }
    $ruleName = "Cypress CFB27 Private Blaze"
    Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue | Remove-NetFirewallRule
    New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Action Allow -Protocol TCP `
        -LocalAddress $VpnBindAddress -LocalPort $BlazePort -RemoteAddress $VpnRemoteAddress | Out-Null
}

Write-Host "Server configured: $configPath"
Write-Host "Player endpoint: ${VpnBindAddress}:$BlazePort"
