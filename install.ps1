# Install i-mux natively on Windows: https://github.com/hamzayogurtcuoglu/i-mux
#
#   irm https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.ps1 | iex
#   .\install.ps1 -FromSource .        (from a checkout, with Go installed)
#
# On Windows i-mux runs on i-mux-core, its built-in terminal multiplexer
# engine (D85). This script downloads the release zip for this CPU, checks its
# SHA-256 against checksums.txt, installs imux-core.exe and imux.exe into
# %LOCALAPPDATA%\Programs\i-mux as one step (rolling back if either fails),
# puts that folder on your user PATH, and offers the Nerd Font the sidebar's
# icons need (imux font install: for your user only, with Windows Terminal and
# VS Code set to it). With -FromSource (or when no release exists yet and a
# checkout sits next to this script), both binaries are built from source with
# Go instead.
#
# Options: parameters when run as a file; when piped into iex, set the
# IMUX_* environment variables first.
#   -Version vX.Y.Z    install this release instead of the latest (IMUX_VERSION)
#   -InstallDir DIR    install directory (IMUX_INSTALL_DIR; default %LOCALAPPDATA%\Programs\i-mux)
#   -FromSource DIR    build this i-mux checkout with Go instead of downloading (IMUX_FROM_SOURCE)
#   -Yes               install the Nerd Font without asking (IMUX_YES=1)
#   -NoModifyPath      do not add the install directory to your user PATH
#   -NoFont            do not offer the Nerd Font (IMUX_NO_FONT=1)
# IMUX_INSTALL_BASE_URL replaces https://github.com/<repo>/releases/download
# (a mirror or a test fixture; checksums.txt still decides).
#
# Keep this file ASCII (Windows PowerShell reads a file without a BOM in the
# ANSI code page) and free of backticks, PowerShell's escape character:
# Windows' malware scan (AMSI) blocks a script with them when a program runs
# it. internal/update's tests check both.
param(
    [string]$Version = $env:IMUX_VERSION,
    [string]$InstallDir = $env:IMUX_INSTALL_DIR,
    [string]$FromSource = $env:IMUX_FROM_SOURCE,
    [switch]$Yes,
    [switch]$NoModifyPath,
    [switch]$NoFont
)

$Repo = 'hamzayogurtcuoglu/i-mux'
$Module = 'github.com/hamzayogurtcuoglu/i-mux'
$Bins = @('imux-core.exe', 'imux.exe')

function Install-Imux {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    if ($env:OS -ne 'Windows_NT') { Fail 'this script is for Windows; on macOS and Linux use install.sh' }
    $assumeYes = $Yes.IsPresent -or $env:IMUX_YES -eq '1'
    $offerFont = -not ($NoFont.IsPresent -or $env:IMUX_NO_FONT -eq '1')
    $dir = $InstallDir
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'Programs\i-mux' }
    $arch = Get-Arch

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('imux-install-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        if ($FromSource) {
            $srcDir = Build-FromSource $FromSource $tmp
        } else {
            $srcDir = Get-Release $arch $tmp
            if (-not $srcDir) {
                if ($PSScriptRoot -and (Test-Checkout $PSScriptRoot)) {
                    Say "No release of $Repo yet: building the checkout in $PSScriptRoot."
                    $srcDir = Build-FromSource $PSScriptRoot $tmp
                } else {
                    Fail "no release of $Repo to download yet. Clone it and run: .\install.ps1 -FromSource <checkout> (needs Go: winget install --id GoLang.Go)"
                }
            }
        }
        $exe = Install-Binaries $srcDir $dir
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    $onPath = Add-ToPath $dir (-not $NoModifyPath.IsPresent)

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

function Confirm-Step([string]$question, [bool]$assumeYes) {
    if ($assumeYes) { return $true }
    if ([Console]::IsInputRedirected -or -not [Environment]::UserInteractive) { return $false }
    $answer = Read-Host "$question [y/N]"
    return $answer -match '^(y|yes)$'
}

function Get-Arch {
    $a = $env:PROCESSOR_ARCHITEW6432
    if (-not $a) { $a = $env:PROCESSOR_ARCHITECTURE }
    switch ($a) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
    }
    Fail "unsupported CPU: $a (imux is built for amd64 and arm64)"
}

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

# Get-Release downloads and verifies the release's zip and returns the folder
# containing imux.exe and imux-core.exe; $null when there is no release.
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
    foreach ($b in $Bins) {
        if (-not (Test-Path -LiteralPath (Join-Path $out $b) -PathType Leaf)) {
            Fail "$zip has no $b"
        }
    }
    return $out
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

function Build-FromSource([string]$src, [string]$tmp) {
    $src = (Resolve-Path -LiteralPath $src).Path
    if (-not (Test-Checkout $src)) { Fail "$src is not an i-mux checkout (no go.mod for $Module)" }
    $go = Find-Go
    if (-not $go) { Fail 'building from source needs Go: winget install --id GoLang.Go, then open a new terminal' }
    $ver = 'dev'
    $commit = ''
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $d = & git -C $src describe --tags --always --dirty 2>$null
        if ($LASTEXITCODE -eq 0 -and $d) { $ver = "$d".Trim() }
        $c = & git -C $src rev-parse --short=12 HEAD 2>$null
        if ($LASTEXITCODE -eq 0 -and $c) { $commit = "$c".Trim() }
    }
    $date = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $ldflags = "-s -w -X $Module/internal/version.Version=$ver -X $Module/internal/version.Commit=$commit -X $Module/internal/version.Date=$date"
    $out = Join-Path $tmp 'build'
    New-Item -ItemType Directory -Path $out -Force | Out-Null
    Say "Building imux $ver and imux-core from $src..."
    $cgo, $goos = $env:CGO_ENABLED, $env:GOOS
    Push-Location -LiteralPath $src
    try {
        $env:CGO_ENABLED = '0'
        $env:GOOS = 'windows'
        & $go build -trimpath -ldflags $ldflags -o (Join-Path $out 'imux.exe') ./cmd/imux
        if ($LASTEXITCODE -ne 0) { Fail "go build imux failed (exit code $LASTEXITCODE)" }
        & $go build -trimpath -ldflags '-s -w' -o (Join-Path $out 'imux-core.exe') ./cmd/imux-core
        if ($LASTEXITCODE -ne 0) { Fail "go build imux-core failed (exit code $LASTEXITCODE)" }
    } finally {
        Pop-Location
        $env:CGO_ENABLED, $env:GOOS = $cgo, $goos
    }
    return $out
}

# Install-Binaries puts imux-core.exe and imux.exe from $srcDir into $dir as
# one step, renaming any running executable to *.old and rolling back if
# either fails.
function Install-Binaries([string]$srcDir, [string]$dir) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $hadOld = @{}
    $written = @{}
    try {
        foreach ($b in $Bins) {
            $src = Join-Path $srcDir $b
            $dst = Join-Path $dir $b
            $old = "$dst.old"
            $stage = "$dst.new"
            if (Test-Path -LiteralPath $dst -PathType Container) { throw "$dst is a directory" }
            if (Test-Path -LiteralPath $old -PathType Container) { throw "$old is a directory" }
            Copy-Item -LiteralPath $src -Destination $stage -Force
            if (Test-Path -LiteralPath $old) {
                Remove-Item -LiteralPath $old -Force
            }
            if (Test-Path -LiteralPath $dst) {
                Move-Item -LiteralPath $dst -Destination $old -Force
                $hadOld[$b] = $true
            }
            Move-Item -LiteralPath $stage -Destination $dst -Force
            $written[$b] = $true
        }
    } catch {
        foreach ($b in $Bins) {
            $dst = Join-Path $dir $b
            $old = "$dst.old"
            Remove-Item -LiteralPath "$dst.new" -Force -ErrorAction SilentlyContinue
            if ($written[$b]) { Remove-Item -LiteralPath $dst -Force -ErrorAction SilentlyContinue }
            if ($hadOld[$b]) {
                Remove-Item -LiteralPath $dst -Force -ErrorAction SilentlyContinue
                Move-Item -LiteralPath $old -Destination $dst -Force -ErrorAction SilentlyContinue
            }
        }
        Fail "cannot install to $dir ($($_.Exception.Message)): nothing was installed"
    }
    foreach ($b in $Bins) {
        $dst = Join-Path $dir $b
        Remove-Item -LiteralPath "$dst.old" -Force -ErrorAction SilentlyContinue
        Say "Installed $dst"
    }
    return (Join-Path $dir 'imux.exe')
}

function Add-ToPath([string]$dir, [bool]$modify) {
    $k = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
        $user = ''
        if ($null -ne $k) {
            $user = [string]$k.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        } else {
            $user = [Environment]::GetEnvironmentVariable('Path', 'User')
        }
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
        $newPath = (($parts + $dir) -join ';')
        if ($null -ne $k) {
            $k.SetValue('Path', $newPath, [Microsoft.Win32.RegistryValueKind]::ExpandString)
        } else {
            [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        }
        $env:Path = "$env:Path;$dir"
        Say "Added $dir to your user PATH"
        return $true
    } finally {
        if ($null -ne $k) { $k.Close() }
    }
}

Install-Imux
