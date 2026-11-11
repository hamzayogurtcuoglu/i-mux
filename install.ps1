# Install imux natively on Windows: https://github.com/hamzayogurtcuoglu/i-mux
#
#   irm https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.ps1 | iex
#   .\install.ps1 -FromSource .        (from a checkout, with Go installed)
#
# On Windows i-mux runs on psmux, a native Windows multiplexer that speaks
# tmux's commands (no WSL). This script installs psmux with winget if it is
# missing, puts imux.exe in %LOCALAPPDATA%\Programs\imux and that folder on
# your user PATH, then offers the Nerd Font the sidebar's icons need
# (imux font install: for your user only, with Windows Terminal and VS Code
# set to it). imux.exe is the release's zip for this CPU, its SHA-256 checked
# against checksums.txt; without a release (or with -FromSource) it is built
# from source with Go.
#
# Options: parameters when run as a file; when piped into iex, set the
# IMUX_* environment variables first.
#   -Version vX.Y.Z    install this release instead of the latest (IMUX_VERSION)
#   -InstallDir DIR    install directory (IMUX_INSTALL_DIR; default %LOCALAPPDATA%\Programs\imux)
#   -FromSource DIR    build this i-mux checkout with Go instead of downloading (IMUX_FROM_SOURCE)
#   -Yes               install psmux and the Nerd Font without asking (IMUX_YES=1)
#   -NoModifyPath      do not add the install directory to your user PATH
#   -NoFont            do not offer the Nerd Font (IMUX_NO_FONT=1)
#   -NoPsmux           do not install psmux
# IMUX_INSTALL_BASE_URL replaces https://github.com/<repo>/releases/download
# (a mirror; checksums.txt still decides).
#
# Keep this file ASCII (Windows PowerShell reads a file without a BOM in the
# ANSI code page) and free of backticks, PowerShell's escape character:
# Windows' malware scan (AMSI) blocks a script with them when a program, such
# as imux update, runs it. internal/update's tests check both.
param(
    [string]$Version = $env:IMUX_VERSION,
    [string]$InstallDir = $env:IMUX_INSTALL_DIR,
    [string]$FromSource = $env:IMUX_FROM_SOURCE,
    [switch]$Yes,
    [switch]$NoModifyPath,
    [switch]$NoFont,
    [switch]$NoPsmux
)

# The GitHub repository whose releases are downloaded. A fork changes this.
$Repo = 'hamzayogurtcuoglu/i-mux'

$PsmuxId = 'marlocarlo.psmux'
$Module = 'github.com/hamzayogurtcuoglu/i-mux'

function Install-Imux {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is slow with its progress bar
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    if ($env:OS -ne 'Windows_NT') { Fail 'this script is for Windows; on macOS and Linux use install.sh' }
    $assumeYes = $Yes.IsPresent -or $env:IMUX_YES -eq '1'
    $offerFont = -not ($NoFont.IsPresent -or $env:IMUX_NO_FONT -eq '1')
    $dir = $InstallDir
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'Programs\imux' }
    $arch = Get-Arch

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('imux-install-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        if ($FromSource) {
            $new = Build-FromSource $FromSource $tmp
        } else {
            $new = Get-Release $arch $tmp
            if (-not $new) {
                # No release of this repository yet: a checkout next to this
                # script builds instead.
                if ($PSScriptRoot -and (Test-Checkout $PSScriptRoot)) {
                    Say "No release of $Repo yet: building the checkout in $PSScriptRoot."
                    $new = Build-FromSource $PSScriptRoot $tmp
                } else {
                    Fail "no release of $Repo to download yet. Clone it and run: .\install.ps1 -FromSource <checkout> (needs Go: winget install --id GoLang.Go)"
                }
            }
        }
        $exe = Install-Binary $new $dir
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    $onPath = Add-ToPath $dir (-not $NoModifyPath.IsPresent)
    if (-not $NoPsmux.IsPresent) { Install-Psmux $assumeYes }

    # The sidebar's icons need a Nerd Font where the terminal draws text.
    if ($offerFont) {
        & $exe font --check *> $null
        if ($LASTEXITCODE -ne 0) {
            Say ''
            Say "The sidebar shows the editor's icons when your terminal has a Nerd Font."
            if (Confirm-Step 'Install JetBrainsMono Nerd Font (7 MB, for you only) and set Windows Terminal and VS Code to it?' $assumeYes) {
                & $exe font install --yes
                if ($LASTEXITCODE -ne 0) { Say 'The font was not installed. Try again later with: imux font install' }
            } else {
                Say 'Skipped. Later: imux font install'
            }
        }
    }

    Say ''
    if ($onPath) {
        Say "Done. Open a new terminal (Windows Terminal is recommended), then run 'imux' in a project folder."
        Say "(In this terminal you can run it now as: $exe; 'imux doctor' checks the setup.)"
    } else {
        Say "Done. Run '$exe' in a project folder ('imux doctor' checks the setup)."
    }
}

function Say([string]$msg) { Write-Host $msg }

function Fail([string]$msg) {
    Write-Host "imux install: $msg" -ForegroundColor Red
    throw "imux install failed: $msg"
}

# Confirm-Step: yes with -Yes; no when there is nobody to ask.
function Confirm-Step([string]$question, [bool]$assumeYes) {
    if ($assumeYes) { return $true }
    if ([Console]::IsInputRedirected -or -not [Environment]::UserInteractive) { return $false }
    $answer = Read-Host "$question [y/N]"
    return $answer -match '^(y|yes)$'
}

function Get-Arch {
    $a = $env:PROCESSOR_ARCHITEW6432 # a 32-bit PowerShell on 64-bit Windows
    if (-not $a) { $a = $env:PROCESSOR_ARCHITECTURE }
    switch ($a) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
    }
    Fail "unsupported CPU: $a (imux is built for amd64 and arm64)"
}

# Get-LatestTag is the newest release's tag ('' when there is none): the
# redirect of /releases/latest, which has no rate limit, else the API.
function Get-LatestTag {
    try {
        $req = [Net.HttpWebRequest]::Create("https://github.com/$Repo/releases/latest")
        $req.Method = 'HEAD'
        $req.AllowAutoRedirect = $false
        $req.UserAgent = 'imux-install'
        $resp = $req.GetResponse()
        $loc = $resp.Headers['Location']
        $resp.Close()
        if ($loc -match '/releases/tag/([^/?#]+)$') { return $Matches[1] }
    } catch { }
    try {
        $r = Invoke-RestMethod -UseBasicParsing -Headers @{ 'User-Agent' = 'imux-install' } -Uri "https://api.github.com/repos/$Repo/releases/latest"
        if ($r.tag_name) { return [string]$r.tag_name }
    } catch { }
    return ''
}

# Get-Release downloads and checks the release's zip and returns the path of
# its imux.exe; $null when the repository has no release.
function Get-Release([string]$arch, [string]$tmp) {
    $tag = $Version
    if (-not $tag) { $tag = Get-LatestTag }
    if (-not $tag) { return $null }
    if ($tag -notmatch '^v?\d+\.\d+\.\d+[\w.\-]*$') { Fail "invalid version: $tag" }
    if (-not $tag.StartsWith('v')) { $tag = "v$tag" }
    $num = $tag.Substring(1)
    $zip = "imux_${num}_windows_${arch}.zip"
    $base = $env:IMUX_INSTALL_BASE_URL
    if (-not $base) { $base = "https://github.com/$Repo/releases/download" }
    $base = $base.TrimEnd('/') + "/$tag"

    Say "Downloading imux $tag for windows/$arch..."
    try {
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$zip" -OutFile (Join-Path $tmp $zip)
        Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
    } catch {
        if ($Version) { Fail "download failed: $base/$zip ($($_.Exception.Message))" }
        Say "Release $tag has no Windows build ($zip)."
        return $null
    }
    $want = ''
    foreach ($line in Get-Content -LiteralPath (Join-Path $tmp 'checksums.txt')) {
        $f = $line -split '\s+'
        if ($f.Count -ge 2 -and ($f[1] -eq $zip -or $f[1] -eq "*$zip")) { $want = $f[0].ToLower() }
    }
    if (-not $want) { Fail "$zip is not listed in checksums.txt" }
    $got = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $tmp $zip)).Hash.ToLower()
    if ($want -ne $got) { Fail "checksum mismatch for $zip (expected $want, got $got)" }
    Say 'Checksum verified.'

    $out = Join-Path $tmp 'zip'
    Expand-Archive -LiteralPath (Join-Path $tmp $zip) -DestinationPath $out -Force
    $exe = Get-ChildItem -LiteralPath $out -Recurse -Filter 'imux.exe' | Select-Object -First 1
    if (-not $exe) { Fail "$zip holds no imux.exe" }
    return $exe.FullName
}

function Test-Checkout([string]$path) {
    $mod = Join-Path $path 'go.mod'
    return (Test-Path -LiteralPath $mod) -and ((Get-Content -LiteralPath $mod -TotalCount 1) -match [regex]::Escape("module $Module"))
}

function Find-Go {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if ($go) { return $go.Source }
    $p = Join-Path $env:ProgramFiles 'Go\bin\go.exe'
    if (Test-Path -LiteralPath $p) { return $p }
    return ''
}

# Build-FromSource builds the checkout at $src into $tmp and returns the path
# of imux.exe; the version is git describe's, as 'make build' stamps it.
function Build-FromSource([string]$src, [string]$tmp) {
    $src = (Resolve-Path -LiteralPath $src).Path
    if (-not (Test-Checkout $src)) { Fail "$src is not an i-mux checkout (no go.mod for $Module)" }
    $go = Find-Go
    if (-not $go) { Fail 'building from source needs Go: winget install --id GoLang.Go, then open a new terminal' }
    $version = 'dev'
    $commit = ''
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $d = & git -C $src describe --tags --always --dirty 2>$null
        if ($LASTEXITCODE -eq 0 -and $d) { $version = "$d".Trim() }
        $c = & git -C $src rev-parse --short=12 HEAD 2>$null
        if ($LASTEXITCODE -eq 0 -and $c) { $commit = "$c".Trim() }
    }
    $date = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $ldflags = "-s -w -X $Module/internal/version.Version=$version -X $Module/internal/version.Commit=$commit -X $Module/internal/version.Date=$date"
    $exe = Join-Path $tmp 'imux.exe'
    Say "Building imux $version from $src..."
    $cgo, $goos = $env:CGO_ENABLED, $env:GOOS
    Push-Location -LiteralPath $src
    try {
        $env:CGO_ENABLED = '0'
        $env:GOOS = 'windows'
        & $go build -trimpath -ldflags $ldflags -o $exe ./cmd/imux
        if ($LASTEXITCODE -ne 0) { Fail "go build failed (exit code $LASTEXITCODE)" }
    } finally {
        Pop-Location
        $env:CGO_ENABLED, $env:GOOS = $cgo, $goos
    }
    return $exe
}

# Install-Binary puts $new into $dir as imux.exe. Windows cannot overwrite a
# running .exe (an open sidebar, or the imux running 'imux update'), but it
# can rename one: the old imux.exe becomes imux.exe.old, which imux deletes
# on a later start, once nothing runs it.
function Install-Binary([string]$new, [string]$dir) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $target = Join-Path $dir 'imux.exe'
    $old = ''
    if (Test-Path -LiteralPath $target) {
        $old = "$target.old"
        if (Test-Path -LiteralPath $old) { Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue }
        if (Test-Path -LiteralPath $old) { $old = "$target.old." + [guid]::NewGuid().ToString('N').Substring(0, 8) }
        try {
            Move-Item -LiteralPath $target -Destination $old -Force
        } catch {
            Fail "cannot replace $target ($($_.Exception.Message)); close i-mux and try again"
        }
    }
    try {
        Copy-Item -LiteralPath $new -Destination $target -Force
    } catch {
        if ($old) { Move-Item -LiteralPath $old -Destination $target -Force -ErrorAction SilentlyContinue }
        Fail "cannot install to $dir ($($_.Exception.Message)); use -InstallDir"
    }
    $v = & $target version 2>$null
    Say "Installed $target ($v)"
    return $target
}

# Add-ToPath puts $dir on the user PATH for new terminals (and this one);
# it reports whether 'imux' works by name in new terminals.
function Add-ToPath([string]$dir, [bool]$modify) {
    $user = [Environment]::GetEnvironmentVariable('Path', 'User')
    $parts = @()
    if ($user) { $parts = @($user -split ';' | Where-Object { $_ }) }
    $has = $parts | Where-Object { $_.TrimEnd('\') -ieq $dir.TrimEnd('\') }
    if ($has) {
        Say "$dir is already on your PATH"
        return $true
    }
    if (-not $modify) {
        Say ''
        Say "$dir is not on your PATH. Add it in Settings > System > About > Advanced system settings > Environment Variables."
        return $false
    }
    [Environment]::SetEnvironmentVariable('Path', (($parts + $dir) -join ';'), 'User')
    $env:Path = "$env:Path;$dir"
    Say "Added $dir to your user PATH"
    return $true
}

# Install-Psmux: imux's multiplexer on Windows, from winget.
function Install-Psmux([bool]$assumeYes) {
    $found = Get-Command psmux -ErrorAction SilentlyContinue
    if (-not $found) {
        $found = Get-ChildItem -Path (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Packages') -Filter "$PsmuxId*" -Directory -ErrorAction SilentlyContinue |
            ForEach-Object { Join-Path $_.FullName 'psmux.exe' } | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    }
    if ($found) {
        $p = if ($found -is [string]) { $found } else { $found.Source }
        $v = (& $p -V 2>$null | Select-Object -Last 1)
        Say "psmux found: $v"
        return
    }
    Say ''
    Say 'imux needs psmux (a native tmux for Windows), which is not installed.'
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        Say 'winget is not available: install psmux from https://github.com/marlocarlo/psmux, then run: imux doctor'
        return
    }
    Say "Installing it runs: winget install --id $PsmuxId -e"
    if (-not (Confirm-Step 'Install psmux now?' $assumeYes)) {
        Say "Skipped. Later: winget install --id $PsmuxId -e"
        return
    }
    $wargs = @('install', '--id', $PsmuxId, '-e')
    if ($assumeYes) { $wargs += @('--accept-package-agreements', '--accept-source-agreements') }
    & winget @wargs
    if ($LASTEXITCODE -ne 0) { Say "psmux was not installed (winget exit code $LASTEXITCODE). Try: winget install --id $PsmuxId -e" }
}

Install-Imux
