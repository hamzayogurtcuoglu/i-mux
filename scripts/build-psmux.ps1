# Build psmux from source into bin\psmux.exe, next to bin\imux.exe.
#
# psmux 3.4.0 (the current release) stops starting run-shell commands once a
# Ctrl+C or a mouse click was sent into a pane, so every imux key binding and
# status bar click fails from then on; its status lines also have no click
# ranges. Both are fixed on psmux's main branch (issue 819 and the
# status-format work), which has no release yet. Until one comes out, this
# script builds the tested commit. install.ps1 copies bin\psmux.exe next to
# imux.exe, and imux uses the psmux next to it before the one on PATH.
#
# Needs (no administrator rights):
#   winget install --id Rustlang.Rustup -e
#   winget install --id MartinStorsjo.LLVM-MinGW.UCRT -e   (dlltool, imm32)
#
#   .\scripts\build-psmux.ps1 [-Commit <sha>] [-Src <dir>]

param(
    [string]$Commit = '32aa803',
    [string]$Src = (Join-Path $env:TEMP 'imux-psmux-src')
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$target = 'x86_64-pc-windows-gnu'

function Fail([string]$msg) { Write-Host "build-psmux: $msg" -ForegroundColor Red; exit 1 }

$cargoBin = Join-Path $env:USERPROFILE '.cargo\bin'
$env:Path = "$cargoBin;$env:Path"
if (-not (Get-Command rustup -ErrorAction SilentlyContinue)) { Fail 'rustup is missing: winget install --id Rustlang.Rustup -e' }
if (-not (Get-Command git -ErrorAction SilentlyContinue)) { Fail 'git is missing: winget install --id Git.Git -e' }

# The GNU toolchain links without Visual Studio or the Windows SDK.
& rustup toolchain install "stable-$target" --profile minimal | Out-Null
if ($LASTEXITCODE -ne 0) { Fail 'rustup could not install the GNU toolchain' }

# Rust's own MinGW tools cannot make import libraries (no assembler) and lack
# imm32; LLVM-MinGW's dlltool and import library fill the gap. Rust's own
# linker stays in use, so the binary needs only Windows' system DLLs.
$llvm = Get-ChildItem -Path (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Packages') -Filter 'MartinStorsjo.LLVM-MinGW*' -Directory -ErrorAction SilentlyContinue |
    ForEach-Object { Get-ChildItem -Path $_.FullName -Directory } | Select-Object -First 1
if (-not $llvm) { Fail 'LLVM-MinGW is missing: winget install --id MartinStorsjo.LLVM-MinGW.UCRT -e' }
$dlltool = Join-Path $llvm.FullName 'bin\dlltool.exe'
$imm32 = Join-Path $llvm.FullName 'x86_64-w64-mingw32\lib\libimm32.a'
$sysroot = (& rustup run "stable-$target" rustc --print sysroot).Trim()
$linker = Join-Path $sysroot "lib\rustlib\$target\bin\self-contained\x86_64-w64-mingw32-gcc.exe"
foreach ($f in @($dlltool, $imm32, $linker)) { if (-not (Test-Path -LiteralPath $f)) { Fail "not found: $f" } }

if (-not (Test-Path -LiteralPath (Join-Path $Src '.git'))) {
    & git clone --quiet https://github.com/psmux/psmux $Src
    if ($LASTEXITCODE -ne 0) { Fail 'git clone failed' }
}
& git -C $Src fetch --quiet origin
& git -C $Src checkout --quiet $Commit
if ($LASTEXITCODE -ne 0) { Fail "commit $Commit not found" }

$libDir = Join-Path $Src 'target\imux-extra-lib'
New-Item -ItemType Directory -Force -Path $libDir | Out-Null
Copy-Item -LiteralPath $imm32 -Destination $libDir -Force

$env:RUSTFLAGS = "-C dlltool=$dlltool -L native=$libDir"
$env:CARGO_TARGET_X86_64_PC_WINDOWS_GNU_LINKER = $linker
Push-Location $Src
try {
    & cargo "+stable-$target" build --release --target $target --bin psmux
    if ($LASTEXITCODE -ne 0) { Fail "cargo build failed (exit code $LASTEXITCODE)" }
} finally {
    Pop-Location
}

$out = Join-Path $repoRoot 'bin'
New-Item -ItemType Directory -Force -Path $out | Out-Null
$built = Join-Path $Src "target\$target\release\psmux.exe"
$dest = Join-Path $out 'psmux.exe'
if (Test-Path -LiteralPath $dest) {
    Remove-Item -LiteralPath ($dest + '.old') -Force -ErrorAction SilentlyContinue
    Rename-Item -LiteralPath $dest -NewName 'psmux.exe.old'
}
Copy-Item -LiteralPath $built -Destination $dest
Write-Host ("Built " + (& $dest -V | Select-Object -Last 1) + " -> $dest")
