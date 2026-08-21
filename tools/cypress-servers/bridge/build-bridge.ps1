param(
    [string]$Output = "$PSScriptRoot\..\cypress_CFB27.dll"
)

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
$outputPath = [IO.Path]::GetFullPath($Output)
$outputDirectory = Split-Path -Parent $outputPath
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
$intermediate = Join-Path $PSScriptRoot "obj"
New-Item -ItemType Directory -Force -Path $intermediate | Out-Null

$source = Join-Path $PSScriptRoot "bridge.cpp"
$definition = Join-Path $PSScriptRoot "dinput8.def"
$object = Join-Path $intermediate "bridge.obj"
$pdb = [IO.Path]::ChangeExtension($outputPath, ".pdb")
$command = 'call "{0}" -arch=x64 -host_arch=x64 >nul && cl /nologo /std:c++20 /EHsc /O2 /W4 /DUNICODE /D_UNICODE /c "{1}" /Fo"{2}" && link /nologo /DLL /DEF:"{3}" /OUT:"{4}" /PDB:"{5}" "{2}" shell32.lib ws2_32.lib psapi.lib' -f `
    $developerShell, $source, $object, $definition, $outputPath, $pdb

& $env:ComSpec /d /s /c $command
if ($LASTEXITCODE -ne 0) {
    throw "Bridge build failed with exit code $LASTEXITCODE"
}
Write-Host "built $outputPath"
