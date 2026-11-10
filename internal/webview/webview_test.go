package webview

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestAssets(t *testing.T) {
	for _, p := range [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}} {
		a, ok := AssetFor(p[0], p[1])
		if !ok || len(a.Sum) != 64 || a.Size < 60<<20 || !strings.HasPrefix(URL(a), "https://github.com/fathyb/carbonyl/releases/download/v0.0.3/carbonyl.") {
			t.Errorf("%v: %+v %s", p, a, URL(a))
		}
	}
	if _, ok := AssetFor("windows", "amd64"); ok {
		t.Error("a Windows build")
	}
}

func TestTarget(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"index.html", "a b.html"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("<p>hi"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for in, want := range map[string]string{
		"":                       "",
		"  https://go.dev/doc  ": "https://go.dev/doc",
		"localhost:3000":         "http://localhost:3000",
		"3000":                   "http://localhost:3000",
		":8080/x":                "http://localhost:8080/x",
		"3000/api?q=1":           "http://localhost:3000/api?q=1",
		":8080":                  "http://localhost:8080",
		"127.0.0.1:8000/a?b=1":   "http://127.0.0.1:8000/a?b=1",
		"[::1]:3000":             "http://[::1]:3000",
		"app.localhost:5173":     "http://app.localhost:5173",
		"go.dev/doc":             "https://go.dev/doc",
		"about:blank":            "about:blank",
		"index.html":             "file://" + dir + "/index.html",
		"a b.html":               "file://" + dir + "/a%20b.html",
		dir + "/index.html":      "file://" + dir + "/index.html",
		"missing.html":           "https://missing.html",
	} {
		if got := Target(in, dir); got != want {
			t.Errorf("Target(%q) = %q, want %q", in, got, want)
		}
	}
	u := Target("a b.html", dir)
	if FilePath(u) != filepath.Join(dir, "a b.html") || FilePath("http://x/y") != "" || Label(u) != "a b.html" {
		t.Errorf("file %q: %q %q", u, FilePath(u), Label(u))
	}
	for u, want := range map[string]string{"http://localhost:3000/": "localhost:3000", "https://go.dev/doc/": "go.dev/doc", "about:blank": "about:blank"} {
		if got := Label(u); got != want {
			t.Errorf("Label(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestFiles(t *testing.T) {
	for name, want := range map[string][2]bool{
		"index.html": {true, true}, "A.HTM": {true, true}, "x.xhtml": {true, true},
		"logo.svg": {false, true}, "README.md": {false, false}, "html": {false, false},
	} {
		if got := [2]bool{IsHTML(name), Previewable(name)}; got != want {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
}

type entry struct {
	name string
	mode fs.FileMode
	body string
}

func makeZip(t *testing.T, entries []entry) string {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "x.zip")
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var fakeCarbonyl = []entry{
	{"carbonyl-0.0.3/", fs.ModeDir | 0o755, ""},
	{"carbonyl-0.0.3/carbonyl", 0o755, "#!/bin/sh\necho Carbonyl 0.0.3\n"},
	{"carbonyl-0.0.3/icudtl.dat", 0o644, "data"},
}

func TestUnzip(t *testing.T) {
	dir := t.TempDir()
	if err := Unzip(makeZip(t, fakeCarbonyl), dir); err != nil {
		t.Fatal(err)
	}
	bin, err := os.Stat(filepath.Join(dir, "carbonyl-0.0.3", "carbonyl"))
	if err != nil || bin.Mode()&0o100 == 0 {
		t.Fatalf("the program is not executable: %v %v", bin, err)
	}
	if d, err := os.Stat(filepath.Join(dir, "carbonyl-0.0.3", "icudtl.dat")); err != nil || d.Mode()&0o100 != 0 {
		t.Errorf("data: %v %v", d, err)
	}
	for _, bad := range [][]entry{
		{{"../evil", 0o644, "x"}},
		{{"/etc/evil", 0o644, "x"}},
		{{"carbonyl-0.0.3/link", fs.ModeSymlink | 0o777, "/etc/passwd"}},
	} {
		if err := Unzip(makeZip(t, bad), t.TempDir()); err == nil {
			t.Errorf("%q unpacked", bad[0].name)
		}
	}
}

// Install downloads the zip, checks it, unpacks it into ~/.imux/tools;
// a download that is not the release's leaves nothing.
func TestInstall(t *testing.T) {
	i18n.SetLocale("en")
	testutil.Isolate(t)
	zipFile := makeZip(t, fakeCarbonyl)
	body, err := os.ReadFile(zipFile)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(body)
	good := hex.EncodeToString(h[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	t.Setenv("IMUX_WEB_URL", srv.URL)
	old := sum
	defer func() { sum = old }()
	cfg := config.Default()

	sum = func(Asset) string { return strings.Repeat("0", 64) }
	var out bytes.Buffer
	in := &Installer{Out: &out, Bar: true, GOOS: "linux", GOARCH: "amd64"}
	if err := in.Install(context.Background()); !errors.Is(err, ErrChecksum) || Installed(cfg) {
		t.Fatalf("a wrong download: %v, installed %v", err, Installed(cfg))
	}

	sum = func(Asset) string { return good }
	out.Reset()
	if err := in.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "100%") || !Installed(cfg) {
		t.Fatalf("installed %v, output %q", Installed(cfg), out.String())
	}
	if v, err := CheckVersion(context.Background(), Browser(cfg)); err != nil || v != "Carbonyl 0.0.3" {
		t.Errorf("version %q %v", v, err)
	}
	if got := Command(cfg, "file:///x.html", true); !reflect.DeepEqual(got, []string{Bin(), "--no-sandbox", "file:///x.html"}) {
		t.Errorf("as root: %q", got)
	}
	if got := Command(cfg, "file:///x.html", false); !reflect.DeepEqual(got, []string{Bin(), "file:///x.html"}) {
		t.Errorf("command: %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(Dir()), "*")); len(left) != 1 {
		t.Errorf("left in tools: %q", left)
	}
	if err := Remove(); err != nil || Installed(cfg) {
		t.Errorf("remove: %v", err)
	}
	if err := (&Installer{GOOS: "windows", GOARCH: "amd64"}).Install(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Errorf("windows: %v", err)
	}
}

// [web] browser runs another program; it is "installed" when on the PATH.
func TestCustomBrowser(t *testing.T) {
	testutil.Isolate(t)
	cfg := config.Default()
	cfg.Web.Browser = "sh -x"
	got := Command(cfg, "http://x", true)
	if len(got) != 3 || filepath.Base(got[0]) != "sh" || got[1] != "-x" || got[2] != "http://x" {
		t.Errorf("custom: %q", got)
	}
	cfg.Web.Browser = "no-such-browser-imux"
	if Installed(cfg) || Command(cfg, "http://x", false) != nil {
		t.Error("a missing browser is installed")
	}
}

func TestMissingLibraries(t *testing.T) {
	out := "\tlinux-vdso.so.1 (0x00007ffd)\n\tlibnss3.so => not found\n\tlibnssutil3.so => not found\n" +
		"\tlibasound.so.2 => not found\n\tlibexpat.so.1 => /lib/x86_64-linux-gnu/libexpat.so.1 (0x7f)\n\tlibfoo.so.9 => not found\n"
	libs := ParseLdd(out)
	if !reflect.DeepEqual(libs, []string{"libnss3.so", "libnssutil3.so", "libasound.so.2", "libfoo.so.9"}) {
		t.Fatalf("libs %q", libs)
	}
	if got := Packages(libs, false); !reflect.DeepEqual(got, []string{"libnss3", "libasound2"}) {
		t.Errorf("packages %q", got)
	}
	if got := Packages(libs, true); !reflect.DeepEqual(got, []string{"libnss3", "libasound2t64"}) {
		t.Errorf("t64 packages %q", got)
	}
}
