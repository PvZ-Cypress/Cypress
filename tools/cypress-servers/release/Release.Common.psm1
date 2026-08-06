Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Assert-CypressFile {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Label
    )
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "$Label was not found: $Path"
    }
    return (Resolve-Path -LiteralPath $Path).Path
}

function Read-CypressJson {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Path)
    $resolved = Assert-CypressFile -Path $Path -Label "JSON file"
    try {
        return Get-Content -LiteralPath $resolved -Raw -Encoding UTF8 | ConvertFrom-Json
    } catch {
        throw "Invalid JSON file '$resolved': $($_.Exception.Message)"
    }
}

function Test-CFBChunksFile {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $false }
    $stream = $null
    try {
        $stream = [IO.File]::OpenRead($Path)
        if ($stream.Length -lt 8) { return $false }
        $header = New-Object byte[] 8
        if ($stream.Read($header, 0, $header.Length) -ne $header.Length) { return $false }
        return [Text.Encoding]::ASCII.GetString($header) -eq "FBCHUNKS"
    } catch {
        return $false
    } finally {
        if ($null -ne $stream) { $stream.Dispose() }
    }
}

function Get-CFB27ExecutableInfo {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$GameDirectory)
    if (-not (Test-Path -LiteralPath $GameDirectory -PathType Container)) {
        throw "CFB27 game directory was not found: $GameDirectory"
    }
    $selected = $null
    foreach ($name in @("CollegeFB27.exe", "CollegeFB27_Trial.exe")) {
        $candidate = Join-Path $GameDirectory $name
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            $selected = (Resolve-Path -LiteralPath $candidate).Path
            break
        }
    }
    if ([string]::IsNullOrWhiteSpace($selected)) {
        throw "CollegeFB27.exe or CollegeFB27_Trial.exe was not found in: $GameDirectory"
    }
    $item = Get-Item -LiteralPath $selected
    return [pscustomobject]@{
        fileName = $item.Name
        fullPath = $item.FullName
        sha256 = (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToUpperInvariant()
        size = [int64]$item.Length
        fileVersion = $item.VersionInfo.FileVersion
    }
}

function Test-CFB27Compatibility {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]$ExecutableInfo,
        [Parameter(Mandatory = $true)]$Manifest
    )
    if ($null -eq $Manifest.builds) { throw "Compatibility manifest has no builds array." }
    $match = @($Manifest.builds | Where-Object {
        [string]::Equals([string]$_.fileName, [string]$ExecutableInfo.fileName, [StringComparison]::OrdinalIgnoreCase) -and
        [string]::Equals([string]$_.sha256, [string]$ExecutableInfo.sha256, [StringComparison]::OrdinalIgnoreCase)
    } | Select-Object -First 1)
    return [pscustomobject]@{
        supported = $match.Count -eq 1
        build = if ($match.Count -eq 1) { $match[0] } else { $null }
    }
}

function ConvertTo-CypressArgumentString {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][AllowEmptyCollection()][AllowEmptyString()][string[]]$Arguments)
    $quoted = foreach ($argument in $Arguments) {
        if ($argument.Length -gt 0 -and $argument -notmatch '[\s"]') {
            $argument
            continue
        }
        $builder = New-Object Text.StringBuilder
        [void]$builder.Append('"')
        $backslashes = 0
        foreach ($character in $argument.ToCharArray()) {
            if ($character -eq '\') {
                $backslashes++
                continue
            }
            if ($character -eq '"') {
                [void]$builder.Append(('\' * (($backslashes * 2) + 1)))
                [void]$builder.Append('"')
            } else {
                if ($backslashes -gt 0) { [void]$builder.Append(('\' * $backslashes)) }
                [void]$builder.Append($character)
            }
            $backslashes = 0
        }
        if ($backslashes -gt 0) { [void]$builder.Append(('\' * ($backslashes * 2))) }
        [void]$builder.Append('"')
        $builder.ToString()
    }
    return ($quoted -join ' ')
}

Export-ModuleMember -Function @(
    "Assert-CypressFile",
    "Read-CypressJson",
    "Test-CFBChunksFile",
    "Get-CFB27ExecutableInfo",
    "Test-CFB27Compatibility",
    "ConvertTo-CypressArgumentString"
)
