param(
    [string]$Output = "$PSScriptRoot\..\build\capturehook.dll"
)

$ErrorActionPreference = "Stop"
$vc = "C:\Program Files\Microsoft Visual Studio\18\Community\VC\Auxiliary\Build\vcvars64.bat"
if (-not (Test-Path -LiteralPath $vc)) { throw "vcvars64.bat not found at $vc" }

$root = $PSScriptRoot
$obj = Join-Path $root "obj"
New-Item -ItemType Directory -Force -Path $obj, (Split-Path -Parent ([IO.Path]::GetFullPath($Output))) | Out-Null
$out = [IO.Path]::GetFullPath($Output)

# MinHook is C; the hook DLL is C++. Compile both, link together.
$minhookSources = @(
    "minhook\src\hook.c",
    "minhook\src\buffer.c",
    "minhook\src\trampoline.c",
    "minhook\src\hde\hde64.c",
    "minhook\src\hde\hde32.c"
) | ForEach-Object { '"' + (Join-Path $root $_) + '"' }

$cCompile = 'cl /nologo /c /O2 /MT /W3 /I"' + (Join-Path $root "minhook\include") + '" /Fo"' + $obj + '\\" ' + ($minhookSources -join ' ')
$cppCompile = 'cl /nologo /c /std:c++20 /EHsc /O2 /MT /W4 /DUNICODE /D_UNICODE /I"' + (Join-Path $root "minhook\include") + '" /Fo"' + $obj + '\capturehook.obj" "' + (Join-Path $root "capturehook.cpp") + '"'
$objs = @("capturehook.obj", "hook.obj", "buffer.obj", "trampoline.obj", "hde64.obj", "hde32.obj") |
    ForEach-Object { '"' + (Join-Path $obj $_) + '"' }
$link = 'link /nologo /DLL /OUT:"' + $out + '" ' + ($objs -join ' ') + ' user32.lib'

$command = 'call "' + $vc + '" >nul && ' + $cCompile + ' && ' + $cppCompile + ' && ' + $link
& $env:ComSpec /d /s /c $command
if ($LASTEXITCODE -ne 0) { throw "capturehook build failed with exit code $LASTEXITCODE" }
Write-Host "built $out"
