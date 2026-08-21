<#
.SYNOPSIS
Injects the ProtoSSL capture hook into the running game.

.DESCRIPTION
Captures decrypted Blaze frames by hooking DirtySDK's ProtoSSLSend/Recv inside
the live game - the same layer EA-MITM uses, but located and hooked by a build
we control.

Run AFTER the game is at the press-start screen with the anticheat stopped, from
an elevated window.

Two modes:
  -Mode probe  (default) reads and logs the bytes at the configured addresses and
               installs nothing. It cannot crash. Use it first to confirm the
               addresses are real function entries in this build.
  -Mode hook   MinHooks ProtoSSLSend/Recv/Connect (threads suspended during the
               patch) and dumps decrypted frames to capture.acp2.

Addresses come from the [ProtoSSL] section and default to the values from the
EA-MITM ini. The capture contains real account data; redact before sharing.
#>
[CmdletBinding()]
param(
    # hwbp = capture via CPU debug registers (works on Denuvo-protected code).
    # probe = read/log addresses only. hook = legacy MinHook (fails on this game).
    [ValidateSet("probe", "hook", "hwbp")]
    [string]$Mode = "probe",
    [string]$ProcessName = "CollegeFB27.exe",
    [string]$Connect = "0x016D4120",
    [string]$Send = "0x016D4940",
    [string]$Recv = "0x016D4830",
    # Which functions to hook, for bisecting a crash. Default to Connect only:
    # it is called rarely, so if hooking crashes, it is the hook mechanism, not a
    # torn hot-path write. Once Connect proves safe, add Recv,Send.
    [string[]]$Hooks = @("Connect")
)

$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $PSCommandPath
$servicesDir = Resolve-Path (Join-Path $scriptRoot "..")
$injector = Join-Path $servicesDir "build\cfb27inject.exe"
$dll = Join-Path $servicesDir "build\capturehook.dll"
foreach ($required in @($injector, $dll)) {
    if (-not (Test-Path -LiteralPath $required)) { throw "Missing $required" }
}

$captureRoot = Join-Path $env:APPDATA "Cypress\CFB27\Capture"
$runDir = Join-Path $captureRoot ("proto_" + (Get-Date -Format "yyyyMMdd_HHmmss"))
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

# Inject a per-run COPY, not build\capturehook.dll: the game keeps an injected
# DLL memory-mapped, which locks the file and blocks rebuilds. The DLL reads its
# ini and defaults its dump/log to its own directory, so a copy in the run dir
# keeps everything for this run together and the canonical build stays free.
$runDll = Join-Path $runDir "capturehook.dll"
Copy-Item -LiteralPath $dll -Destination $runDll -Force
$dll = $runDll
$hookConnect = if ($Hooks -contains "Connect") { 1 } else { 0 }
$hookRecv = if ($Hooks -contains "Recv") { 1 } else { 0 }
$hookSend = if ($Hooks -contains "Send") { 1 } else { 0 }
$ini = @(
    "[Capture]",
    "Mode=$Mode",
    "DumpDir=$runDir",
    "[ProtoSSL]",
    "Connect=$Connect",
    "Send=$Send",
    "Recv=$Recv",
    "[Hooks]",
    "Connect=$hookConnect",
    "Recv=$hookRecv",
    "Send=$hookSend"
) -join "`r`n"
Set-Content -LiteralPath (Join-Path $runDir "capturehook.ini") -Value $ini -Encoding ASCII

$game = Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($ProcessName)) -ErrorAction SilentlyContinue
if (-not $game) {
    throw "$ProcessName is not running. Reach the press-start screen with the anticheat stopped, then run this."
}

Write-Host "mode=$Mode  hooks=$($Hooks -join ',')  target=$ProcessName (pid $($game.Id))"
Write-Host "run dir: $runDir"
& $injector -dll $dll -process $ProcessName
if ($LASTEXITCODE -ne 0) {
    throw "Injection failed. Run this window as Administrator and confirm the anticheat is stopped."
}

Start-Sleep -Seconds 2
$log = Join-Path $runDir "capturehook.log"
Write-Host ""
if (Test-Path -LiteralPath $log) {
    Write-Host "--- capturehook.log ---" -ForegroundColor Green
    Get-Content -LiteralPath $log
} else {
    Write-Host "no log yet at $log" -ForegroundColor Yellow
}
Write-Host ""
if ($Mode -eq "probe") {
    Write-Host "If the addresses read as function entries, re-run with -Mode hwbp,"
    Write-Host "then walk the Dynasty screens. Dump: $runDir\capture.acp2"
} else {
    Write-Host "Capture is armed. Walk these in the game, then close it:" -ForegroundColor Cyan
    Write-Host "  1. Load / Continue Dynasty menu"
    Write-Host "  2. Create a new Online Dynasty"
    Write-Host "  3. Members / invite screen"
    Write-Host "Dump: $runDir\capture.acp2"
    Write-Host "(check capturehook.log for 'hwbp: capture armed')"
}
