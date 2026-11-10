// Package webview is the web preview (D44): the terminal browser i-mux
// installs when asked (Carbonyl), the URLs it opens and the command its pane
// runs. The tmux side and the popups are in cli and installview.
package webview

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// Version is the Carbonyl release i-mux installs.
const Version = "0.0.3"

// Sizes the messages give, in MB.
const (
	DownloadMB = 74
	DiskMB     = 170
)

// Asset is one of the release's zips: its name, size in bytes and SHA-256.
type Asset struct {
	Name string
	Size int64
	Sum  string
}

// assets by GOOS/GOARCH; the sums were taken from the files GitHub serves
// and are checked on every download.
var assets = map[string]Asset{
	"linux/amd64":  {"carbonyl.linux-amd64.zip", 78049320, "46a902ea29bb32f773fb4ee341423705b92345a810422b8df93eaea6a1ec7ad2"},
	"linux/arm64":  {"carbonyl.linux-arm64.zip", 80183490, "5b75c99138cdabe454935e2c60994adce0bd4571f16e4eecfdf867c59a1197be"},
	"darwin/amd64": {"carbonyl.macos-amd64.zip", 77040274, "29717506254364de1988a070cd3877c7de31c9de91c0878535ac71b33d7029a3"},
	"darwin/arm64": {"carbonyl.macos-arm64.zip", 68738483, "8cf9197264d8ea94a8ff9d00eebf35d8d094c7f84b8530f1bd41f9eaaed98064"},
}

// AssetFor is the zip for goos and goarch.
func AssetFor(goos, goarch string) (Asset, bool) {
	a, ok := assets[goos+"/"+goarch]
	return a, ok
}

// URL is where a is downloaded from; IMUX_WEB_URL replaces it (a mirror;
// tests).
func URL(a Asset) string {
	if u := os.Getenv("IMUX_WEB_URL"); u != "" {
		return u
	}
	return "https://github.com/fathyb/carbonyl/releases/download/v" + Version + "/" + a.Name
}

// Dir is the unpacked Carbonyl.
func Dir() string { return filepath.Join(paths.Tools(), "carbonyl-"+Version) }

// Bin is its program.
func Bin() string { return filepath.Join(Dir(), "carbonyl") }

// Browser is the command the preview runs a URL with: [web] browser's words
// (its program found on the PATH), else i-mux's Carbonyl; nil when it is
// not installed.
func Browser(cfg *config.Config) []string {
	if f := strings.Fields(cfg.Web.Browser); len(f) > 0 {
		p, err := exec.LookPath(f[0])
		if err != nil {
			return nil
		}
		return append([]string{p}, f[1:]...)
	}
	if fi, err := os.Stat(Bin()); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
		return []string{Bin()}
	}
	return nil
}

// Installed reports whether the preview has its browser.
func Installed(cfg *config.Config) bool { return Browser(cfg) != nil }

// Command is the pane's command for u. Carbonyl's Chromium will not run as
// root without --no-sandbox.
func Command(cfg *config.Config, u string, root bool) []string {
	b := Browser(cfg)
	if b == nil {
		return nil
	}
	if cfg.Web.Browser == "" && root {
		b = append(b, "--no-sandbox")
	}
	return append(b, u)
}

// IsHTML reports a web page file: a click on it in the explorer opens it in
// the preview.
func IsHTML(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm", ".xhtml":
		return true
	}
	return false
}

// Previewable reports a file the preview shows: a web page or an SVG image.
func Previewable(name string) bool {
	return IsHTML(name) || strings.EqualFold(filepath.Ext(name), ".svg")
}

var portOnly = regexp.MustCompile(`^:?[0-9]{1,5}([/?#].*)?$`)

// Target turns what the user gave into a URL: an existing file (absolute,
// ~/…, or relative to dir) a file:// URL; a port ("3000", ":3000/api")
// http://localhost:3000; text without a scheme http:// (localhost and IP
// addresses) or https://. Blank stays "".
func Target(s, dir string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "://") {
		return s
	}
	for _, scheme := range []string{"about:", "data:", "view-source:"} {
		if strings.HasPrefix(s, scheme) {
			return s
		}
	}
	p := s
	if strings.HasPrefix(p, "~/") {
		p = filepath.Join(paths.UserHome(), p[2:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	if _, err := os.Stat(p); err == nil {
		return FileURL(p)
	}
	if portOnly.MatchString(s) {
		return "http://localhost:" + strings.TrimPrefix(s, ":")
	}
	host := s
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil {
		return "http://" + s
	}
	return "https://" + s
}

// FileURL is the file:// URL of absolute path p.
func FileURL(p string) string { return (&url.URL{Scheme: "file", Path: p}).String() }

// FilePath is the path of a file:// URL ("" for other URLs).
func FilePath(u string) string {
	pu, err := url.Parse(u)
	if err != nil || pu.Scheme != "file" {
		return ""
	}
	return pu.Path
}

// Label names a page's pane: a file's name, else its host and path
// ("localhost:3000", "go.dev/doc").
func Label(u string) string {
	pu, err := url.Parse(u)
	switch {
	case err != nil:
		return u
	case pu.Scheme == "file":
		return filepath.Base(pu.Path)
	}
	if l := pu.Host + strings.TrimSuffix(pu.Path, "/"); l != "" {
		return l
	}
	return u
}

// Errors of Install.
var (
	ErrChecksum    = errors.New("the download does not match its checksum")
	ErrUnsupported = errors.New("no Carbonyl build for this system")
)

// sum is the checksum Install wants for a (tests replace it).
var sum = func(a Asset) string { return a.Sum }

// Installer downloads and unpacks Carbonyl.
type Installer struct {
	Out          io.Writer // progress and steps
	Bar          bool      // redraw the progress on one line (a terminal)
	GOOS, GOARCH string    // "" = this machine's
}

// Install puts Carbonyl into Dir: the release's zip, checked, unpacked into
// a temporary folder that is renamed when complete.
func (in *Installer) Install(ctx context.Context) error {
	goos, goarch := in.GOOS, in.GOARCH
	if goos == "" {
		goos, goarch = runtime.GOOS, runtime.GOARCH
	}
	a, ok := AssetFor(goos, goarch)
	if !ok {
		return fmt.Errorf("%w (%s/%s)", ErrUnsupported, goos, goarch)
	}
	if err := paths.EnsureDir(paths.Tools()); err != nil {
		return err
	}
	part := filepath.Join(paths.Tools(), a.Name+".part")
	defer os.Remove(part)
	if err := in.fetch(ctx, URL(a), part, a.Size); err != nil {
		return err
	}
	in.say(i18n.T("web.checking"))
	if err := check(part, sum(a)); err != nil {
		return err
	}
	in.say(i18n.T("web.unpacking"))
	tmp, err := os.MkdirTemp(paths.Tools(), ".carbonyl-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := Unzip(part, tmp); err != nil {
		return err
	}
	src := filepath.Join(tmp, "carbonyl-"+Version)
	if fi, err := os.Stat(filepath.Join(src, "carbonyl")); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%s has no carbonyl-%s/carbonyl", a.Name, Version)
	}
	if err := os.RemoveAll(Dir()); err != nil {
		return err
	}
	return os.Rename(src, Dir())
}

func (in *Installer) say(s string) {
	if in.Out != nil {
		fmt.Fprintln(in.Out, "  "+s)
	}
}

// Remove deletes Carbonyl and what a broken install left.
func Remove() error {
	if err := os.RemoveAll(Dir()); err != nil {
		return err
	}
	left, _ := filepath.Glob(filepath.Join(paths.Tools(), ".carbonyl-*"))
	parts, _ := filepath.Glob(filepath.Join(paths.Tools(), "carbonyl.*.part"))
	for _, p := range append(left, parts...) {
		_ = os.RemoveAll(p)
	}
	return nil
}

func check(file, want string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return ErrChecksum
	}
	return nil
}

func (in *Installer) fetch(ctx context.Context, u, file string, size int64) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", u, resp.Status)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = size
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	p := &progress{out: in.Out, bar: in.Bar, total: total}
	if in.Out != nil && !in.Bar {
		fmt.Fprintln(in.Out, "  "+i18n.T("web.downloading")+" "+u)
	}
	_, err = io.Copy(io.MultiWriter(f, p), resp.Body)
	p.end()
	if err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// progress draws a download's bar: ━━━━━─────  42%  31.2 of 74.4 MB.
type progress struct {
	out      io.Writer
	bar      bool
	total, n int64
	last     time.Time
}

func (p *progress) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if now := time.Now(); p.bar && p.out != nil && now.Sub(p.last) >= 100*time.Millisecond {
		p.last = now
		p.draw()
	}
	return len(b), nil
}

func (p *progress) end() {
	if p.bar && p.out != nil {
		p.draw()
		fmt.Fprintln(p.out)
	}
}

func (p *progress) draw() {
	pct := 0
	if p.total > 0 {
		pct = int(min(p.n*100/p.total, 100))
	}
	const width = 30
	done := pct * width / 100
	mb := func(n int64) float64 { return float64(n) / (1 << 20) }
	fmt.Fprintf(p.out, "\r  %s \x1b[36m%s\x1b[38;5;238m%s\x1b[0m %3d%%  %s\x1b[K", i18n.T("web.downloading"),
		strings.Repeat("━", done), strings.Repeat("─", width-done), pct, i18n.T("web.progress", mb(p.n), mb(p.total)))
}

// Unzip unpacks zip file into dir: folders and regular files, with their
// modes. Any other entry, or a path that would leave dir, is an error.
func Unzip(file, dir string) error {
	r, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		name := filepath.FromSlash(strings.TrimSuffix(f.Name, "/"))
		if !filepath.IsLocal(name) {
			return fmt.Errorf("unsafe path in %s: %q", filepath.Base(file), f.Name)
		}
		dst := filepath.Join(dir, name)
		switch m := f.Mode(); {
		case m.IsDir():
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case m.IsRegular():
			if err := extract(f, dst, m.Perm()|0o600); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected entry in %s: %q", filepath.Base(file), f.Name)
		}
	}
	return nil
}

func extract(f *zip.File, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, rc); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// CheckVersion runs the browser with --version ("Carbonyl 0.0.3").
func CheckVersion(ctx context.Context, argv []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, argv[0], append(argv[1:len(argv):len(argv)], "--version")...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Missing lists the shared libraries bin needs that this machine lacks
// (ldd's "not found"; Linux only).
func Missing(ctx context.Context, bin string) []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "ldd", bin).Output()
	return ParseLdd(string(out))
}

// ParseLdd lists the libraries ldd's output did not find.
func ParseLdd(out string) []string {
	var libs []string
	for _, l := range strings.Split(out, "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(l), " => ")
		if ok && strings.HasPrefix(strings.TrimSpace(rest), "not found") {
			libs = append(libs, name)
		}
	}
	return libs
}

// debPackages hold the libraries Carbonyl needs on Debian and Ubuntu.
var debPackages = map[string]string{
	"libnss3.so":     "libnss3",
	"libnssutil3.so": "libnss3",
	"libsmime3.so":   "libnss3",
	"libnspr4.so":    "libnspr4",
	"libplc4.so":     "libnspr4",
	"libplds4.so":    "libnspr4",
	"libexpat.so.1":  "libexpat1",
	"libasound.so.2": "libasound2",
}

// Packages are the Debian and Ubuntu packages with libs (unknown ones are
// left out); t64 names ALSA's as Ubuntu 24.04 and Debian 13 do.
func Packages(libs []string, t64 bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range libs {
		p, ok := debPackages[l]
		if !ok {
			continue
		}
		if p == "libasound2" && t64 {
			p = "libasound2t64"
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// AptPackages are Packages for this machine; nil without apt-get.
func AptPackages(ctx context.Context, libs []string) []string {
	if _, err := exec.LookPath("apt-get"); err != nil {
		return nil
	}
	t64 := exec.CommandContext(ctx, "apt-cache", "show", "libasound2t64").Run() == nil
	return Packages(libs, t64)
}
