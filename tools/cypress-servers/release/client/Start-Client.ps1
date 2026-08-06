[CmdletBinding()]
param(
    [string]$PackageRoot = $PSScriptRoot,
    [string]$StateRoot = (Join-Path $env:APPDATA "Cypress\CFB27\Remote"),
    [switch]$NoLaunch
)

$ErrorActionPreference = "Stop"
$root = [IO.Path]::GetFullPath($PackageRoot)
$modulePath = Join-Path $root "Release.Common.psm1"
if (-not (Test-Path -LiteralPath $modulePath)) { $modulePath = Join-Path (Split-Path -Parent $PSScriptRoot) "Release.Common.psm1" }
Import-Module $modulePath -Force
$config = Read-CypressJson -Path (Join-Path $StateRoot "client.json")
$manifest = Read-CypressJson -Path (Join-Path $root "compatibility.json")
$executable = Get-CFB27ExecutableInfo -GameDirectory ([string]$config.gameDirectory)
$compatibility = Test-CFB27Compatibility -ExecutableInfo $executable -Manifest $manifest
if (-not $compatibility.supported) { throw "The installed CFB27 executable changed and is not supported. Run Setup-Client.ps1 to create an update report." }

$bridgePath = Assert-CypressFile -Path (Join-Path ([string]$config.gameDirectory) "dinput8.dll") -Label "installed CFB27 bridge"
$endpointsPath = Assert-CypressFile -Path (Join-Path ([string]$config.gameDirectory) "cfb27-endpoints.json") -Label "installed endpoint manifest"
if (-not [string]::Equals((Get-FileHash -LiteralPath $bridgePath -Algorithm SHA256).Hash, [string]$config.installedDllSha256, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Installed dinput8.dll differs from the bridge recorded by setup."
}

New-Item -ItemType Directory -Force -Path $StateRoot | Out-Null
$bridgeConfigPath = Join-Path $StateRoot "cfb27-bridge.ini"
$runDir = Join-Path $StateRoot ("runs\" + (Get-Date -Format "yyyyMMdd_HHmmss"))
New-Item -ItemType Directory -Force -Path $runDir | Out-Null
$bridgeConfig = @(
    "# Written by Start-Client.ps1",
    "blazeHost=$($config.serverAddress)",
    "blazePort=$($config.blazePort)",
    "profile=$($config.profile)",
    "runDirectory=$runDir",
    "endpointsFile=$endpointsPath",
    "enableBearSslBypass=true",
    "enableRedirectorNetworkRedirect=true",
    "enableCandidateEndpointRedirects=false",
    "enableProtoSslVerifyProbe=false",
    "enableCertVerifyHook=false",
    "certVerifyForce=false",
    "enableFailStateWatch=false"
)
[IO.File]::WriteAllLines($bridgeConfigPath, $bridgeConfig, [Text.Encoding]::ASCII)

$startInfo = New-Object Diagnostics.ProcessStartInfo
$startInfo.FileName = $executable.fullPath
$startInfo.WorkingDirectory = [string]$config.gameDirectory
$startInfo.Arguments = '-playerName "' + ([string]$config.profile -replace '"', '\"') + '" -console -allowMultipleInstances -Game.Platform GamePlatform_Win32 -name "Cypress CFB27 Private"'
$startInfo.UseShellExecute = $false
$startInfo.EnvironmentVariables["CYPRESS_EMBEDDED"] = "1"
$startInfo.EnvironmentVariables["CYPRESS_CFB27_PRIVATE_ONLINE_DYNASTY"] = "1"
$startInfo.EnvironmentVariables["CYPRESS_CFB27_EXTERNAL_PASSTHROUGH"] = "0"
$startInfo.EnvironmentVariables["CYPRESS_CFB27_DISCOVERY"] = "1"
$startInfo.EnvironmentVariables["CYPRESS_CFB27_BLAZE_HOST"] = [string]$config.serverAddress
$startInfo.EnvironmentVariables["CYPRESS_CFB27_BLAZE_PORT"] = [string]$config.blazePort
$startInfo.EnvironmentVariables["CYPRESS_CFB27_PROFILE"] = [string]$config.profile
$startInfo.EnvironmentVariables["CYPRESS_CFB27_RUN_DIR"] = $runDir
$startInfo.EnvironmentVariables["CYPRESS_CFB27_BRIDGE_CONFIG"] = $bridgeConfigPath
$startInfo.EnvironmentVariables["CYPRESS_CFB27_ENDPOINTS_FILE"] = $endpointsPath
if ($NoLaunch) {
    Write-Host "PASS client launch validation"
    return
}
$process = [Diagnostics.Process]::Start($startInfo)
Write-Host "CFB27 launched with PID $($process.Id) for server $($config.serverAddress):$($config.blazePort)"
