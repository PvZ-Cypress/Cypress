$ErrorActionPreference = "Stop"

function Assert-True { param([bool]$Condition, [string]$Message) if (-not $Condition) { throw "ASSERT TRUE FAILED: $Message" } }
function Assert-False { param([bool]$Condition, [string]$Message) if ($Condition) { throw "ASSERT FALSE FAILED: $Message" } }
function Assert-Equal { param($Expected, $Actual, [string]$Message) if ($Expected -ne $Actual) { throw "ASSERT EQUAL FAILED: $Message expected=[$Expected] actual=[$Actual]" } }

$releaseRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$clientSource = Join-Path $releaseRoot "client"
foreach ($name in @("Setup-Client.ps1", "Start-Client.ps1", "Uninstall-Client.ps1")) {
    $path = Join-Path $clientSource $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "missing client script: $path" }
    $tokens = $null
    $errors = $null
    [Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$errors) | Out-Null
    Assert-Equal 0 $errors.Count "$name should parse"
}

$fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ("cypress-client-scripts-" + [guid]::NewGuid().ToString("N"))
$packageRoot = Join-Path $fixtureRoot "package"
$stateRoot = Join-Path $fixtureRoot "state"
$gameRoot = Join-Path $fixtureRoot "game"
New-Item -ItemType Directory -Path (Join-Path $packageRoot "payload"), $gameRoot -Force | Out-Null
try {
    $gameExe = Join-Path $gameRoot "CollegeFB27.exe"
    [IO.File]::WriteAllBytes($gameExe, [Text.Encoding]::ASCII.GetBytes("known-player-game"))
    [IO.File]::WriteAllBytes((Join-Path $packageRoot "payload\cypress_CFB27.dll"), [Text.Encoding]::ASCII.GetBytes("bridge-payload"))
    [IO.File]::WriteAllText((Join-Path $packageRoot "payload\cfb27-endpoints.json"), '{"schemaVersion":1}')
    Copy-Item -LiteralPath (Join-Path $releaseRoot "Release.Common.psm1") -Destination (Join-Path $packageRoot "Release.Common.psm1")
    $hash = (Get-FileHash -LiteralPath $gameExe -Algorithm SHA256).Hash
    $knownManifest = @{ schemaVersion = 1; builds = @(@{ fileName = "CollegeFB27.exe"; sha256 = $hash; size = 17; fileVersion = $null; bridgeProfile = "fixture" }) }
    [IO.File]::WriteAllText((Join-Path $packageRoot "compatibility.json"), ($knownManifest | ConvertTo-Json -Depth 5))

    $existingDll = Join-Path $gameRoot "dinput8.dll"
    [IO.File]::WriteAllText($existingDll, "original-dinput")
    $setup = Join-Path $clientSource "Setup-Client.ps1"
    & $setup -PackageRoot $packageRoot -StateRoot $stateRoot -GameDirectory $gameRoot -ServerAddress "100.64.0.5" -BlazePort 27920 -Profile "PlayerOne"
    Assert-Equal "bridge-payload" (Get-Content -LiteralPath $existingDll -Raw) "bridge should be installed"
    Assert-True (Test-Path -LiteralPath (Join-Path $gameRoot "cfb27-endpoints.json")) "endpoint file should be installed"
    $clientConfig = Get-Content -LiteralPath (Join-Path $stateRoot "client.json") -Raw | ConvertFrom-Json
    Assert-Equal "100.64.0.5" $clientConfig.serverAddress "server address should persist"
    Assert-True (Test-Path -LiteralPath $clientConfig.backupDll) "prior DLL should be backed up"

    & (Join-Path $clientSource "Start-Client.ps1") -PackageRoot $packageRoot -StateRoot $stateRoot -NoLaunch
    $bridgeConfig = Get-Content -LiteralPath (Join-Path $stateRoot "cfb27-bridge.ini") -Raw
    Assert-True ($bridgeConfig.Contains("blazeHost=100.64.0.5")) "bridge config should use remote host"
    Assert-True ($bridgeConfig.Contains("blazePort=27920")) "bridge config should use configured port"

    & (Join-Path $clientSource "Uninstall-Client.ps1") -PackageRoot $packageRoot -StateRoot $stateRoot
    Assert-Equal "original-dinput" (Get-Content -LiteralPath $existingDll -Raw) "uninstall should restore prior DLL"
    Assert-False (Test-Path -LiteralPath (Join-Path $gameRoot "cfb27-endpoints.json")) "uninstall should remove installed endpoints"

    [IO.File]::WriteAllText((Join-Path $packageRoot "compatibility.json"), (@{ schemaVersion = 1; builds = @() } | ConvertTo-Json -Depth 5))
    $gameBefore = (Get-ChildItem -LiteralPath $gameRoot -File | Select-Object -ExpandProperty Name | Sort-Object) -join "|"
    $unknownFailed = $false
    try { & $setup -PackageRoot $packageRoot -StateRoot $stateRoot -GameDirectory $gameRoot -ServerAddress "100.64.0.5" } catch { $unknownFailed = $true }
    Assert-True $unknownFailed "unknown game build should fail"
    $gameAfter = (Get-ChildItem -LiteralPath $gameRoot -File | Select-Object -ExpandProperty Name | Sort-Object) -join "|"
    Assert-Equal $gameBefore $gameAfter "unknown build should not modify game directory"
    Assert-True ((Get-ChildItem -LiteralPath (Join-Path $stateRoot "compatibility-reports") -File).Count -ge 1) "unknown build should create a report"

    $startText = Get-Content -LiteralPath (Join-Path $clientSource "Start-Client.ps1") -Raw
    foreach ($required in @("UseShellExecute", "CYPRESS_CFB27_PRIVATE_ONLINE_DYNASTY", "CYPRESS_CFB27_BLAZE_HOST", "CYPRESS_CFB27_BRIDGE_CONFIG")) {
        Assert-True ($startText.Contains($required)) "Start-Client should contain $required"
    }
    Assert-False ($startText.Contains("dynasty.exe")) "client launch must not start local Dynasty service"
    Assert-False ($startText.Contains("cfb27blaze.exe")) "client launch must not start local Blaze service"

    Write-Host "PASS Test-ClientScripts"
} finally {
    Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
}
