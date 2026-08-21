param(
    [switch]$PreflightOnly,
    [string]$GameDirectory = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\..\.."))
$bridgeRoot = Join-Path $repoRoot "tools\cypress-servers\bridge"
$servicesRoot = Join-Path $repoRoot "tools\cypress-servers"
$launcherProject = Join-Path $repoRoot "Launcher\CypressLauncher.csproj"
$launcherDll = Join-Path $repoRoot "Launcher\cypress_CFB27.dll"
$launcherExe = Join-Path $repoRoot "Launcher\bin\Release\net8.0-windows\CypressLauncher.exe"

$runningLauncher = Get-Process CypressLauncher -ErrorAction SilentlyContinue
if ($runningLauncher) {
    $pids = ($runningLauncher | ForEach-Object { $_.Id }) -join ", "
    throw "Close the running Cypress Launcher before preparing a new capture build (PID: $pids)."
}

if ([string]::IsNullOrWhiteSpace($GameDirectory)) {
    $launcherData = Join-Path $env:APPDATA "Cypress\launcherdata.json"
    if (Test-Path -LiteralPath $launcherData) {
        $settings = Get-Content -Raw -LiteralPath $launcherData | ConvertFrom-Json
        $GameDirectory = [string]$settings.CFB27.GameDirectory
    }
}

if ([string]::IsNullOrWhiteSpace($GameDirectory)) {
    throw "CFB27 game directory is not configured. Set it in Cypress or pass -GameDirectory."
}
$GameDirectory = [IO.Path]::GetFullPath($GameDirectory)
$gameExe = Join-Path $GameDirectory "CollegeFB27.exe"
if (-not (Test-Path -LiteralPath $gameExe)) {
    throw "CollegeFB27.exe was not found at $gameExe"
}

Write-Host "Private Online Dynasty capture preflight"
Write-Host "Repository: $repoRoot"
Write-Host "Game: $gameExe"
Write-Host "External pass-through: disabled"

& (Join-Path $bridgeRoot "test-bridge.ps1")
if ($LASTEXITCODE -ne 0) { throw "Bridge tests failed with exit code $LASTEXITCODE" }

& (Join-Path $bridgeRoot "build-bridge.ps1") -Output $launcherDll
if ($LASTEXITCODE -ne 0) { throw "Bridge build failed with exit code $LASTEXITCODE" }
if (-not (Test-Path -LiteralPath $launcherDll)) { throw "Bridge output was not created: $launcherDll" }

Push-Location $servicesRoot
try {
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw "Go tests failed with exit code $LASTEXITCODE" }
    & (Join-Path $servicesRoot "build.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Private service build failed with exit code $LASTEXITCODE" }
}
finally {
    Pop-Location
}

& dotnet build $launcherProject -c Release -f net8.0-windows --nologo
if ($LASTEXITCODE -ne 0) { throw "Launcher build failed with exit code $LASTEXITCODE" }
if (-not (Test-Path -LiteralPath $launcherExe)) { throw "Launcher output was not created: $launcherExe" }

Write-Host "Preflight complete."
Write-Host "Launcher: $launcherExe"
Write-Host "Capture root: $(Join-Path $env:APPDATA 'Cypress\CFB27\Private\runs')"

if (-not $PreflightOnly) {
    Write-Host "Starting Cypress. Use the CFB27 Host action to begin the private Online Dynasty capture."
    $previousPrepare = $env:CYPRESS_CFB27_PRIVATE_PREPARE
    try {
        $env:CYPRESS_CFB27_PRIVATE_PREPARE = "1"
        Start-Process -FilePath $launcherExe -WorkingDirectory (Split-Path -Parent $launcherExe)
    }
    finally {
        if ($null -eq $previousPrepare) {
            Remove-Item Env:CYPRESS_CFB27_PRIVATE_PREPARE -ErrorAction SilentlyContinue
        } else {
            $env:CYPRESS_CFB27_PRIVATE_PREPARE = $previousPrepare
        }
    }
}
