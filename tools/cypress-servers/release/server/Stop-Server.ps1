[CmdletBinding()]
param([string]$PackageRoot = $PSScriptRoot)

$ErrorActionPreference = "Stop"
$root = [IO.Path]::GetFullPath($PackageRoot)
$pidFile = Join-Path $root "run\server-pids.json"
if (-not (Test-Path -LiteralPath $pidFile -PathType Leaf)) {
    Write-Host "Server is not recorded as running."
    return
}
$state = Get-Content -LiteralPath $pidFile -Raw -Encoding UTF8 | ConvertFrom-Json
foreach ($entry in @($state.blaze, $state.dynasty)) {
    $process = Get-Process -Id ([int]$entry.pid) -ErrorAction SilentlyContinue
    if ($null -eq $process) { continue }
    $actualPath = $null
    try { $actualPath = $process.Path } catch { }
    if ([string]::IsNullOrWhiteSpace($actualPath) -or -not [string]::Equals([IO.Path]::GetFullPath($actualPath), [IO.Path]::GetFullPath([string]$entry.executable), [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to stop PID $($entry.pid): executable path does not match the recorded Cypress service."
    }
    Stop-Process -Id $process.Id -Force
}
Remove-Item -LiteralPath $pidFile -Force
Write-Host "CFB27 private server stopped."
