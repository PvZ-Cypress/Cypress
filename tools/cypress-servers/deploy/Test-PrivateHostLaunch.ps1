$ErrorActionPreference = "Stop"

$scriptPath = Join-Path $PSScriptRoot "start-cfb27-private-host.ps1"
$scriptText = Get-Content -LiteralPath $scriptPath -Raw

$activation = '$env:CYPRESS_CFB27_PRIVATE_ONLINE_DYNASTY = "1"'
$launch = '$gameProc = Start-Process'
$activationOffset = $scriptText.IndexOf($activation, [StringComparison]::Ordinal)
$launchOffset = $scriptText.IndexOf($launch, [StringComparison]::Ordinal)

if ($activationOffset -lt 0) {
    throw "private host launch must activate the bridge with CYPRESS_CFB27_PRIVATE_ONLINE_DYNASTY=1"
}
if ($launchOffset -lt 0 -or $activationOffset -gt $launchOffset) {
    throw "private bridge activation must be set before the game process starts"
}
if (-not $scriptText.Contains('"privateLaunchExpiresUtc=$privateLaunchExpiresUtc"')) {
    throw "private host launch must persist a time-limited bridge activation lease"
}

Write-Host "PASS Test-PrivateHostLaunch"
