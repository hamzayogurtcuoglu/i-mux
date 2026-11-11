package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.27.0", "0.26.1", true},
		{"0.26.1", "0.26.1", false},
		{"0.26.1", "0.27.0", false},
		{"1.0.0", "0.99.99", true},
		{"v0.10.0", "0.9.9", true},
		{"0.27.0", "dev", false},
		{"x", "0.1.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/v0.27.0", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer srv.Close()
	old := LatestURL
	defer func() { LatestURL = old }()
	LatestURL = srv.URL + "/releases/latest"
	if v, err := Latest(context.Background()); err != nil || v != "0.27.0" {
		t.Fatalf("latest %q %v", v, err)
	}
	LatestURL = srv.URL + "/down"
	if _, err := Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "504") {
		t.Fatalf("an outage: %v", err)
	}
}

func TestInstallCmd(t *testing.T) {
	c := InstallCmd("0.27.0", "/Users/me/.local/bin")
	if c[0] != "sh" || !strings.Contains(c[2], "--version") || c[4] != "https://github.com/"+Repo+"/releases/download/v0.27.0/install.sh" ||
		c[5] != "v0.27.0" || c[6] != "/Users/me/.local/bin" {
		t.Fatalf("%q", c)
	}
	if !Homebrew("/opt/homebrew/Cellar/imux/0.26.1/bin") || Homebrew("/Users/me/.local/bin") {
		t.Fatal("homebrew")
	}
}

// TestInstaller: on Windows the release's install.ps1 is downloaded and
// run as a file by PowerShell, the folder passed as one argument.
func TestInstaller(t *testing.T) {
	c := WindowsInstallCmd(`C:\Temp\imux-install-1.ps1`, "0.40.0", `C:\Users\o'me\AppData\Local\Programs\imux`)
	want := []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", `C:\Temp\imux-install-1.ps1`,
		"-Version", "v0.40.0", "-InstallDir", `C:\Users\o'me\AppData\Local\Programs\imux`, "-NoModifyPath", "-NoFont", "-NoPsmux"}
	if strings.Join(c, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", c)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0.40.0/install.ps1":
			_, _ = w.Write([]byte("# the installer"))
		case "/v0.40.0/install.sh":
			_, _ = w.Write([]byte("#!/bin/sh"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	old := DownloadBase
	defer func() { DownloadBase = old }()
	DownloadBase = srv.URL
	argv, cleanup, err := Installer(context.Background(), "0.40.0", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if runtime.GOOS != "windows" {
		if argv[0] != "sh" || argv[4] != srv.URL+"/v0.40.0/install.sh" {
			t.Fatalf("%q", argv)
		}
		return
	}
	b, err := os.ReadFile(argv[6])
	if argv[0] != "powershell.exe" || err != nil || string(b) != "# the installer" {
		t.Fatalf("%q %v %q", argv, err, b)
	}
	cleanup()
	if _, err := os.Stat(argv[6]); err == nil {
		t.Fatal("the script is left behind")
	}
	if _, _, err := Installer(context.Background(), "0.41.0", t.TempDir()); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("no such release: %v", err)
	}
}

// TestRemoveOld: the copies an update renamed aside go; imux.exe and other
// files stay.
func TestRemoveOld(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"imux.exe", "imux.exe.old", "imux.exe.old.1a2b", "imux.exe.older", "psmux.exe.old"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if n := removeOld(filepath.Join(dir, "imux.exe")); n != 2 {
		t.Fatalf("removed %d", n)
	}
	left, _ := os.ReadDir(dir)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "imux.exe,imux.exe.older,psmux.exe.old" {
		t.Fatalf("left %v", names)
	}
}

// TestInstallPS1 guards install.ps1, which imux update runs on Windows: it
// is ASCII (Windows PowerShell reads a file without a BOM in the ANSI code
// page) and has no backtick (Windows' malware scan blocks such a script
// when a program, as imux update, starts it), and it takes the options
// WindowsInstallCmd passes.
func TestInstallPS1(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range b {
		if c > 127 || c == '`' {
			t.Fatalf("install.ps1 byte %d is %q", i, c)
		}
	}
	for _, opt := range WindowsInstallCmd("x.ps1", "1.2.3", "dir")[7:] {
		if strings.HasPrefix(opt, "-") && !strings.Contains(string(b), "$"+opt[1:]) {
			t.Errorf("install.ps1 has no %s parameter", opt)
		}
	}
}
