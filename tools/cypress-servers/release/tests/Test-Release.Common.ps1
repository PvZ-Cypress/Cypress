$ErrorActionPreference = "Stop"

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw "ASSERT TRUE FAILED: $Message" }
}

function Assert-False {
    param([bool]$Condition, [string]$Message)
    if ($Condition) { throw "ASSERT FALSE FAILED: $Message" }
}

function Assert-Equal {
    param($Expected, $Actual, [string]$Message)
    if ($Expected -ne $Actual) {
        throw "ASSERT EQUAL FAILED: $Message expected=[$Expected] actual=[$Actual]"
    }
}

$releaseRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
Import-Module (Join-Path $releaseRoot "Release.Common.psm1") -Force

$fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ("cypress-release-common-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $fixtureRoot | Out-Null
try {
    $validSeed = Join-Path $fixtureRoot "DYNASTY-valid"
    $invalidSeed = Join-Path $fixtureRoot "DYNASTY-invalid"
    [IO.File]::WriteAllBytes($validSeed, [Text.Encoding]::ASCII.GetBytes("FBCHUNKSfixture"))
    [IO.File]::WriteAllBytes($invalidSeed, [Text.Encoding]::ASCII.GetBytes("NOTCHUNKfixture"))
    Assert-True (Test-CFBChunksFile -Path $validSeed) "FBCHUNKS seed should be accepted"
    Assert-False (Test-CFBChunksFile -Path $invalidSeed) "invalid seed should be rejected"
    Assert-False (Test-CFBChunksFile -Path (Join-Path $fixtureRoot "missing")) "missing seed should be rejected"

    $gameDir = Join-Path $fixtureRoot "game"
    New-Item -ItemType Directory -Path $gameDir | Out-Null
    $gameExe = Join-Path $gameDir "CollegeFB27.exe"
    [IO.File]::WriteAllBytes($gameExe, [Text.Encoding]::ASCII.GetBytes("known-game-build"))
    $info = Get-CFB27ExecutableInfo -GameDirectory $gameDir
    $expectedHash = (Get-FileHash -LiteralPath $gameExe -Algorithm SHA256).Hash.ToUpperInvariant()
    Assert-Equal "CollegeFB27.exe" $info.fileName "game executable name"
    Assert-Equal $expectedHash $info.sha256 "game executable hash"
    Assert-Equal 16 $info.size "game executable size"

    $manifest = [pscustomobject]@{
        schemaVersion = 1
        builds = @([pscustomobject]@{
            fileName = "CollegeFB27.exe"
            sha256 = $expectedHash
            size = 16
            fileVersion = $null
            bridgeProfile = "fixture"
        })
    }
    $match = Test-CFB27Compatibility -ExecutableInfo $info -Manifest $manifest
    Assert-True $match.supported "known build should be supported"
    Assert-Equal "fixture" $match.build.bridgeProfile "matching profile"

    $unknown = [pscustomobject]@{
        fileName = $info.fileName
        fullPath = $info.fullPath
        sha256 = ("0" * 64)
        size = $info.size
        fileVersion = $info.fileVersion
    }
    Assert-False (Test-CFB27Compatibility -ExecutableInfo $unknown -Manifest $manifest).supported "unknown hash should be rejected"

    $validJson = Join-Path $fixtureRoot "valid.json"
    $invalidJson = Join-Path $fixtureRoot "invalid.json"
    [IO.File]::WriteAllText($validJson, '{"value":27}')
    [IO.File]::WriteAllText($invalidJson, '{not-json}')
    Assert-Equal 27 (Read-CypressJson -Path $validJson).value "valid JSON value"
    $invalidFailed = $false
    try { Read-CypressJson -Path $invalidJson | Out-Null } catch { $invalidFailed = $true }
    Assert-True $invalidFailed "malformed JSON should fail"

    $quoted = ConvertTo-CypressArgumentString -Arguments @("plain", "C:\Program Files\Cypress", 'say"hello', "")
    Assert-Equal 'plain "C:\Program Files\Cypress" "say\"hello" ""' $quoted "Windows command-line quoting"

    Write-Host "PASS Test-Release.Common"
} finally {
    Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
}
