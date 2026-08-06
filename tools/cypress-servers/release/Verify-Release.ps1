[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$ReleaseRoot,
    [string]$DynastySeed = "",
    [switch]$SkipLiveServer
)

$ErrorActionPreference = "Stop"
$root = [IO.Path]::GetFullPath($ReleaseRoot)
$releaseSource = $PSScriptRoot
& (Join-Path $releaseSource "tests\Test-ReleaseLayout.ps1") -ReleaseRoot $root
$releaseManifest = Get-Content -LiteralPath (Join-Path $root "manifest.json") -Raw | ConvertFrom-Json
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("cypress-release-verify-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
    $extracted = @{}
    foreach ($package in $releaseManifest.packages) {
        $zip = Join-Path $root ([string]$package.file)
        $actualHash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash
        if (-not [string]::Equals($actualHash, [string]$package.sha256, [StringComparison]::OrdinalIgnoreCase)) { throw "ZIP hash mismatch: $zip" }
        $destination = Join-Path $temporary ([IO.Path]::GetFileNameWithoutExtension([string]$package.file))
        Expand-Archive -LiteralPath $zip -DestinationPath $destination
        if ([string]$package.file -like "*Server*") { $extracted.server = $destination } else { $extracted.client = $destination }
        foreach ($script in Get-ChildItem -LiteralPath $destination -Recurse -Filter *.ps1 -File) {
            $tokens = $null; $errors = $null
            [Management.Automation.Language.Parser]::ParseFile($script.FullName, [ref]$tokens, [ref]$errors) | Out-Null
            if ($errors.Count -ne 0) { throw "PowerShell parse failure in $($script.FullName): $($errors[0].Message)" }
        }
        $manifest = Get-Content -LiteralPath (Join-Path $destination "manifest.json") -Raw | ConvertFrom-Json
        foreach ($entry in $manifest.files) {
            $file = Join-Path $destination ([string]$entry.path -replace '/', '\')
            if ((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash -ne [string]$entry.sha256) { throw "Extracted file hash mismatch: $file" }
        }
    }

    if (-not $SkipLiveServer) {
        if ([string]::IsNullOrWhiteSpace($DynastySeed)) {
            $saveRoot = Join-Path ([Environment]::GetFolderPath("MyDocuments")) "EA SPORTS College Football 27\Saves"
            $DynastySeed = Get-ChildItem -LiteralPath $saveRoot -File -ErrorAction SilentlyContinue | Where-Object {
                $_.Name.StartsWith("DYNASTY", [StringComparison]::OrdinalIgnoreCase)
            } | Sort-Object LastWriteTimeUtc -Descending | Where-Object {
                $stream = $null
                try { $stream = [IO.File]::OpenRead($_.FullName); $header = New-Object byte[] 8; $stream.Read($header, 0, 8) -eq 8 -and [Text.Encoding]::ASCII.GetString($header) -eq "FBCHUNKS" } finally { if ($null -ne $stream) { $stream.Dispose() } }
            } | Select-Object -First 1 -ExpandProperty FullName
        }
        if ([string]::IsNullOrWhiteSpace($DynastySeed)) { throw "No FBCHUNKS Dynasty seed was found for live extracted-server verification. Use -DynastySeed or -SkipLiveServer." }
        function Get-FreePort { $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0); $listener.Start(); try { return ([Net.IPEndPoint]$listener.LocalEndpoint).Port } finally { $listener.Stop() } }
        $dynastyPort = Get-FreePort; $blazePort = Get-FreePort; $diagnosticsPort = Get-FreePort
        & (Join-Path $extracted.server "Setup-Server.ps1") -PackageRoot $extracted.server -VpnBindAddress "127.0.0.1" -VpnRemoteAddress "127.0.0.1" -DynastySeed $DynastySeed -DynastyPort $dynastyPort -BlazePort $blazePort -DiagnosticsPort $diagnosticsPort
        try {
            & (Join-Path $extracted.server "Start-Server.ps1") -PackageRoot $extracted.server
            & (Join-Path $extracted.server "Test-Server.ps1") -PackageRoot $extracted.server -RequireRunning
        } finally {
            & (Join-Path $extracted.server "Stop-Server.ps1") -PackageRoot $extracted.server
        }
    }
    Write-Host "PASS extracted release verification"
} finally {
    Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}
