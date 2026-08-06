[CmdletBinding()]
param(
    [ValidateSet("Release")][string]$Configuration = "Release",
    [string]$DynastyAssetsRoot = "C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets",
    [string]$OutputRoot = "",
    [string]$Version = "",
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"
$releaseSource = $PSScriptRoot
$repoRoot = [IO.Path]::GetFullPath((Join-Path $releaseSource "..\..\.."))
if ([string]::IsNullOrWhiteSpace($OutputRoot)) { $OutputRoot = Join-Path $repoRoot "dist\cfb27-private" }
$outputBase = [IO.Path]::GetFullPath($OutputRoot)
$sourceDirty = @(& git -C $repoRoot status --porcelain --untracked-files=normal).Count -gt 0
if ([string]::IsNullOrWhiteSpace($Version)) {
    $shortHash = (& git -C $repoRoot rev-parse --short HEAD).Trim()
    $Version = "{0}-{1}" -f (Get-Date -Format "yyyy.MM.dd"), $shortHash
    if ($sourceDirty) { $Version += "-dirty" }
}
if ($Version -notmatch '^[0-9A-Za-z._-]+$') { throw "Version contains unsupported filename characters: $Version" }
$releaseRoot = Join-Path $outputBase $Version
$stageRoot = Join-Path $releaseRoot "stage"
$serverStage = Join-Path $stageRoot "Cypress-CFB27-Server"
$clientStage = Join-Path $stageRoot "Cypress-CFB27-Client"
$buildRoot = Join-Path $releaseRoot "build"

if (Test-Path -LiteralPath $releaseRoot) {
    $resolvedBase = [IO.Path]::GetFullPath($outputBase).TrimEnd('\') + '\'
    $resolvedTarget = [IO.Path]::GetFullPath($releaseRoot).TrimEnd('\') + '\'
    if (-not $resolvedTarget.StartsWith($resolvedBase, [StringComparison]::OrdinalIgnoreCase)) { throw "Unsafe release cleanup target: $releaseRoot" }
    Remove-Item -LiteralPath $releaseRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $serverStage, $clientStage, $buildRoot | Out-Null

function Invoke-Checked {
    param([string]$Label, [string]$FilePath, [string[]]$Arguments, [string]$WorkingDirectory)
    Write-Host "==> $Label"
    Push-Location $WorkingDirectory
    try {
        & $FilePath @Arguments
        if ($LASTEXITCODE -ne 0) { throw "$Label failed with exit code $LASTEXITCODE." }
    } finally {
        Pop-Location
    }
}

function Copy-Tree {
    param([string]$Source, [string]$Destination)
    if (-not (Test-Path -LiteralPath $Source)) { throw "Release source not found: $Source" }
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    Get-ChildItem -LiteralPath $Source -Force | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $Destination -Recurse -Force
    }
}

function New-PackageManifest {
    param([string]$PackageRoot, [string]$PackageName)
    $files = @(Get-ChildItem -LiteralPath $PackageRoot -Recurse -File | Where-Object { $_.Name -ne "manifest.json" } | Sort-Object FullName | ForEach-Object {
        [ordered]@{
            path = $_.FullName.Substring($PackageRoot.Length).TrimStart('\').Replace('\', '/')
            size = [int64]$_.Length
            sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash
        }
    })
    $manifest = [ordered]@{ schemaVersion = 1; package = $PackageName; version = $Version; sourceDirty = $sourceDirty; files = $files }
    [IO.File]::WriteAllText((Join-Path $PackageRoot "manifest.json"), (($manifest | ConvertTo-Json -Depth 6) + [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))
}

$powershell = (Get-Command powershell.exe -ErrorAction Stop).Source
$go = (Get-Command go.exe -ErrorAction Stop).Source
$node = (Get-Command node.exe -ErrorAction Stop).Source
$npm = Join-Path (Split-Path -Parent $node) "npm.cmd"
$dotnet = (Get-Command dotnet.exe -ErrorAction Stop).Source
$cmakeCommand = Get-Command cmake.exe -ErrorAction SilentlyContinue
$cmake = if ($null -ne $cmakeCommand) { $cmakeCommand.Source } else { "C:\Program Files\Microsoft Visual Studio\2022\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe" }
if (-not (Test-Path -LiteralPath $cmake -PathType Leaf)) { throw "CMake was not found." }

if (-not $SkipTests) {
    foreach ($testName in @("Test-Release.Common.ps1", "Test-ServerScripts.ps1", "Test-ClientScripts.ps1")) {
        Invoke-Checked $testName $powershell @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", (Join-Path $releaseSource "tests\$testName")) $repoRoot
    }
    Invoke-Checked "Go tests" $go @("test", "./...") (Join-Path $repoRoot "tools\cypress-servers")
    Invoke-Checked "Node tests" $npm @("test") (Join-Path $repoRoot "tools\cypress-servers")
    Invoke-Checked "Launcher tests" $dotnet @("test", "Launcher.Tests\Launcher.Tests.csproj", "-c", "Release") $repoRoot
}

$goBuild = Join-Path $buildRoot "go"
New-Item -ItemType Directory -Force -Path $goBuild | Out-Null
Invoke-Checked "Build Dynasty service" $go @("build", "-trimpath", "-o", (Join-Path $goBuild "dynasty.exe"), "./cmd/dynasty") (Join-Path $repoRoot "tools\cypress-servers")
Invoke-Checked "Build Blaze service" $go @("build", "-trimpath", "-o", (Join-Path $goBuild "cfb27blaze.exe"), "./cmd/cfb27blaze") (Join-Path $repoRoot "tools\cypress-servers")

$bridgeBuild = Join-Path $buildRoot "bridge"
Invoke-Checked "Configure CFB27 bridge" $cmake @("--fresh", "-S", (Join-Path $repoRoot "Server"), "-B", $bridgeBuild, "-A", "x64", "-DCYPRESS_CFB27=ON", "-DBUILD_TESTING=ON") $repoRoot
Invoke-Checked "Build CFB27 bridge" $cmake @("--build", $bridgeBuild, "--config", "Release", "--parallel") $repoRoot
if (-not $SkipTests) { Invoke-Checked "CFB27 bridge tests" $cmake @("--build", $bridgeBuild, "--target", "RUN_TESTS", "--config", "Release") $repoRoot }
$bridgeDll = Join-Path $bridgeBuild "Release\cypress_CFB27.dll"
if (-not (Test-Path -LiteralPath $bridgeDll -PathType Leaf)) { throw "Fresh CFB27 bridge was not produced: $bridgeDll" }

$launcherPublish = Join-Path $buildRoot "launcher"
Invoke-Checked "Publish self-contained launcher" $dotnet @("publish", "Launcher\CypressLauncher.csproj", "-c", "Release", "-f", "net8.0-windows", "-r", "win-x64", "--self-contained", "true", "-o", $launcherPublish, "/p:LangVersion=latest") $repoRoot
Copy-Item -LiteralPath $bridgeDll -Destination (Join-Path $launcherPublish "cypress_CFB27.dll") -Force

$nodeDeps = Join-Path $buildRoot "node-deps"
New-Item -ItemType Directory -Force -Path $nodeDeps | Out-Null
Copy-Item -LiteralPath (Join-Path $repoRoot "tools\cypress-servers\package.json") -Destination $nodeDeps
Copy-Item -LiteralPath (Join-Path $repoRoot "tools\cypress-servers\package-lock.json") -Destination $nodeDeps
Invoke-Checked "Install production Node dependencies" $npm @("ci", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund") $nodeDeps

if (-not (Test-Path -LiteralPath $DynastyAssetsRoot -PathType Container)) { throw "Dynasty assets root was not found: $DynastyAssetsRoot" }

foreach ($path in @($serverStage, $clientStage)) {
    Copy-Item -LiteralPath (Join-Path $repoRoot "LICENSE") -Destination (Join-Path $path "LICENSE")
    [IO.File]::WriteAllText((Join-Path $path "VERSION.txt"), ($Version + [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))
    Copy-Item -LiteralPath (Join-Path $releaseSource "Release.Common.psm1") -Destination $path
}
foreach ($name in @("Setup-Server.ps1", "Start-Server.ps1", "Stop-Server.ps1", "Test-Server.ps1", "README-SERVER.md")) { Copy-Item -LiteralPath (Join-Path $releaseSource "server\$name") -Destination $serverStage }
New-Item -ItemType Directory -Force -Path (Join-Path $serverStage "bin"), (Join-Path $serverStage "runtime"), (Join-Path $serverStage "tools\cfb27assetexport"), (Join-Path $serverStage "tools\cfb27franchise"), (Join-Path $serverStage "config"), (Join-Path $serverStage "assets\Dynasty_Assets") | Out-Null
Copy-Item -LiteralPath (Join-Path $goBuild "dynasty.exe") -Destination (Join-Path $serverStage "bin")
Copy-Item -LiteralPath (Join-Path $goBuild "cfb27blaze.exe") -Destination (Join-Path $serverStage "bin")
Copy-Item -LiteralPath $node -Destination (Join-Path $serverStage "runtime\node.exe")
$nodeVersion = (& $node --version).Trim().TrimStart('v')
$nodeLicenseUrl = "https://raw.githubusercontent.com/nodejs/node/v$nodeVersion/LICENSE"
Invoke-WebRequest -Uri $nodeLicenseUrl -UseBasicParsing -OutFile (Join-Path $serverStage "runtime\LICENSE")
Copy-Item -LiteralPath (Join-Path $repoRoot "tools\cypress-servers\cmd\cfb27assetexport\main.mjs") -Destination (Join-Path $serverStage "tools\cfb27assetexport")
Copy-Item -LiteralPath (Join-Path $repoRoot "tools\cypress-servers\cmd\cfb27franchise\main.mjs") -Destination (Join-Path $serverStage "tools\cfb27franchise")
Copy-Tree (Join-Path $nodeDeps "node_modules") (Join-Path $serverStage "node_modules")
Copy-Tree $DynastyAssetsRoot (Join-Path $serverStage "assets\Dynasty_Assets")
Copy-Item -LiteralPath (Join-Path $releaseSource "server.example.json") -Destination (Join-Path $serverStage "config\server.example.json")

foreach ($name in @("Setup-Client.ps1", "Start-Client.ps1", "Uninstall-Client.ps1", "README-PLAYER.md")) { Copy-Item -LiteralPath (Join-Path $releaseSource "client\$name") -Destination $clientStage }
New-Item -ItemType Directory -Force -Path (Join-Path $clientStage "payload"), (Join-Path $clientStage "launcher") | Out-Null
Copy-Item -LiteralPath (Join-Path $releaseSource "compatibility.json") -Destination $clientStage
Copy-Item -LiteralPath $bridgeDll -Destination (Join-Path $clientStage "payload\cypress_CFB27.dll")
Copy-Item -LiteralPath (Join-Path $repoRoot "tools\cypress-servers\deploy\cfb27-endpoints.example.json") -Destination (Join-Path $clientStage "payload\cfb27-endpoints.json")
Copy-Tree $launcherPublish (Join-Path $clientStage "launcher")
Get-ChildItem -LiteralPath (Join-Path $clientStage "launcher") -Recurse -File | Where-Object {
    $_.Extension -in @(".pdb", ".lib", ".exp")
} | Remove-Item -Force

New-PackageManifest $serverStage "Cypress-CFB27-Server"
New-PackageManifest $clientStage "Cypress-CFB27-Client"
$serverZip = Join-Path $releaseRoot ("Cypress-CFB27-Server-{0}-win-x64.zip" -f $Version)
$clientZip = Join-Path $releaseRoot ("Cypress-CFB27-Client-{0}-win-x64.zip" -f $Version)
Compress-Archive -Path (Join-Path $serverStage "*") -DestinationPath $serverZip -CompressionLevel Optimal
Compress-Archive -Path (Join-Path $clientStage "*") -DestinationPath $clientZip -CompressionLevel Optimal
$packageEntries = @(@($serverZip, $clientZip) | ForEach-Object {
    [ordered]@{ file = Split-Path -Leaf $_; size = (Get-Item -LiteralPath $_).Length; sha256 = (Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash }
})
$releaseManifest = [ordered]@{ schemaVersion = 1; version = $Version; gitCommit = (& git -C $repoRoot rev-parse HEAD).Trim(); sourceDirty = $sourceDirty; builtAtUtc = [DateTime]::UtcNow.ToString("O"); packages = $packageEntries }
[IO.File]::WriteAllText((Join-Path $releaseRoot "manifest.json"), (($releaseManifest | ConvertTo-Json -Depth 5) + [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))
$checksumLines = @($packageEntries | ForEach-Object { "$($_.sha256)  $($_.file)" })
[IO.File]::WriteAllLines((Join-Path $releaseRoot "SHA256SUMS.txt"), $checksumLines, (New-Object Text.UTF8Encoding($false)))

& $powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $releaseSource "tests\Test-ReleaseLayout.ps1") -ReleaseRoot $releaseRoot
if ($LASTEXITCODE -ne 0) { throw "Release layout verification failed." }
Write-Host "RELEASE_ROOT=$releaseRoot"
Write-Host "SERVER_ZIP=$serverZip"
Write-Host "CLIENT_ZIP=$clientZip"
foreach ($entry in $packageEntries) { Write-Host "$($entry.sha256)  $($entry.file)" }
