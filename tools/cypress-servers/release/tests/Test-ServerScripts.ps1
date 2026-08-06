$ErrorActionPreference = "Stop"

function Assert-True { param([bool]$Condition, [string]$Message) if (-not $Condition) { throw "ASSERT TRUE FAILED: $Message" } }
function Assert-Equal { param($Expected, $Actual, [string]$Message) if ($Expected -ne $Actual) { throw "ASSERT EQUAL FAILED: $Message expected=[$Expected] actual=[$Actual]" } }

$releaseRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$serverSource = Join-Path $releaseRoot "server"
foreach ($name in @("Setup-Server.ps1", "Start-Server.ps1", "Stop-Server.ps1", "Test-Server.ps1")) {
    $path = Join-Path $serverSource $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "missing server script: $path" }
    $tokens = $null
    $errors = $null
    [Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$errors) | Out-Null
    Assert-Equal 0 $errors.Count "$name should parse"
}

$fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ("cypress-server-scripts-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $fixtureRoot | Out-Null
try {
    foreach ($directory in @("assets\Dynasty_Assets\0", "config", "data", "bin", "runtime", "tools", "node_modules")) {
        New-Item -ItemType Directory -Path (Join-Path $fixtureRoot $directory) -Force | Out-Null
    }
    foreach ($name in @("franchise-schemas.FTX", "dynasty-dynasty-binary.FTC")) {
        [IO.File]::WriteAllText((Join-Path $fixtureRoot "assets\Dynasty_Assets\0\$name"), "fixture")
    }
    [IO.File]::WriteAllText((Join-Path $fixtureRoot "data\preserve.txt"), "keep")
    $validSeed = Join-Path $fixtureRoot "valid-seed"
    $invalidSeed = Join-Path $fixtureRoot "invalid-seed"
    [IO.File]::WriteAllBytes($validSeed, [Text.Encoding]::ASCII.GetBytes("FBCHUNKSfixture"))
    [IO.File]::WriteAllBytes($invalidSeed, [Text.Encoding]::ASCII.GetBytes("NOTCHUNKfixture"))

    $setup = Join-Path $serverSource "Setup-Server.ps1"
    & $setup -PackageRoot $fixtureRoot -VpnBindAddress "100.64.0.5" -VpnRemoteAddress "100.64.0.0/10" -DynastySeed $validSeed -Profile "Host" -AssetSlot 0
    Assert-True (Test-Path -LiteralPath (Join-Path $fixtureRoot "config\server.json")) "setup should write server config"
    Assert-Equal "keep" (Get-Content -LiteralPath (Join-Path $fixtureRoot "data\preserve.txt") -Raw) "setup should preserve existing data"
    Assert-True (Test-Path -LiteralPath (Join-Path $fixtureRoot "data\seed\DYNASTY-SEED")) "setup should copy seed"
    $config = Get-Content -LiteralPath (Join-Path $fixtureRoot "config\server.json") -Raw | ConvertFrom-Json
    Assert-Equal "100.64.0.5" $config.vpnBindAddress "VPN bind address"
    Assert-Equal "100.64.0.0/10" $config.vpnRemoteAddress "VPN remote range"

    $invalidFailed = $false
    try { & $setup -PackageRoot $fixtureRoot -VpnBindAddress "100.64.0.5" -DynastySeed $invalidSeed -AssetSlot 0 } catch { $invalidFailed = $true }
    Assert-True $invalidFailed "invalid seed should fail"

    $slotFailed = $false
    try { & $setup -PackageRoot $fixtureRoot -VpnBindAddress "100.64.0.5" -DynastySeed $validSeed -AssetSlot 9 } catch { $slotFailed = $true }
    Assert-True $slotFailed "missing asset slot should fail"

    Write-Host "PASS Test-ServerScripts"
} finally {
    Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
}
