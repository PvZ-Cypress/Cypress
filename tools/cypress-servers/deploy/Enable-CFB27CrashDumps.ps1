<#
.SYNOPSIS
Turns on full Windows crash dumps for CollegeFB27.exe.

.DESCRIPTION
If the bridge crashes the game on a thread we do not control (for example the
game calling through a patched pointer), our own logging cannot see it. A WER
local dump captures the faulting module and offset regardless of which thread
faults. Run once, elevated.

Dumps land in %LOCALAPPDATA%\CrashDumps\CollegeFB27.exe.*.dmp
#>
[CmdletBinding()]
param(
    [ValidateSet("CollegeFB27.exe", "CollegeFB27_Trial.exe")]
    [string]$ProcessName = "CollegeFB27.exe"
)

$ErrorActionPreference = "Stop"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
if (-not (New-Object Security.Principal.WindowsPrincipal($identity)).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this from an elevated PowerShell (it writes to HKLM)."
}

$base = "HKLM:\SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps"
$key = Join-Path $base $ProcessName
$dumpFolder = Join-Path $env:LOCALAPPDATA "CrashDumps"

New-Item -Path $key -Force | Out-Null
New-ItemProperty -Path $key -Name "DumpFolder" -Value $dumpFolder -PropertyType ExpandString -Force | Out-Null
New-ItemProperty -Path $key -Name "DumpCount" -Value 10 -PropertyType DWord -Force | Out-Null
# 2 = full dump: includes the faulting module list and thread stacks.
New-ItemProperty -Path $key -Name "DumpType" -Value 2 -PropertyType DWord -Force | Out-Null

New-Item -ItemType Directory -Force -Path $dumpFolder | Out-Null

Write-Host "full crash dumps enabled for $ProcessName" -ForegroundColor Green
Write-Host "dumps will be written to: $dumpFolder"
