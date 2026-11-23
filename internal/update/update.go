// Package update finds the newest i-mux release and installs it (ARCHITECTURE
// "Icons, Settings on the rail, updates").
package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/fetch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// Repo is where releases are published.
const Repo = "hamzayogurtcuoglu/i-mux"

// LatestURL redirects to the newest release's tag; tests replace it or set
// IMUX_UPDATE_URL.
var LatestURL = "https://github.com/" + Repo + "/releases/latest"

// BaseURL is where release archives and checksums.txt live; tests replace it
// or set IMUX_UPDATE_URL.
var BaseURL = "https://github.com/" + Repo + "/releases/download"

func latestURL() string {
	if u := os.Getenv("IMUX_UPDATE_URL"); u != "" {
		return strings.TrimRight(u, "/") + "/releases/latest"
	}
	return LatestURL
}

func baseURL() string {
	if u := os.Getenv("IMUX_UPDATE_URL"); u != "" {
		return strings.TrimRight(u, "/") + "/releases/download"
	}
	return BaseURL
}

// Every is how often the watcher asks.
const Every = 24 * time.Hour

var client = &http.Client{
	Timeout:       15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Latest returns the newest release's version ("0.27.0").
func Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, latestURL(), nil)
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

// parseBuild reads a release like parse, and also a development build in
// `git describe` form ("v0.65.0-3-ga514af6", "v0.65.0-dirty"), which
// `make install-local` builds: it gives the build's tag, which the build is at
// or past (D95).
func parseBuild(v string) (n [3]int, ok bool) {
	v = strings.TrimSuffix(strings.TrimSpace(v), "-dirty")
	if f := strings.Split(v, "-"); len(f) == 3 && describeTail(f[1], f[2]) {
		v = f[0]
	}
	return parse(v)
}

// describeTail reports whether n and g are what git describe adds after a
// tag ("-3-ga514af6"): a count of commits and "g" with an abbreviated commit.
func describeTail(n, g string) bool {
	if c, err := strconv.Atoi(n); err != nil || c < 0 || strings.HasPrefix(n, "+") {
		return false
	}
	if len(g) < 5 || g[0] != 'g' {
		return false
	}
	return strings.Trim(g[1:], "0123456789abcdef") == ""
}

// Newer reports whether release a is newer than the running build b: a
// release, or a development build in git describe form, to which every
// release above its tag is newer (D95). Never for an a that is not a release,
// nor for a b that is neither ("dev", a bare commit).
func Newer(a, b string) bool {
	x, ok1 := parse(a)
	y, ok2 := parseBuild(b)
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
// that release's installer on Unix.
func InstallCmd(version, dir string) []string {
	tag := "v" + strings.TrimPrefix(version, "v")
	url := "https://github.com/" + Repo + "/releases/download/" + tag + "/install.sh"
	return []string{"sh", "-c", `curl -fsSL "$1" | sh -s -- --version "$2" --prefix "$3" --no-modify-path --yes`, "sh", url, tag, dir}
}

// Bins returns the executables a release ships in imux's folder: the engine
// first, then imux itself (D83, D85).
func Bins() []string {
	return []string{"imux-core" + sysx.ExeSuffix, "imux" + sysx.ExeSuffix}
}

// ArchiveName is the release archive's file name for version on this system.
func ArchiveName(version string) string {
	num := strings.TrimPrefix(strings.TrimSpace(version), "v")
	return "imux_" + num + "_" + runtime.GOOS + "_" + runtime.GOARCH + archiveExt
}

// InstallArchive downloads the release archive and checksums.txt for version
// from BaseURL, verifies the archive's SHA-256 checksum, unpacks it and
// replaces both binaries in dir as one step (T90.7).
func InstallArchive(ctx context.Context, version, dir string, out io.Writer) error {
	num := strings.TrimPrefix(strings.TrimSpace(version), "v")
	tag := "v" + num
	archive := ArchiveName(num)
	base := strings.TrimRight(baseURL(), "/") + "/" + tag

	tmp, err := os.MkdirTemp("", "imux-update-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	sumsFile := filepath.Join(tmp, "checksums.txt")
	if err := fetch.Download(ctx, base+"/checksums.txt", sumsFile, 0, nil, false); err != nil {
		return err
	}
	want, err := checksumFor(sumsFile, archive)
	if err != nil {
		return err
	}

	archFile := filepath.Join(tmp, archive)
	if err := fetch.Download(ctx, base+"/"+archive, archFile, 0, out, false); err != nil {
		return err
	}
	if out != nil {
		fmt.Fprintln(out, "  "+i18n.T("fetch.checking"))
	}
	if err := fetch.Check(archFile, want); err != nil {
		return err
	}

	unpackDir := filepath.Join(tmp, "unpack")
	unpack := fetch.Untar
	if strings.HasSuffix(archive, ".zip") {
		unpack = fetch.Unzip
	}
	if err := unpack(archFile, unpackDir); err != nil {
		return err
	}
	if err := Replace(dir, unpackDir); err != nil {
		return err
	}
	// The Nerd Font that ships with i-mux (D94), beside it as setup.exe and
	// install.ps1 put it; imux font install takes it from there. Best
	// effort: without it, the font is downloaded when needed.
	_ = copyFonts(filepath.Join(unpackDir, "fonts"), filepath.Join(dir, "fonts"))
	if out != nil {
		for _, b := range Bins() {
			fmt.Fprintln(out, "Installed "+filepath.Join(dir, b))
		}
	}
	return nil
}

// copyFonts copies the files of the release's fonts folder src to dst; a
// release without one copies nothing.
func copyFonts(src, dst string) error {
	ents, err := os.ReadDir(src)
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func checksumFor(sumsFile, archive string) (string, error) {
	b, err := os.ReadFile(sumsFile)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && strings.TrimPrefix(f[1], "*") == archive {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("%s is not listed in checksums.txt", archive)
}

// Replace installs imux-core and imux from srcDir into dir as one step. Each
// existing binary is renamed to <name>.old (so a running executable on
// Windows can be replaced) before the new one is moved into place from
// <name>.new; if any step fails, every binary already moved is put back from
// its .old file. The *.old files are removed on the next start by CleanOld.
func Replace(dir, srcDir string) error {
	bins := Bins()
	for _, b := range bins {
		fi, err := os.Stat(filepath.Join(srcDir, b))
		if err != nil || !fi.Mode().IsRegular() {
			return fmt.Errorf("archive has no %s", b)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	hadOld := make(map[string]bool, len(bins))
	written := make(map[string]bool, len(bins))
	rollback := func() {
		for _, b := range bins {
			dst := filepath.Join(dir, b)
			old := dst + ".old"
			_ = os.Remove(dst + ".new")
			if written[b] {
				_ = os.Remove(dst)
			}
			if hadOld[b] {
				_ = os.Remove(dst)
				_ = os.Rename(old, dst)
			}
		}
	}
	for _, b := range bins {
		src := filepath.Join(srcDir, b)
		dst := filepath.Join(dir, b)
		old := dst + ".old"
		stage := dst + ".new"
		if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
			rollback()
			return fmt.Errorf("%s is a directory", dst)
		}
		if fi, err := os.Stat(old); err == nil && fi.IsDir() {
			rollback()
			return fmt.Errorf("%s is a directory", old)
		}
		if err := copyBin(src, stage); err != nil {
			rollback()
			return err
		}
		if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollback()
			return err
		}
		if _, err := os.Stat(dst); err == nil {
			if err := os.Rename(dst, old); err != nil {
				rollback()
				return err
			}
			hadOld[b] = true
		}
		if err := os.Rename(stage, dst); err != nil {
			rollback()
			return err
		}
		written[b] = true
	}
	return nil
}

func copyBin(src, dst string) error {
	_ = os.Remove(dst)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

// CleanOld removes *.old binaries left in the running imux's folder by an
// earlier update.
func CleanOld() {
	if d, err := Dir(); err == nil {
		CleanOldIn(d)
	}
}

// CleanOldIn removes *.old binaries left in dir by an earlier update. A file
// still held open by a running process on Windows (an i-mux-core server whose
// sessions have not ended yet) cannot be removed and stays until a later
// start.
func CleanOldIn(dir string) {
	for _, b := range Bins() {
		p := filepath.Join(dir, b+".old")
		if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() {
			_ = os.Remove(p)
		}
	}
}
