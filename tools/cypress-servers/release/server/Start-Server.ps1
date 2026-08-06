[CmdletBinding()]
param([string]$PackageRoot = $PSScriptRoot)

$ErrorActionPreference = "Stop"
$root = [IO.Path]::GetFullPath($PackageRoot)
$modulePath = Join-Path $PSScriptRoot "Release.Common.psm1"
if (-not (Test-Path -LiteralPath $modulePath)) { $modulePath = Join-Path (Split-Path -Parent $PSScriptRoot) "Release.Common.psm1" }
Import-Module $modulePath -Force
& (Join-Path $PSScriptRoot "Test-Server.ps1") -PackageRoot $root
$config = Get-Content -LiteralPath (Join-Path $root "config\server.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$runStateDir = Join-Path $root "run"
$pidFile = Join-Path $runStateDir "server-pids.json"
if (Test-Path -LiteralPath $pidFile) { throw "Server PID file already exists. Run Stop-Server.ps1 first: $pidFile" }

$runId = Get-Date -Format "yyyyMMdd_HHmmss"
$runDir = Join-Path $root "runs\$runId"
$dataDir = Join-Path $root "data"
New-Item -ItemType Directory -Force -Path $runStateDir, $runDir, (Join-Path $dataDir "dynasties") | Out-Null

function Start-CypressProcess {
    param([string]$Name, [string]$FilePath, [string[]]$Arguments)
    return Start-Process -FilePath $FilePath -ArgumentList (ConvertTo-CypressArgumentString -Arguments $Arguments) -WorkingDirectory $runDir `
        -RedirectStandardOutput (Join-Path $runDir "$Name.stdout.log") `
        -RedirectStandardError (Join-Path $runDir "$Name.stderr.log") -WindowStyle Hidden -PassThru
}

function Wait-CypressHealth {
    param([string]$Name, [string]$Uri, [Diagnostics.Process]$Process, [int]$TimeoutSeconds)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if ($Process.HasExited) { throw "$Name exited before becoming healthy with code $($Process.ExitCode)." }
        try {
            $health = Invoke-RestMethod -Uri $Uri -TimeoutSec 3
            if ($health.status -eq "ok") { return }
        } catch { }
        Start-Sleep -Milliseconds 500
    }
    throw "$Name did not become healthy at $Uri within $TimeoutSeconds seconds."
}

$node = Join-Path $root "runtime\node.exe"
$assetRoot = Join-Path $root "assets\Dynasty_Assets"
$catalog = Join-Path $dataDir ("cfb27-assets-slot{0}.json" -f [int]$config.assetSlot)
$exporter = Join-Path $root "tools\cfb27assetexport\main.mjs"
$exportArguments = @($exporter, "--asset-root", $assetRoot, "--slot", [string]$config.assetSlot, "--output", $catalog)
$export = Start-Process -FilePath $node -ArgumentList (ConvertTo-CypressArgumentString -Arguments $exportArguments) -WorkingDirectory $root -Wait -PassThru -WindowStyle Hidden
if ($export.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $catalog)) { throw "Dynasty asset export failed with code $($export.ExitCode)." }

$dynastyExe = Join-Path $root "bin\dynasty.exe"
$blazeExe = Join-Path $root "bin\cfb27blaze.exe"
$seed = Join-Path $root ([string]$config.dynastySeed -replace '/', '\')
$slotRoot = Join-Path $assetRoot ([string]$config.assetSlot)
$dynasty = $null
$blaze = $null
try {
    $dynasty = Start-CypressProcess "dynasty" $dynastyExe @(
        "-bind", "127.0.0.1", "-port", [string]$config.dynastyPort,
        "-schema-root", $slotRoot, "-db", (Join-Path $dataDir "cfb27_dynasty.db"),
        "-seed", $seed, "-data-dir", (Join-Path $dataDir "dynasties"),
        "-node", $node, "-franchise-tool", (Join-Path $root "tools\cfb27franchise\main.mjs")
    )
    Wait-CypressHealth "Dynasty" ("http://127.0.0.1:{0}/health" -f [int]$config.dynastyPort) $dynasty 300
    $blaze = Start-CypressProcess "cfb27blaze" $blazeExe @(
        "-bind", [string]$config.vpnBindAddress, "-port", [string]$config.blazePort,
        "-diagnostics-bind", "127.0.0.1", "-diagnostics-port", [string]$config.diagnosticsPort,
        "-dynasty-url", ("http://127.0.0.1:{0}" -f [int]$config.dynastyPort),
        "-coach-catalog", $catalog, "-profile", [string]$config.profile,
        "-tls-mode", "tls13", "-run-id", $runId, "-log-file", (Join-Path $runDir "cfb27-blaze.jsonl")
    )
    Wait-CypressHealth "Blaze" ("http://127.0.0.1:{0}/health" -f [int]$config.diagnosticsPort) $blaze 30
    $state = [ordered]@{
        runId = $runId
        runDirectory = $runDir
        dynasty = @{ pid = $dynasty.Id; executable = $dynastyExe }
        blaze = @{ pid = $blaze.Id; executable = $blazeExe }
    }
    $temporary = "$pidFile.tmp"
    [IO.File]::WriteAllText($temporary, (($state | ConvertTo-Json -Depth 5) + [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))
    Move-Item -LiteralPath $temporary -Destination $pidFile -Force
} catch {
    if ($null -ne $blaze -and -not $blaze.HasExited) { Stop-Process -Id $blaze.Id -Force -ErrorAction SilentlyContinue }
    if ($null -ne $dynasty -and -not $dynasty.HasExited) { Stop-Process -Id $dynasty.Id -Force -ErrorAction SilentlyContinue }
    throw
}
Write-Host "CFB27 private server ready at $($config.vpnBindAddress):$($config.blazePort)"
Write-Host "Run logs: $runDir"
