//go:build e2e

package e2e

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// fakeRelease writes <dir>/v9.9.9/{archive,checksums.txt} for this machine.
func fakeRelease(t *testing.T, bin string, corrupt bool) string {
	t.Helper()
	dir := t.TempDir()
	rel := filepath.Join(dir, "v9.9.9")
	if err := os.MkdirAll(rel, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "imux_9.9.9_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	f, err := os.Create(filepath.Join(rel, name))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	_ = tw.WriteHeader(&tar.Header{Name: "imux", Mode: 0o755, Size: int64(len(data))})
	_, _ = tw.Write(data)
	_ = tw.WriteHeader(&tar.Header{Name: "LICENSE", Mode: 0o644, Size: 3})
	_, _ = tw.Write([]byte("MIT"))
	tw.Close()
	gz.Close()
	f.Close()
	b, _ := os.ReadFile(filepath.Join(rel, name))
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	if corrupt {
		h = strings.Repeat("0", 64)
	}
	testutil.WriteFile(t, filepath.Join(rel, "checksums.txt"), []byte(h+"  "+name+"\n"+strings.Repeat("1", 64)+"  other.tar.gz\n"))
	return dir
}

func runInstall(t *testing.T, base, path string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{filepath.Join(testutil.ModuleRoot(t), "install.sh")}, args...)...)
	// HOME is the test's temporary home (testutil.Isolate), so the profile the
	// script edits is never the developer's.
	cmd.Env = append(os.Environ(), "IMUX_INSTALL_BASE_URL=file://"+base, "PATH="+path, "SHELL=/bin/zsh", "ZDOTDIR=")
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestInstallScript (T7.3): downloads, verifies the checksum, installs to
// --prefix, and puts the prefix on PATH in the shell profile exactly once; a
// wrong checksum aborts.
func TestInstallScript(t *testing.T) {
	bin := imuxBin(t)
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	h := testutil.Isolate(t)
	prefix := filepath.Join(t.TempDir(), "bin")
	tm, _ := exec.LookPath("tmux")
	path := filepath.Dir(tm) + ":/usr/bin:/bin"
	base := fakeRelease(t, bin, false)
	out, err := runInstall(t, base, path, "--prefix", prefix, "--version", "v9.9.9")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	profile := filepath.Join(h.Home, ".zshrc")
	for _, want := range []string{"Checksum verified.", "Installed " + prefix + "/imux", "Added " + prefix + " to PATH in " + profile, "tmux found", "Open a new terminal"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	line := `export PATH="` + prefix + `:$PATH"`
	// A second run (an upgrade) installs again without a second PATH line;
	// a version without the "v" is accepted.
	out, err = runInstall(t, base, path, "--prefix", prefix, "--version", "9.9.9")
	if err != nil || !strings.Contains(out, "already on PATH in "+profile) {
		t.Fatalf("reinstall: %v\n%s", err, out)
	}
	if b, _ := os.ReadFile(profile); strings.Count(string(b), line) != 1 {
		t.Fatalf("profile should hold the PATH line once:\n%s", b)
	}
	// --no-modify-path leaves the profile alone and only explains.
	other := filepath.Join(t.TempDir(), "bin")
	out, err = runInstall(t, base, path, "--prefix", other, "--version", "v9.9.9", "--no-modify-path")
	if err != nil || !strings.Contains(out, "is not on your PATH") {
		t.Fatalf("--no-modify-path: %v\n%s", err, out)
	}
	if b, _ := os.ReadFile(profile); strings.Contains(string(b), other) {
		t.Fatalf("--no-modify-path changed the profile:\n%s", b)
	}
	v, err := exec.Command(filepath.Join(prefix, "imux"), "version").Output()
	if err != nil || !strings.HasPrefix(string(v), "imux ") {
		t.Fatalf("installed binary: %v %q", err, v)
	}
	out, err = runInstall(t, fakeRelease(t, bin, true), path, "--prefix", filepath.Join(t.TempDir(), "b2"), "--version", "v9.9.9")
	if err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("corrupt archive accepted: %v\n%s", err, out)
	}
}

// TestInstallScriptWithoutTmux: without tmux and without a known package
// manager the script only explains what to do; it never runs sudo.
func TestInstallScriptWithoutTmux(t *testing.T) {
	bin := imuxBin(t)
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	testutil.Isolate(t)
	tools := t.TempDir()
	for _, name := range []string{"sh", "curl", "tar", "gzip", "sha256sum", "shasum", "mkdir", "cp", "chmod", "mv", "sed", "awk", "cut", "head", "uname", "mktemp", "rm", "id", "cat", "basename", "dirname", "grep", "tr"} {
		if p, err := exec.LookPath(name); err == nil {
			_ = os.Symlink(p, filepath.Join(tools, name))
		}
	}
	prefix := filepath.Join(t.TempDir(), "bin")
	out, err := runInstall(t, fakeRelease(t, bin, false), tools+":"+prefix, "--prefix", prefix, "--version", "v9.9.9")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "imux needs tmux") || !strings.Contains(out, "Install tmux with your package manager") ||
		strings.Contains(out, "sudo") || strings.Contains(out, "is not on your PATH") {
		t.Fatalf("output:\n%s", out)
	}
}
