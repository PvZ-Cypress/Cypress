<#
.SYNOPSIS
Diagnoses why a joining player's CFB27 is not reaching the private host.

.DESCRIPTION
Run this on the JOINING player's PC (not the host). It checks every link in the
chain and prints PASS/FAIL for each, so the failure is identified in one step
instead of by guesswork over chat.

    .\Test-CFB27Client.ps1 -HostAddress 100.110.136.90

Send the host the whole output.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$HostAddress,
    [string]$GameDir = "C:\Program Files\EA Games\EA SPORTS College Football 27",
    [int]$BlazePort = 27920
)

$script:failed = 0
function Report {
    param([string]$Name, [bool]$Ok, [string]$Detail, [string]$Fix = "")
    if ($Ok) {
        Write-Host ("  PASS  {0}" -f $Name) -ForegroundColor Green
    } else {
        $script:failed++
        Write-Host ("  FAIL  {0}" -f $Name) -ForegroundColor Red
    }
    if ($Detail) { Write-Host ("        {0}" -f $Detail) -ForegroundColor DarkGray }
    if (-not $Ok -and $Fix) { Write-Host ("        fix: {0}" -f $Fix) -ForegroundColor Yellow }
}

Write-Host ""
Write-Host "CFB27 client diagnostics -> $HostAddress`:$BlazePort" -ForegroundColor Cyan
Write-Host ""

# 1. Tailscale up and the host visible as a peer.
$tsExe = "C:\Program Files\Tailscale\tailscale.exe"
if (Test-Path -LiteralPath $tsExe) {
    $status = & $tsExe status 2>&1 | Out-String
    Report "Tailscale running" ($LASTEXITCODE -eq 0) ""  "start Tailscale and sign in"
    Report "host is a Tailscale peer" ($status -match [regex]::Escape($HostAddress)) `
        "" "both PCs must be signed into the SAME tailnet"
} else {
    Report "Tailscale installed" $false "not found at $tsExe" "install Tailscale and sign in"
}

# 2. The actual thing that matters: can we open a TCP connection to the server?
$tcp = Test-NetConnection -ComputerName $HostAddress -Port $BlazePort -WarningAction SilentlyContinue
Report "TCP reach ${HostAddress}:$BlazePort" $tcp.TcpTestSucceeded `
    "" "host must run start-cfb27-private-host.ps1 -BindAddress $HostAddress, and allow the port"

# 3. Bridge DLL installed in the game directory.
$dll = Join-Path $GameDir "dinput8.dll"
$haveDll = Test-Path -LiteralPath $dll
$detail = ""
if ($haveDll) {
    $item = Get-Item -LiteralPath $dll
    $hash = (Get-FileHash -LiteralPath $dll -Algorithm MD5).Hash
    $detail = "$($item.Length) bytes, modified $($item.LastWriteTime), MD5 $hash"
}
Report "bridge installed in game dir" $haveDll $detail "run Setup-CFB27Client.ps1 -HostAddress $HostAddress (elevated)"

# Known-bad builds, by MD5. These predate the fixes that make a joining player
# work at all, and they fail in ways that look like a network problem.
$knownBad = @{
    "8A0E4588A145A94040E05EE690FB5E99" =
        "disarms mid-session when the launch lease expires, and sends WinHTTP traffic to this PC's own loopback"
    "F4860B70BF70F5A86498A91BD1CD03BC" =
        "sends WinHTTP traffic to this PC's own loopback instead of the host"
}
if ($haveDll -and $knownBad.ContainsKey($hash)) {
    Report "bridge build is current" $false $knownBad[$hash] `
        "get the refreshed CFB27-Client-Setup folder from the host and re-run Setup-CFB27Client.ps1"
} elseif ($haveDll) {
    Report "bridge build is current" $true "MD5 $hash is not a known-bad build"
}

# 4. Bridge config: correct host, and a lease that has not expired.
$ini = Join-Path $env:APPDATA "Cypress\CFB27\Private\cfb27-bridge.ini"
if (Test-Path -LiteralPath $ini) {
    $text = Get-Content -LiteralPath $ini
    $cfgHost = ($text | Where-Object { $_ -match '^blazeHost=' }) -replace '^blazeHost=', ''
    Report "config points at host" ($cfgHost -eq $HostAddress) `
        "blazeHost=$cfgHost" "re-run Setup-CFB27Client.ps1 -HostAddress $HostAddress"

    $expiresRaw = ($text | Where-Object { $_ -match '^privateLaunchExpiresUtc=' }) -replace '^privateLaunchExpiresUtc=', ''
    $expires = 0
    [void][int64]::TryParse($expiresRaw, [ref]$expires)
    $expiresAt = [DateTimeOffset]::FromUnixTimeSeconds($expires)
    $live = $expiresAt -gt [DateTimeOffset]::UtcNow
    Report "launch lease still valid" $live "expires $($expiresAt.ToLocalTime())" `
        "the lease timed out - re-run Setup-CFB27Client.ps1 before playing"
} else {
    Report "bridge config present" $false "missing $ini" "run Setup-CFB27Client.ps1 -HostAddress $HostAddress"
}

# 5. A stale capture marker silently diverts the client away from the host.
$marker = Join-Path $env:APPDATA "Cypress\CFB27\Private\capture-mode"
Report "no stale capture marker" (-not (Test-Path -LiteralPath $marker)) "" "delete $marker"

# 6. The bridge log is the proof it actually loaded into the game.
$log = Join-Path $env:APPDATA "Cypress\CFB27\Private\cfb27-bridge.log"
if (Test-Path -LiteralPath $log) {
    $item = Get-Item -LiteralPath $log
    Report "bridge has loaded into the game" $true "last write $($item.LastWriteTime)"

    # The decisive check. The log records where the bridge actually sends the
    # game's traffic. An old bridge redirects to this machine's own loopback
    # regardless of blazeHost, which looks identical to "cannot connect" from
    # in-game but is really a stale DLL.
    $redirects = Select-String -LiteralPath $log -Pattern 'redirect #\d+ via connect: .* -> (\S+):(\d+)' -AllMatches
    if ($redirects) {
        $last = $redirects[-1].Matches[0]
        $target = $last.Groups[1].Value
        $onHost = ($target -eq $HostAddress)
        Report "bridge redirects to the host" $onHost "most recent target: ${target}:$($last.Groups[2].Value)" `
            "the bridge is sending traffic to $target instead of the host. Confirm the installed DLL matches the host's current build, then re-run Setup-CFB27Client.ps1"
    } else {
        Report "bridge redirects to the host" $false "no redirects recorded yet" `
            "launch the game and reach the main menu, then re-run this"
    }
    Write-Host ""
    Write-Host "  last 15 lines of cfb27-bridge.log:" -ForegroundColor Cyan
    Get-Content -LiteralPath $log -Tail 15 | ForEach-Object { Write-Host "        $_" -ForegroundColor DarkGray }
} else {
    Report "bridge log exists" $false "missing $log" "launch the game once so the bridge can load, then re-run this"
}

Write-Host ""
if ($script:failed -eq 0) {
    Write-Host "All checks passed - this PC is set up correctly." -ForegroundColor Green
} else {
    Write-Host "$script:failed check(s) failed. Fix the topmost FAIL first." -ForegroundColor Yellow
}
Write-Host ""
