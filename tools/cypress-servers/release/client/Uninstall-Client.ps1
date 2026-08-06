[CmdletBinding()]
param(
    [string]$PackageRoot = $PSScriptRoot,
    [string]$StateRoot = (Join-Path $env:APPDATA "Cypress\CFB27\Remote")
)

$ErrorActionPreference = "Stop"
$configPath = Join-Path $StateRoot "client.json"
if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
    Write-Host "No CFB27 remote client installation is recorded."
    return
}
$config = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json

function Restore-CypressFile {
    param([string]$Destination, [string]$InstalledHash, [AllowNull()][string]$Backup)
    if (Test-Path -LiteralPath $Destination -PathType Leaf) {
        $currentHash = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash
        if (-not [string]::Equals($currentHash, $InstalledHash, [StringComparison]::OrdinalIgnoreCase)) {
            Write-Warning "Leaving modified file in place: $Destination"
            return
        }
        Remove-Item -LiteralPath $Destination -Force
    }
    if (-not [string]::IsNullOrWhiteSpace($Backup) -and (Test-Path -LiteralPath $Backup -PathType Leaf)) {
        Copy-Item -LiteralPath $Backup -Destination $Destination
    }
}

$gameRoot = [string]$config.gameDirectory
Restore-CypressFile (Join-Path $gameRoot "dinput8.dll") ([string]$config.installedDllSha256) ([string]$config.backupDll)
Restore-CypressFile (Join-Path $gameRoot "cfb27-endpoints.json") ([string]$config.installedEndpointsSha256) ([string]$config.backupEndpoints)
Remove-Item -LiteralPath $configPath -Force
Remove-Item -LiteralPath (Join-Path $StateRoot "cfb27-bridge.ini") -Force -ErrorAction SilentlyContinue
Write-Host "CFB27 remote client uninstalled."
