// Package update finds the newest i-mux release and installs it with the
// release's own installer (ARCHITECTURE "Icons, Settings on the rail,
// updates").
package update

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
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
// that release's installer.
func InstallCmd(version, dir string) []string {
	tag := "v" + strings.TrimPrefix(version, "v")
	url := "https://github.com/" + Repo + "/releases/download/" + tag + "/install.sh"
	return []string{"sh", "-c", `curl -fsSL "$1" | sh -s -- --version "$2" --prefix "$3" --no-modify-path --yes`, "sh", url, tag, dir}
}
