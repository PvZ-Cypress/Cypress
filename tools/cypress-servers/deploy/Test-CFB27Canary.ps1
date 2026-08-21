<#
.SYNOPSIS
Injects a do-nothing canary DLL into the running game to isolate the capture
crash.

.DESCRIPTION
The canary writes one marker file from DllMain and does nothing else - no
threads, no patching. Run it exactly like the capture: game at the press-start
screen with the anticheat stopped, this window elevated.

  Game survives + canary.txt appears -> injection is safe; the bridge's own
                                         startup is what crashes. I fix the bridge.
  Game crashes                        -> the process rejects any injected module;
                                         injection is a dead end and I change tack.
#>
[CmdletBinding()]
param(
    [string]$ProcessName = "CollegeFB27.exe"
)

$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $PSCommandPath
$servicesDir = Resolve-Path (Join-Path $scriptRoot "..")
$injector = Join-Path $servicesDir "build\cfb27inject.exe"
$canary = Join-Path $servicesDir "build\canary.dll"
$marker = Join-Path $env:APPDATA "Cypress\CFB27\Private\canary.txt"

foreach ($required in @($injector, $canary)) {
    if (-not (Test-Path -LiteralPath $required)) { throw "Missing $required" }
}

$game = Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($ProcessName)) -ErrorAction SilentlyContinue
if (-not $game) {
    throw "$ProcessName is not running. Reach the press-start screen with the anticheat stopped, then run this."
}

Remove-Item -LiteralPath $marker -Force -ErrorAction SilentlyContinue
Write-Host "injecting canary into $ProcessName (pid $($game.Id))"
& $injector -dll $canary -process $ProcessName
$injectExit = $LASTEXITCODE

Start-Sleep -Seconds 3
$stillAlive = -not (Get-Process -Id $game.Id -ErrorAction SilentlyContinue).HasExited 2>$null
$stillAlive = [bool](Get-Process -Id $game.Id -ErrorAction SilentlyContinue)

Write-Host ""
if (Test-Path -LiteralPath $marker) {
    Write-Host "RESULT: canary marker was written." -ForegroundColor Green
    Get-Content -LiteralPath $marker | ForEach-Object { Write-Host "  $_" }
} else {
    Write-Host "RESULT: no canary marker." -ForegroundColor Yellow
}
if ($stillAlive) {
    Write-Host "RESULT: game is still running after injection." -ForegroundColor Green
    Write-Host "-> Injection is safe. Report this and I will fix the bridge startup."
} else {
    Write-Host "RESULT: game exited after injection." -ForegroundColor Red
    Write-Host "-> The process rejects injected modules. Report this and I will change approach."
}
if ($injectExit -ne 0) {
    Write-Host "(injector reported a non-zero exit; include that in your report)"
}
