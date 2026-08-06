[CmdletBinding()]
param(
    [string]$PackageRoot = $PSScriptRoot,
    [switch]$RequireRunning
)

$ErrorActionPreference = "Stop"
$modulePath = Join-Path $PSScriptRoot "Release.Common.psm1"
if (-not (Test-Path -LiteralPath $modulePath)) { $modulePath = Join-Path (Split-Path -Parent $PSScriptRoot) "Release.Common.psm1" }
Import-Module $modulePath -Force

$root = [IO.Path]::GetFullPath($PackageRoot)
$config = Read-CypressJson -Path (Join-Path $root "config\server.json")
if (-not [Net.IPAddress]::TryParse([string]$config.vpnBindAddress, [ref]([Net.IPAddress]$null))) {
    throw "Configured VPN bind address is invalid: $($config.vpnBindAddress)"
}
foreach ($portName in @("blazePort", "diagnosticsPort", "dynastyPort")) {
    $port = [int]$config.$portName
    if ($port -lt 1 -or $port -gt 65535) { throw "Configured $portName is invalid: $port" }
}
foreach ($required in @(
    @{ Path = "bin\dynasty.exe"; Label = "Dynasty service" },
    @{ Path = "bin\cfb27blaze.exe"; Label = "Blaze service" },
    @{ Path = "runtime\node.exe"; Label = "portable Node.js" },
    @{ Path = "tools\cfb27assetexport\main.mjs"; Label = "asset exporter" },
    @{ Path = "tools\cfb27franchise\main.mjs"; Label = "franchise tool" },
    @{ Path = "node_modules\madden-franchise\package.json"; Label = "madden-franchise dependency" },
    @{ Path = ([string]$config.dynastySeed -replace '/', '\'); Label = "Dynasty seed" },
    @{ Path = ("assets\Dynasty_Assets\{0}\franchise-schemas.FTX" -f [int]$config.assetSlot); Label = "main Dynasty schema" },
    @{ Path = ("assets\Dynasty_Assets\{0}\dynasty-dynasty-binary.FTC" -f [int]$config.assetSlot); Label = "Dynasty template" }
)) {
    Assert-CypressFile -Path (Join-Path $root $required.Path) -Label $required.Label | Out-Null
}
if (-not (Test-CFBChunksFile -Path (Join-Path $root ([string]$config.dynastySeed -replace '/', '\')))) {
    throw "Configured Dynasty seed is not an FBCHUNKS save."
}

if ($RequireRunning) {
    $dynasty = Invoke-RestMethod -Uri ("http://127.0.0.1:{0}/health" -f [int]$config.dynastyPort) -TimeoutSec 5
    $blaze = Invoke-RestMethod -Uri ("http://127.0.0.1:{0}/health" -f [int]$config.diagnosticsPort) -TimeoutSec 5
    if ($dynasty.status -ne "ok") { throw "Dynasty health status is not ok." }
    if ($blaze.status -ne "ok") { throw "Blaze health status is not ok." }
}
Write-Host "PASS server validation"
