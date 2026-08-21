param(
    [string]$GameDir = "C:\Program Files\EA Games\EA SPORTS College Football 27",
    [string]$Profile = "LocalPlayer",
    [string]$DynastyAssetsRoot = "C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets",
    [int]$DynastyAssetsSlot = 0,
    [string]$DynastySeed = "",
    [string]$TlsMode = $env:CYPRESS_CFB27_TLS_MODE,
    # Address the Blaze server listens on and that clients are pointed at.
    #
    # "auto" (the default) uses this machine's Tailscale address when there is
    # one, so a second player can join without remembering a flag; forgetting it
    # bound the server to loopback and looked, from the other PC, exactly like a
    # network fault. With no Tailscale interface it falls back to loopback.
    # Pass 127.0.0.1 explicitly to force host-only.
    [string]$BindAddress = "auto",
    [int]$BlazePort = 27920,
    [int]$BlazeDiagnosticsPort = 27921,
    [switch]$NoLaunchGame,
    [switch]$SkipInstallDll,
    # The launcher rebuilds the Go services by default so a stale binary can
    # never silently be tested instead of the current source. Pass -NoBuild to
    # run exactly what is already in build\.
    [switch]$NoBuild,
    # Diagnostic: skip the bridge's certificate-pinning byte patch. Environment
    # variables do not reliably reach the game process, so this rides in the ini.
    [switch]$NoProtoSslPatch,
    # How long the bridge stays armed. Must comfortably exceed a play session.
    [int]$LeaseHours = 12
)

$ErrorActionPreference = "Stop"

function Test-IsAdministrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Invoke-SelfElevated {
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", "`"$PSCommandPath`"",
        "-GameDir", "`"$GameDir`"",
        "-Profile", "`"$Profile`"",
        "-DynastyAssetsRoot", "`"$DynastyAssetsRoot`"",
        "-DynastyAssetsSlot", "$DynastyAssetsSlot",
        "-TlsMode", "`"$TlsMode`"",
        "-BlazePort", "$BlazePort",
        "-BlazeDiagnosticsPort", "$BlazeDiagnosticsPort"
    )
    if (-not [string]::IsNullOrWhiteSpace($DynastySeed)) { $args += @("-DynastySeed", "`"$DynastySeed`"") }
    if ($NoLaunchGame) { $args += "-NoLaunchGame" }
    if ($SkipInstallDll) { $args += "-SkipInstallDll" }
    Start-Process -FilePath powershell.exe -ArgumentList ($args -join " ") -Verb RunAs
}

if (-not $SkipInstallDll -and -not (Test-IsAdministrator)) {
    Invoke-SelfElevated
    return
}

$scriptRoot = Split-Path -Parent $PSCommandPath
$servicesDir = Resolve-Path (Join-Path $scriptRoot "..")
$packageRoot = Resolve-Path (Join-Path $scriptRoot "..\..\..")
$buildDir = Join-Path $servicesDir "build"
$launcherDir = Join-Path $packageRoot "Launcher"
$privateRoot = Join-Path $env:APPDATA "Cypress\CFB27\Private"
$dataDir = Join-Path $privateRoot "data"
$runDir = Join-Path $privateRoot ("runs\cli_" + (Get-Date -Format "yyyyMMdd_HHmmss"))
$bridgeConfigPath = Join-Path $privateRoot "cfb27-bridge.ini"
$logFile = Join-Path $runDir "private-start.log"
$runId = Split-Path -Leaf $runDir

New-Item -ItemType Directory -Force -Path $dataDir, $runDir | Out-Null

function Write-Log {
    param([string]$Message)
    $line = "{0:O} {1}" -f (Get-Date), $Message
    Write-Host $line
    Add-Content -LiteralPath $logFile -Value $line
}

function Resolve-FirstExistingPath {
    param([string[]]$Candidates, [string]$Label)
    foreach ($candidate in $Candidates) {
        if (Test-Path -LiteralPath $candidate) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    throw "$Label was not found. Checked: $($Candidates -join '; ')"
}

function Find-CFB27DynastySeed {
    $savesDirectory = Join-Path ([Environment]::GetFolderPath("MyDocuments")) "EA SPORTS College Football 27\Saves"
    if (-not (Test-Path -LiteralPath $savesDirectory -PathType Container)) {
        throw "CFB27 saves directory was not found: $savesDirectory"
    }
    foreach ($file in Get-ChildItem -LiteralPath $savesDirectory -File |
        Where-Object { $_.Name.StartsWith("DYNASTY", [StringComparison]::OrdinalIgnoreCase) } |
        Sort-Object LastWriteTimeUtc -Descending) {
        $stream = $null
        try {
            $stream = [IO.File]::OpenRead($file.FullName)
            $header = New-Object byte[] 8
            if ($stream.Read($header, 0, $header.Length) -eq $header.Length -and
                [Text.Encoding]::ASCII.GetString($header) -eq "FBCHUNKS") {
                return $file.FullName
            }
        } finally {
            if ($null -ne $stream) { $stream.Dispose() }
        }
    }
    throw "No DYNASTY* FBCHUNKS offline save was found in $savesDirectory"
}

function Wait-HttpHealthy {
    param(
        [string]$Name,
        [string]$Url,
        [Diagnostics.Process]$Process,
        [int]$TimeoutSeconds,
        [string]$ExpectedRunId = ""
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
                if (-not [string]::IsNullOrWhiteSpace($ExpectedRunId)) {
                    $health = $response.Content | ConvertFrom-Json
                    if ($health.runId -ne $ExpectedRunId) {
                        Write-Log "$Name health at $Url had stale runId=$($health.runId); expected=$ExpectedRunId"
                        Start-Sleep -Milliseconds 500
                        continue
                    }
                }
                Write-Log "$Name healthy at $Url"
                return
            }
        } catch { }

        if ($Process.HasExited) {
            $stdout = Get-Content -LiteralPath (Join-Path $runDir "$Name.stdout.log") -Raw -ErrorAction SilentlyContinue
            $stderr = Get-Content -LiteralPath (Join-Path $runDir "$Name.stderr.log") -Raw -ErrorAction SilentlyContinue
            throw "$Name exited early with code $($Process.ExitCode). stderr=$stderr stdout=$stdout"
        }
        Start-Sleep -Milliseconds 500
    }
    throw "$Name did not become healthy at $Url within $TimeoutSeconds seconds."
}

function Start-LoggedProcess {
    param(
        [string]$Name,
        [string]$Exe,
        [string[]]$Arguments,
        [string]$WorkingDirectory
    )
    Write-Log "$Name exe=$Exe"
    Write-Log "$Name args=$($Arguments -join ' ')"
    $quotedArguments = @($Arguments | ForEach-Object {
        if ($_ -match '[\s"]') {
            '"' + ($_ -replace '"', '\"') + '"'
        } else {
            $_
        }
    })
    $process = Start-Process `
        -FilePath $Exe `
        -ArgumentList $quotedArguments `
        -WorkingDirectory $WorkingDirectory `
        -RedirectStandardOutput (Join-Path $runDir "$Name.stdout.log") `
        -RedirectStandardError (Join-Path $runDir "$Name.stderr.log") `
        -WindowStyle Hidden `
        -PassThru
    Write-Log "$Name pid=$($process.Id)"
    return $process
}

Write-Log "CFB27 private host start"
Write-Log "packageRoot=$packageRoot"
Write-Log "servicesDir=$servicesDir"
Write-Log "runDir=$runDir"
Write-Log "gameDir=$GameDir"
Write-Log "profile=$Profile"
Write-Log "dynastyAssetsRoot=$DynastyAssetsRoot"
Write-Log "dynastyAssetsSlot=$DynastyAssetsSlot"
if ([string]::IsNullOrWhiteSpace($TlsMode)) {
    $TlsMode = "tls13"
}
Write-Log "tlsMode=$TlsMode"
if ($BindAddress -eq "auto") {
    $tailscaleIP = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.InterfaceAlias -like "Tailscale*" -and $_.IPAddress -notmatch '^(127\.|169\.254\.)' } |
        Select-Object -First 1 -ExpandProperty IPAddress
    if ($tailscaleIP) {
        $BindAddress = $tailscaleIP
        Write-Log "bindAddress=auto -> $BindAddress (Tailscale)"
    } else {
        $BindAddress = "127.0.0.1"
        Write-Log "bindAddress=auto -> 127.0.0.1 (no Tailscale interface found)"
    }
}
Write-Log "blazePort=$BlazePort"
Write-Log "blazeDiagnosticsPort=$BlazeDiagnosticsPort"

if ($BindAddress -eq "127.0.0.1") {
    # Loopback-only binding is correct for solo play but silently makes the
    # server unreachable for a second player: from their side it is
    # indistinguishable from a network fault. Say so plainly, and name any
    # address they could actually be pointed at.
    Write-Host ""
    Write-Host "LOCAL ONLY - no other player can join this server." -ForegroundColor Yellow
    Write-Host "  Listening on 127.0.0.1 only. To host for someone else, restart with:" -ForegroundColor Yellow
    $candidates = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -notmatch '^(127\.|169\.254\.)' } |
        Sort-Object { $_.InterfaceAlias -notlike "Tailscale*" }
    foreach ($c in ($candidates | Select-Object -First 3)) {
        Write-Host ("    -BindAddress {0}    ({1})" -f $c.IPAddress, $c.InterfaceAlias) -ForegroundColor Yellow
    }
    Write-Host ""
}

if ($BindAddress -ne "127.0.0.1") {
    Write-Log "hosting for remote players on ${BindAddress}:$BlazePort"
    # A second player cannot reach the server unless the port is open. Scope the
    # rule to the private profile so it is not exposed on public networks.
    $ruleName = "Cypress CFB27 Blaze $BlazePort"
    if (-not (Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue)) {
        try {
            New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Action Allow `
                -Protocol TCP -LocalPort $BlazePort -Profile Private | Out-Null
            Write-Log "added firewall rule: $ruleName"
        } catch {
            Write-Log "WARNING could not add firewall rule (needs elevation): $_"
        }
    }
}

# Rebuild before launching. Go's build cache makes this near-instant when
# nothing changed, and it removes the failure mode where a code fix is tested
# against yesterday's binary and wrongly judged not to work.
if (-not $NoBuild) {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if (-not $go) {
        Write-Log "WARNING go not on PATH; using existing binaries in $buildDir"
    } else {
        New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
        # go build must run inside the module; the launcher is usually invoked
        # from an unrelated directory (e.g. C:\Windows\system32).
        Push-Location -LiteralPath $servicesDir
        try {
            foreach ($target in @("cfb27blaze", "dynasty")) {
                $out = Join-Path $buildDir "$target.exe"
                Write-Log "building $target.exe"
                & $go.Source build -o $out "./cmd/$target"
                if ($LASTEXITCODE -ne 0) {
                    throw "go build failed for $target (exit $LASTEXITCODE). Fix the build or pass -NoBuild."
                }
            }
        } finally {
            Pop-Location
        }
        Write-Log "build up to date"
    }
}

$dynastyExe = Resolve-FirstExistingPath @(
    (Join-Path $buildDir "dynasty.exe"),
    (Join-Path $servicesDir "dynasty.exe")
) "dynasty.exe"

$blazeExe = Resolve-FirstExistingPath @(
    (Join-Path $buildDir "cfb27blaze.exe"),
    (Join-Path $servicesDir "cfb27blaze.exe")
) "cfb27blaze.exe"

$assetSlotRoot = Resolve-FirstExistingPath @(
    (Join-Path $DynastyAssetsRoot ([string]$DynastyAssetsSlot)),
    (Join-Path $scriptRoot "Dynasty_Files"),
    (Join-Path ([Environment]::GetFolderPath("DesktopDirectory")) "CFB27\Dynasty_Files"),
    (Join-Path ([Environment]::GetFolderPath("DesktopDirectory")) "CFB27\Release\Dynasty_Assets\2")
) "Dynasty_Files"
$schemaRoot = $assetSlotRoot

$coachCatalogPath = Join-Path $dataDir ("cfb27-coaches-slot{0}.json" -f $DynastyAssetsSlot)
$assetExporter = Join-Path $servicesDir "cmd\cfb27assetexport\main.mjs"
$maddenFranchisePackage = Join-Path $servicesDir "node_modules\madden-franchise\package.json"
if (-not (Test-Path -LiteralPath $maddenFranchisePackage)) {
    $npm = Resolve-FirstExistingPath @(
        (Join-Path (Split-Path -Parent (Get-Command node.exe).Source) "npm.cmd"),
        "C:\Program Files\nodejs\npm.cmd"
    ) "npm.cmd"
    Write-Log "installing local CFB27 asset decoder dependencies"
    $npmProc = Start-Process -FilePath $npm -ArgumentList @("install", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund") -WorkingDirectory $servicesDir -Wait -PassThru -WindowStyle Hidden
    if ($npmProc.ExitCode -ne 0) {
        throw "npm dependency install failed with exit code $($npmProc.ExitCode)"
    }
}
$node = (Get-Command node.exe -ErrorAction Stop).Source
$franchiseTool = Resolve-FirstExistingPath @(
    (Join-Path $servicesDir "cmd\cfb27franchise\main.mjs")
) "CFB27 franchise mutator"
if ([string]::IsNullOrWhiteSpace($DynastySeed)) {
    $DynastySeed = Find-CFB27DynastySeed
} else {
    $DynastySeed = Resolve-FirstExistingPath @($DynastySeed) "CFB27 Dynasty seed"
}
Write-Log "dynastySeed=$DynastySeed"
Write-Log "exporting authoritative coach catalog=$coachCatalogPath"
$exportProc = Start-Process -FilePath $node -ArgumentList @(
    $assetExporter,
    "--asset-root", $DynastyAssetsRoot,
    "--slot", ([string]$DynastyAssetsSlot),
    "--output", $coachCatalogPath
) -WorkingDirectory $servicesDir -Wait -PassThru -WindowStyle Hidden
if ($exportProc.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $coachCatalogPath)) {
    throw "CFB27 coach catalog export failed with exit code $($exportProc.ExitCode)"
}

# build-bridge.ps1 writes to the services directory, so that copy is preferred:
# picking the Launcher copy first meant a rebuilt bridge was silently ignored and
# the previous DLL was installed instead.
$dllPath = Resolve-FirstExistingPath @(
    (Join-Path $servicesDir "cypress_CFB27.dll"),
    (Join-Path $launcherDir "cypress_CFB27.dll"),
    (Join-Path $packageRoot "Server\build\Release\cypress_CFB27.dll")
) "cypress_CFB27.dll"
Write-Log "bridge dll=$dllPath ($((Get-Item -LiteralPath $dllPath).LastWriteTime))"

$endpointsPath = Resolve-FirstExistingPath @(
    (Join-Path $packageRoot "cfb27-endpoints.json"),
    (Join-Path $scriptRoot "cfb27-endpoints.example.json")
) "cfb27-endpoints.json"

Stop-Process -Name dynasty, cfb27blaze -Force -ErrorAction SilentlyContinue

$dynastyProc = Start-LoggedProcess "dynasty" $dynastyExe @(
    "-bind", "127.0.0.1",
    "-port", "27910",
    "-schema-root", $schemaRoot,
    "-db", (Join-Path $dataDir "cfb27_dynasty.db"),
    "-seed", $DynastySeed,
    "-data-dir", (Join-Path $dataDir "dynasties"),
    "-node", $node,
    "-franchise-tool", $franchiseTool
) $runDir
Wait-HttpHealthy "dynasty" "http://127.0.0.1:27910/health" $dynastyProc 300

$blazeProc = Start-LoggedProcess "cfb27blaze" $blazeExe @(
    "-bind", $BindAddress,
    "-port", ([string]$BlazePort),
    "-diagnostics-bind", "127.0.0.1",
    "-diagnostics-port", ([string]$BlazeDiagnosticsPort),
    "-dynasty-url", "http://127.0.0.1:27910",
    "-coach-catalog", $coachCatalogPath,
    "-profile", $Profile,
    "-tls-mode", $TlsMode,
    "-run-id", $runId,
    "-log-file", (Join-Path $runDir "cfb27-blaze.jsonl")
) $runDir
Wait-HttpHealthy "cfb27blaze" "http://127.0.0.1:$BlazeDiagnosticsPort/health" $blazeProc 30 $runId

if (-not $SkipInstallDll) {
    if (-not (Test-Path -LiteralPath $GameDir)) {
        throw "Game directory not found: $GameDir"
    }
    Stop-Process -Name CollegeFB27, CollegeFB27_Trial, EAAntiCheat.GameServiceLauncher -Force -ErrorAction SilentlyContinue
    Copy-Item -LiteralPath $dllPath -Destination (Join-Path $GameDir "dinput8.dll") -Force
    Copy-Item -LiteralPath $endpointsPath -Destination (Join-Path $GameDir "cfb27-endpoints.json") -Force
    Write-Log "installed dinput8.dll and cfb27-endpoints.json"
}

$bridgeEndpointsFile = Join-Path $GameDir "cfb27-endpoints.json"
if ($SkipInstallDll -and -not (Test-Path -LiteralPath $bridgeEndpointsFile)) {
    $bridgeEndpointsFile = $endpointsPath
}

# The lease must outlast a play session, not just the launch. At 10 minutes it
# expired mid-game; combined with the bridge re-reading it, that dropped a live
# session with "lost your connection to the EA Servers".
$privateLaunchExpiresUtc = [DateTimeOffset]::UtcNow.AddHours($LeaseHours).ToUnixTimeSeconds()
Write-Log "launch lease valid until $([DateTimeOffset]::FromUnixTimeSeconds($privateLaunchExpiresUtc).ToLocalTime())"

# Name the host from its own Cypress launcher sign-in, the same way joining
# players name themselves. Guessing names produces someone else's; this reads the
# account actually signed in on this machine.
$launcherData = Join-Path $env:APPDATA "Cypress\launcherdata.json"
if (Test-Path -LiteralPath $launcherData) {
    try {
        $data = Get-Content -LiteralPath $launcherData -Raw | ConvertFrom-Json
        $hostPlayerName = ""
        if ($data.Identity -and $data.Identity.Username) { $hostPlayerName = [string]$data.Identity.Username }
        elseif ($data.Username) { $hostPlayerName = [string]$data.Username }
        if (-not [string]::IsNullOrWhiteSpace($hostPlayerName)) {
            # Register through the bind address, not loopback: the roster is keyed
            # on the address a player connects from, and the host's game connects
            # from this address rather than 127.0.0.1.
            $registerUrl = "http://${BindAddress}:$BlazePort/cypress/register?name=$([uri]::EscapeDataString($hostPlayerName))"
            try {
                Invoke-RestMethod -Uri $registerUrl -TimeoutSec 10 -ErrorAction Stop | Out-Null
                Write-Log "host registered as '$hostPlayerName'"
            } catch {
                Write-Log "WARNING could not register host name: $_"
            }
        }
    } catch {
        Write-Log "WARNING could not read $launcherData"
    }
}
$bridgeConfig = @(
    "# Written by start-cfb27-private-host.ps1",
    "blazeHost=$BindAddress",
    "blazePort=$BlazePort",
    "profile=$Profile",
    "runDirectory=$runDir",
    "endpointsFile=$bridgeEndpointsFile",
    "privateLaunchExpiresUtc=$privateLaunchExpiresUtc",
    "enableBearSslBypass=true",
    "noProtoSslPatch=$(if ($NoProtoSslPatch) { '1' } else { '0' })",
    "dumpRuntimeCodeBytes=false",
    "enableCandidateEndpointRedirects=false",
    "enableProtoSslVerifyProbe=false",
    "enableCertVerifyHook=false",
    "certVerifyForce=false",
    "enableFailStateWatch=false"
)
Set-Content -LiteralPath $bridgeConfigPath -Value $bridgeConfig -Encoding ASCII
Write-Log "wrote persistent bridge config=$bridgeConfigPath"

if ($NoLaunchGame) {
    Write-Log "services ready; game launch skipped"
    Write-Host ""
    Write-Host "Services are ready. Run folder:"
    Write-Host $runDir
    return
}

$gameExe = Resolve-FirstExistingPath @(
    (Join-Path $GameDir "CollegeFB27.exe"),
    (Join-Path $GameDir "CollegeFB27_Trial.exe")
) "CFB27 executable"

$env:CYPRESS_EMBEDDED = "1"
$env:CYPRESS_CFB27_DISCOVERY = "1"
$env:CYPRESS_CFB27_PRIVATE_ONLINE_DYNASTY = "1"
$env:CYPRESS_CFB27_BLAZE_HOST = $BindAddress
$env:CYPRESS_CFB27_BLAZE_PORT = [string]$BlazePort
$env:CYPRESS_CFB27_PROFILE = $Profile
$env:CYPRESS_CFB27_RUN_DIR = $runDir
$env:CYPRESS_CFB27_BRIDGE_CONFIG = $bridgeConfigPath
$env:CYPRESS_CFB27_ENDPOINTS_FILE = (Join-Path $GameDir "cfb27-endpoints.json")
$env:CYPRESS_CFB27_DYNASTY_URL = "http://127.0.0.1:27910"
$env:CYPRESS_CFB27_DYNASTY_PROFILE = $Profile

$gameArgs = @(
    "-playerName", $Profile,
    "-console",
    "-allowMultipleInstances",
    "-Game.Platform", "GamePlatform_Win32",
    "-name", "Cypress CFB27 Private"
)
Write-Log "launching game exe=$gameExe"
Write-Log "game args=$($gameArgs -join ' ')"
$gameProc = Start-Process -FilePath $gameExe -ArgumentList $gameArgs -WorkingDirectory $GameDir -PassThru
Write-Log "game launch pid=$($gameProc.Id)"

Write-Host ""
Write-Host "CFB27 private host launched. Run folder:"
Write-Host $runDir
