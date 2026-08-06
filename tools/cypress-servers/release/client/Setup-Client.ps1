[CmdletBinding()]
param(
    [string]$PackageRoot = $PSScriptRoot,
    [string]$StateRoot = (Join-Path $env:APPDATA "Cypress\CFB27\Remote"),
    [Parameter(Mandatory = $true)][string]$GameDirectory,
    [Parameter(Mandatory = $true)][string]$ServerAddress,
    [ValidateRange(1, 65535)][int]$BlazePort = 27920,
    [string]$Profile = "LocalPlayer"
)

$ErrorActionPreference = "Stop"
$root = [IO.Path]::GetFullPath($PackageRoot)
$modulePath = Join-Path $root "Release.Common.psm1"
if (-not (Test-Path -LiteralPath $modulePath)) { $modulePath = Join-Path (Split-Path -Parent $PSScriptRoot) "Release.Common.psm1" }
Import-Module $modulePath -Force
if ([string]::IsNullOrWhiteSpace($ServerAddress)) { throw "Server address must not be empty." }
if ([string]::IsNullOrWhiteSpace($Profile)) { throw "Profile must not be empty." }

$manifest = Read-CypressJson -Path (Join-Path $root "compatibility.json")
$executable = Get-CFB27ExecutableInfo -GameDirectory $GameDirectory
$compatibility = Test-CFB27Compatibility -ExecutableInfo $executable -Manifest $manifest
if (-not $compatibility.supported) {
    $reportDir = Join-Path $StateRoot "compatibility-reports"
    New-Item -ItemType Directory -Force -Path $reportDir | Out-Null
    $reportPath = Join-Path $reportDir ("unsupported-{0}.json" -f (Get-Date -Format "yyyyMMdd_HHmmss"))
    $report = [ordered]@{
        schemaVersion = 1
        detectedAtUtc = [DateTime]::UtcNow.ToString("O")
        fileName = $executable.fileName
        size = $executable.size
        fileVersion = $executable.fileVersion
        sha256 = $executable.sha256
    }
    [IO.File]::WriteAllText($reportPath, (($report | ConvertTo-Json -Depth 4) + [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))
    throw "This CFB27 build is not supported. No game files were changed. Compatibility report: $reportPath"
}

$bridgeSource = Assert-CypressFile -Path (Join-Path $root "payload\cypress_CFB27.dll") -Label "CFB27 bridge payload"
$endpointsSource = Assert-CypressFile -Path (Join-Path $root "payload\cfb27-endpoints.json") -Label "CFB27 endpoint payload"
$gameRoot = [IO.Path]::GetFullPath($GameDirectory)
$bridgeDestination = Join-Path $gameRoot "dinput8.dll"
$endpointsDestination = Join-Path $gameRoot "cfb27-endpoints.json"
$backupDir = Join-Path $StateRoot ("backups\" + (Get-Date -Format "yyyyMMdd_HHmmss"))
$backupDll = $null
$backupEndpoints = $null
if (Test-Path -LiteralPath $bridgeDestination -PathType Leaf) {
    New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
    $backupDll = Join-Path $backupDir "dinput8.dll"
    Copy-Item -LiteralPath $bridgeDestination -Destination $backupDll
}
if (Test-Path -LiteralPath $endpointsDestination -PathType Leaf) {
    New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
    $backupEndpoints = Join-Path $backupDir "cfb27-endpoints.json"
    Copy-Item -LiteralPath $endpointsDestination -Destination $backupEndpoints
}

Copy-Item -LiteralPath $bridgeSource -Destination $bridgeDestination -Force
Copy-Item -LiteralPath $endpointsSource -Destination $endpointsDestination -Force
New-Item -ItemType Directory -Force -Path $StateRoot | Out-Null
$clientConfig = [ordered]@{
    schemaVersion = 1
    gameDirectory = $gameRoot
    gameExecutable = $executable.fileName
    gameSha256 = $executable.sha256
    bridgeProfile = $compatibility.build.bridgeProfile
    serverAddress = $ServerAddress
    blazePort = $BlazePort
    profile = $Profile
    installedDllSha256 = (Get-FileHash -LiteralPath $bridgeDestination -Algorithm SHA256).Hash
    installedEndpointsSha256 = (Get-FileHash -LiteralPath $endpointsDestination -Algorithm SHA256).Hash
    backupDll = $backupDll
    backupEndpoints = $backupEndpoints
}
$configPath = Join-Path $StateRoot "client.json"
[IO.File]::WriteAllText($configPath, (($clientConfig | ConvertTo-Json -Depth 5) + [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))
Write-Host "Client configured for ${ServerAddress}:$BlazePort"
Write-Host "Start with Start-Client.ps1"
