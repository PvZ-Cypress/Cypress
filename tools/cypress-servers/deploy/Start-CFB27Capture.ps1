<#
.SYNOPSIS
Starts the recorder and loads the bridge into the already-running game.

.DESCRIPTION
Records a real EA session so the Dynasty commands we cannot implement from the
existing fixtures can be derived from ground truth: the list/load pair behind the
Load Dynasty menu (BootStatus 103/105/110) and the invite/join flow, none of
which appear in any capture we hold.

Run this AFTER the game is at the press-start screen and the anticheat has been
stopped. The bridge cannot be present in the game directory at launch — the
anticheat inspects it and refuses to start — so it is injected into the live
process instead.

The capture proxy listens on each observed port plus 10000, so it never contends
with the private Blaze server on 27920.

The resulting capture contains real account tokens. Redact before sharing.
#>
[CmdletBinding()]
param(
    [int[]]$Ports = @(27920, 443, 11000, 44325),
    [string]$ProcessName = "CollegeFB27.exe",
    [switch]$NoInject
)

$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $PSCommandPath
$servicesDir = Resolve-Path (Join-Path $scriptRoot "..")
$proxyExe = Join-Path $servicesDir "build\cfb27mitm.exe"
$privateRoot = Join-Path $env:APPDATA "Cypress\CFB27\Private"
$marker = Join-Path $privateRoot "capture-mode"

$captureRoot = Join-Path $env:APPDATA "Cypress\CFB27\Capture"
$runDir = Join-Path $captureRoot ("run_" + (Get-Date -Format "yyyyMMdd_HHmmss"))
New-Item -ItemType Directory -Force -Path $runDir, $privateRoot | Out-Null
$captureFile = Join-Path $runDir "cfb27-mitm.jsonl"

if (-not (Test-Path -LiteralPath $proxyExe)) {
    throw "Capture proxy not found at $proxyExe. Build it with: go build -o build\cfb27mitm.exe .\cmd\cfb27mitm"
}

$offset = 10000
foreach ($port in $Ports) {
    $listen = $port + $offset
    $busy = Get-NetTCPConnection -LocalPort $listen -State Listen -ErrorAction SilentlyContinue
    if ($busy) {
        throw "Port $listen is already in use by PID $($busy[0].OwningProcess)."
    }
}

$portArgument = ($Ports -join ",")
$proxy = Start-Process -FilePath $proxyExe -ArgumentList @(
    "-bind", "127.0.0.1",
    "-ports", $portArgument,
    "-log-file", $captureFile
) -WorkingDirectory $runDir -PassThru `
  -RedirectStandardOutput (Join-Path $runDir "proxy.stdout.log") `
  -RedirectStandardError (Join-Path $runDir "proxy.stderr.log") -WindowStyle Hidden
Start-Sleep -Seconds 1
if ($proxy.HasExited) {
    $err = Get-Content -LiteralPath (Join-Path $runDir "proxy.stderr.log") -Raw -ErrorAction SilentlyContinue
    throw "Capture proxy exited immediately. $err"
}

# Written last: while this file exists the bridge sends the game to the proxy
# instead of the private server, whichever way the game is launched.
Set-Content -LiteralPath $marker -Value $runDir -Encoding ASCII

Set-Content -LiteralPath (Join-Path $runDir "proxy.pid") -Value $proxy.Id -Encoding ASCII

if (-not $NoInject) {
    $injector = Join-Path $servicesDir "build\cfb27inject.exe"
    $captureDll = Join-Path $servicesDir "build\cypress_CFB27_capture.dll"
    foreach ($required in @($injector, $captureDll)) {
        if (-not (Test-Path -LiteralPath $required)) {
            Stop-Process -InputObject $proxy -Force -ErrorAction SilentlyContinue
            Remove-Item -LiteralPath $marker -Force -ErrorAction SilentlyContinue
            throw "Missing $required"
        }
    }
    $game = Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($ProcessName)) -ErrorAction SilentlyContinue
    if (-not $game) {
        Stop-Process -InputObject $proxy -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $marker -Force -ErrorAction SilentlyContinue
        throw "$ProcessName is not running. Launch the game, reach the press-start screen, stop the anticheat, then run this again."
    }
    Write-Host "injecting bridge into $ProcessName (pid $($game.Id))"
    & $injector -dll $captureDll -process $ProcessName
    if ($LASTEXITCODE -ne 0) {
        Stop-Process -InputObject $proxy -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $marker -Force -ErrorAction SilentlyContinue
        throw "Injection failed. Run this window as Administrator, and confirm the anticheat is stopped."
    }
}

Write-Host ""
Write-Host "CAPTURE IS LIVE." -ForegroundColor Green
Write-Host "proxy pid=$($proxy.Id)  ports=$(($Ports | ForEach-Object { $_ + $offset }) -join ',')"
Write-Host "capture file: $captureFile"
Write-Host ""
Write-Host "This session reaches EA's servers. It is against EA's terms of" -ForegroundColor Yellow
Write-Host "service and could put the signed-in account at risk." -ForegroundColor Yellow
Write-Host ""
Write-Host "Go now - walk these three flows in the game:" -ForegroundColor Cyan
Write-Host "  1. Open the Load / Continue Dynasty menu"
Write-Host "  2. Create a new Online Dynasty"
Write-Host "  3. Open the Members / invite screen"
Write-Host ""
Write-Host "When finished, close the game and run:  Stop-CFB27Capture.ps1"
