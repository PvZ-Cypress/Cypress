$ErrorActionPreference = "Stop"

$vswhere = Join-Path ${env:ProgramFiles(x86)} "Microsoft Visual Studio\Installer\vswhere.exe"
if (-not (Test-Path -LiteralPath $vswhere)) {
    throw "Visual Studio Build Tools were not found."
}

$visualStudio = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if (-not $visualStudio) {
    throw "The Visual C++ x64 toolchain was not found."
}

$developerShell = Join-Path $visualStudio "Common7\Tools\VsDevCmd.bat"
$testSource = Join-Path $PSScriptRoot "tests\runtime_scan_tests.cpp"
$testOutput = Join-Path $PSScriptRoot "obj\runtime_scan_tests.exe"
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $testOutput) | Out-Null

$command = 'call "{0}" -arch=x64 -host_arch=x64 >nul && cl /nologo /std:c++20 /EHsc /W4 "{1}" /Fe"{2}"' -f `
    $developerShell, $testSource, $testOutput

& $env:ComSpec /d /s /c $command
if ($LASTEXITCODE -ne 0) {
    throw "Bridge test build failed with exit code $LASTEXITCODE"
}

& $testOutput
if ($LASTEXITCODE -ne 0) {
    throw "Bridge tests failed with exit code $LASTEXITCODE"
}

