[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$ReleaseRoot)

$ErrorActionPreference = "Stop"
function Assert-True { param([bool]$Condition, [string]$Message) if (-not $Condition) { throw "ASSERT TRUE FAILED: $Message" } }
function Assert-Equal { param($Expected, $Actual, [string]$Message) if ($Expected -ne $Actual) { throw "ASSERT EQUAL FAILED: $Message expected=[$Expected] actual=[$Actual]" } }

$root = [IO.Path]::GetFullPath($ReleaseRoot)
if (-not (Test-Path -LiteralPath $root -PathType Container)) { throw "release root not found: $root" }
$serverRoot = Join-Path $root "stage\Cypress-CFB27-Server"
$clientRoot = Join-Path $root "stage\Cypress-CFB27-Client"

$serverRequired = @(
    "Setup-Server.ps1", "Start-Server.ps1", "Stop-Server.ps1", "Test-Server.ps1",
    "Release.Common.psm1", "README-SERVER.md", "VERSION.txt", "manifest.json",
    "config\server.example.json", "bin\dynasty.exe", "bin\cfb27blaze.exe",
    "runtime\node.exe", "runtime\LICENSE", "tools\cfb27assetexport\main.mjs",
    "tools\cfb27franchise\main.mjs", "node_modules\madden-franchise\package.json",
    "assets\Dynasty_Assets\0\franchise-schemas.FTX",
    "assets\Dynasty_Assets\0\dynasty-dynasty-binary.FTC"
)
$clientRequired = @(
    "Setup-Client.ps1", "Start-Client.ps1", "Uninstall-Client.ps1",
    "Release.Common.psm1", "README-PLAYER.md", "VERSION.txt", "manifest.json",
    "compatibility.json", "payload\cypress_CFB27.dll", "payload\cfb27-endpoints.json",
    "launcher\CypressLauncher.exe"
)
foreach ($relative in $serverRequired) { Assert-True (Test-Path -LiteralPath (Join-Path $serverRoot $relative) -PathType Leaf) "server file $relative" }
foreach ($relative in $clientRequired) { Assert-True (Test-Path -LiteralPath (Join-Path $clientRoot $relative) -PathType Leaf) "client file $relative" }

foreach ($package in @($serverRoot, $clientRoot)) {
    $badFiles = @(Get-ChildItem -LiteralPath $package -Recurse -File | Where-Object {
        $_.Extension -in @(".db", ".log", ".obj", ".pdb", ".lib", ".exp") -or
        $_.FullName -match '[\\/]captures[\\/]' -or $_.FullName -match '[\\/]\.git[\\/]'
    })
    Assert-Equal 0 $badFiles.Count "package should exclude private and build-only files"
    $manifest = Get-Content -LiteralPath (Join-Path $package "manifest.json") -Raw | ConvertFrom-Json
    foreach ($entry in $manifest.files) {
        $file = Join-Path $package ([string]$entry.path -replace '/', '\')
        Assert-True (Test-Path -LiteralPath $file -PathType Leaf) "manifest path $($entry.path)"
        Assert-Equal ([string]$entry.sha256) (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash "manifest hash $($entry.path)"
    }
}

$version = (Get-Content -LiteralPath (Join-Path $serverRoot "VERSION.txt") -Raw).Trim()
Assert-Equal $version (Get-Content -LiteralPath (Join-Path $clientRoot "VERSION.txt") -Raw).Trim() "package versions"
$serverZip = Join-Path $root ("Cypress-CFB27-Server-{0}-win-x64.zip" -f $version)
$clientZip = Join-Path $root ("Cypress-CFB27-Client-{0}-win-x64.zip" -f $version)
Assert-True (Test-Path -LiteralPath $serverZip -PathType Leaf) "server ZIP"
Assert-True (Test-Path -LiteralPath $clientZip -PathType Leaf) "client ZIP"
Assert-True (Test-Path -LiteralPath (Join-Path $root "SHA256SUMS.txt") -PathType Leaf) "release checksums"
Write-Host "PASS Test-ReleaseLayout"
