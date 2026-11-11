// Package update finds the newest i-mux release and installs it with the
// release's own installer (ARCHITECTURE "Icons, Settings on the rail,
// updates"): install.sh on macOS and Linux, install.ps1 in PowerShell on
// Windows. Windows cannot overwrite a running .exe: install.ps1 renames the
// old imux.exe to imux.exe.old first, and RemoveOld deletes it on a later
// start.
package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is where releases are published.
const Repo = "hamzayogurtcuoglu/i-mux"

// LatestURL redirects to the newest release's tag; tests replace it.
var LatestURL = "https://github.com/" + Repo + "/releases/latest"

// Every is how often the watcher asks.
const Every = 24 * time.Hour

var client = &http.Client{
	Timeout:       15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Latest returns the newest release's version ("0.27.0").
func Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, LatestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "i-mux-update")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", errors.New("GitHub answered " + resp.Status)
	}
	return strings.TrimPrefix(loc[i+len("/tag/"):], "v"), nil
}

// parse reads "1.2.3" (a leading v is fine); ok is false for anything else
// ("dev", a commit).
func parse(v string) (n [3]int, ok bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) != 3 {
		return n, false
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil {
			return n, false
		}
		n[i] = x
	}
	return n, true
}

// Newer reports whether release a is newer than b; never for versions that
// are not releases.
func Newer(a, b string) bool {
	x, ok1 := parse(a)
	y, ok2 := parse(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

// Dir is the folder of the running imux (symlinks resolved).
func Dir() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return filepath.Dir(p), nil
}

// Homebrew reports whether imux in dir was installed by Homebrew.
func Homebrew(dir string) bool { return strings.Contains(dir, "/Cellar/") }

// InstallCmd is the command that installs release version into dir with
// that release's install.sh (macOS, Linux).
func InstallCmd(version, dir string) []string {
	tag := Tag(version)
	url := DownloadBase + "/" + tag + "/install.sh"
	return []string{"sh", "-c", `curl -fsSL "$1" | sh -s -- --version "$2" --prefix "$3" --no-modify-path --yes`, "sh", url, tag, dir}
}

// DownloadBase is where release files are; tests replace it.
var DownloadBase = "https://github.com/" + Repo + "/releases/download"

// Tag is a version as its release tag: "v0.27.0".
func Tag(version string) string { return "v" + strings.TrimPrefix(version, "v") }

// Installer returns the command that installs release version into dir
// and a cleanup to run after it. On Windows the release's install.ps1 is
// downloaded here and run as a file: PowerShell's malware scan (AMSI)
// blocks a one-liner that downloads a script and runs it.
func Installer(ctx context.Context, version, dir string) ([]string, func(), error) {
	if runtime.GOOS != "windows" {
		return InstallCmd(version, dir), func() {}, nil
	}
	f, err := os.CreateTemp("", "imux-install-*.ps1")
	if err != nil {
		return nil, nil, err
	}
	name := f.Name()
	cleanup := func() { _ = os.Remove(name) }
	err = download(ctx, DownloadBase+"/"+Tag(version)+"/install.ps1", f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return WindowsInstallCmd(name, version, dir), cleanup, nil
}

// WindowsInstallCmd runs install.ps1 at script for release version into
// dir: no PATH change (dir is where imux already is), no font and no psmux
// prompts (imux runs, so psmux is there).
func WindowsInstallCmd(script, version, dir string) []string {
	return []string{
		"powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script,
		"-Version", Tag(version), "-InstallDir", dir, "-NoModifyPath", "-NoFont", "-NoPsmux",
	}
}

// download copies url into w, following GitHub's redirects to its storage.
func download(ctx context.Context, url string, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "i-mux-update")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errors.New(url + ": " + resp.Status)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// OldSuffix marks the imux.exe an update on Windows renamed aside.
const OldSuffix = ".old"

// RemoveOld deletes the imux.exe.old files an update on Windows left next
// to the running imux. One that still runs (a sidebar of the old version)
// cannot be deleted yet and stays until a later start. Elsewhere it does
// nothing: there the installer replaces the binary in place.
func RemoveOld() {
	if runtime.GOOS != "windows" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		removeOld(exe)
	}
}

// removeOld deletes exe's renamed copies (exe.old, exe.old.<n>) and
// returns how many went.
func removeOld(exe string) int {
	dir, base := filepath.Split(exe)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	prefix := strings.ToLower(base + OldSuffix)
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if e.IsDir() || !(name == prefix || strings.HasPrefix(name, prefix+".")) {
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			n++
		}
	}
	return n
}
