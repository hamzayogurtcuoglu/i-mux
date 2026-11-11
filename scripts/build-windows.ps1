# Build bin\imux.exe on Windows (the Makefile needs make and a POSIX shell).
#
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1 -Arch arm64 -Output dist\imux.exe
#
# It stamps the version, commit and build date the way 'make build' does
# (git describe, when git is installed) and builds without cgo.
param(
    [ValidateSet('', 'amd64', 'arm64')][string]$Arch = '',
    [string]$Output = ''
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$pkg = 'github.com/hamzayogurtcuoglu/i-mux'
if (-not $Output) { $Output = Join-Path $root 'bin\imux.exe' }

$go = Get-Command go -ErrorAction SilentlyContinue
if ($go) { $go = $go.Source } else { $go = Join-Path $env:ProgramFiles 'Go\bin\go.exe' }
if (-not (Test-Path -LiteralPath $go)) { throw 'Go is not installed: winget install --id GoLang.Go' }

$version = 'dev'
$commit = ''
if (Get-Command git -ErrorAction SilentlyContinue) {
    $d = & git -C $root describe --tags --always --dirty 2>$null
    if ($LASTEXITCODE -eq 0 -and $d) { $version = "$d".Trim() }
    $c = & git -C $root rev-parse --short=12 HEAD 2>$null
    if ($LASTEXITCODE -eq 0 -and $c) { $commit = "$c".Trim() }
}
$date = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
$ldflags = "-s -w -X $pkg/internal/version.Version=$version -X $pkg/internal/version.Commit=$commit -X $pkg/internal/version.Date=$date"

$saved = @{ CGO_ENABLED = $env:CGO_ENABLED; GOOS = $env:GOOS; GOARCH = $env:GOARCH }
Push-Location $root
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    if ($Arch) { $env:GOARCH = $Arch }
    & $go build -trimpath -ldflags $ldflags -o $Output ./cmd/imux
    if ($LASTEXITCODE -ne 0) { throw "go build failed (exit code $LASTEXITCODE)" }
} finally {
    Pop-Location
    foreach ($k in $saved.Keys) {
        if ($null -eq $saved[$k]) { Remove-Item -Path "env:$k" -ErrorAction SilentlyContinue }
        else { Set-Item -Path "env:$k" -Value $saved[$k] }
    }
}
Write-Host "Built $Output ($version)"
