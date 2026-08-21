<#
.SYNOPSIS
Turns capture mode off and stops the recorder.

.DESCRIPTION
Removes the marker file, so the next game launch goes back to the private server,
and stops the capture proxy. Safe to run even if capture mode was never on.
#>
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$privateRoot = Join-Path $env:APPDATA "Cypress\CFB27\Private"
$marker = Join-Path $privateRoot "capture-mode"

if (Test-Path -LiteralPath $marker) {
    $runDir = (Get-Content -LiteralPath $marker -Raw).Trim()
    Remove-Item -LiteralPath $marker -Force
    Write-Host "capture mode off (marker removed)"
    if ($runDir -and (Test-Path -LiteralPath $runDir)) {
        $captureFile = Join-Path $runDir "cfb27-mitm.jsonl"
        if (Test-Path -LiteralPath $captureFile) {
            $size = (Get-Item -LiteralPath $captureFile).Length
            $lines = (Get-Content -LiteralPath $captureFile | Measure-Object -Line).Lines
            Write-Host "capture: $captureFile"
            Write-Host "recorded $lines record(s), $size byte(s)"
        } else {
            Write-Host "no capture file was produced in $runDir"
        }
    }
} else {
    Write-Host "capture mode was not on"
}

$proxies = Get-Process -Name cfb27mitm -ErrorAction SilentlyContinue
if ($proxies) {
    Stop-Process -InputObject $proxies -Force
    Write-Host "stopped capture proxy"
} else {
    Write-Host "capture proxy was not running"
}

Write-Host ""
Write-Host "The next game launch will use the private server again."
