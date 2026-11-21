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

// fakeRelease writes <dir>/v9.9.9/{archive,checksums.txt} for this machine,
// with imux only, as releases before the engine (D83).
func fakeRelease(t *testing.T, bin string, corrupt bool) string {
	t.Helper()
	return fakeReleaseCore(t, bin, nil, corrupt)
}

// fakeReleaseCore is fakeRelease with core as the archive's imux-core when
// not nil, as a release with the engine ships it (T88.4).
func fakeReleaseCore(t *testing.T, bin string, core []byte, corrupt bool) string {
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
	if core != nil {
		_ = tw.WriteHeader(&tar.Header{Name: "imux-core", Mode: 0o755, Size: int64(len(core))})
		_, _ = tw.Write(core)
	}
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
	// IMUX_NO_FONT: the Nerd Font step has its own test (and must not
	// touch this machine's fonts).
	cmd.Env = append(os.Environ(), "IMUX_INSTALL_BASE_URL=file://"+base, "PATH="+path, "SHELL=/bin/zsh", "ZDOTDIR=", "IMUX_NO_FONT=1")
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestInstallScript (T7.3): downloads, verifies the checksum, installs to
// --prefix, and puts the prefix on PATH in the shell profile exactly once; a
// wrong checksum aborts.
func TestInstallScript(t *testing.T) {
	skipOnSystem(t)
	bin := imuxBin(t)
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	h := testutil.Isolate(t)
	prefix := filepath.Join(t.TempDir(), "bin")
	path := "/usr/bin:/bin"
	base := fakeRelease(t, bin, false)
	out, err := runInstall(t, base, path, "--prefix", prefix, "--version", "v9.9.9")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	profile := filepath.Join(h.Home, ".zshrc")
	for _, want := range []string{"Checksum verified.", "Installed " + prefix + "/imux", "Added " + prefix + " to PATH in " + profile, "Open a new terminal"} {
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

// TestInstallScriptWithoutTmux (D87): i-mux runs on imux-core, so without
// tmux the installer completes and says nothing about tmux; it never runs
// sudo.
func TestInstallScriptWithoutTmux(t *testing.T) {
	skipOnSystem(t)
	bin := imuxBin(t)
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	testutil.Isolate(t)
	tools := installTools(t)
	prefix := filepath.Join(t.TempDir(), "bin")
	out, err := runInstall(t, fakeReleaseCore(t, bin, []byte("#!/bin/sh\necho 'tmux 3.8 (imux-core 9.9.9)'\n"), false), tools+":"+prefix, "--prefix", prefix, "--version", "v9.9.9")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if strings.Contains(out, "tmux") || strings.Contains(out, "sudo") || strings.Contains(out, "is not on your PATH") {
		t.Fatalf("output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(prefix, "imux-core")); err != nil {
		t.Fatal("the engine was not installed")
	}
}

// TestInstallScriptOldTmuxSessions (D87, T92.3): sessions an earlier i-mux
// still runs on tmux get a note that the next imux moves them; the installer
// installs nothing else (Homebrew is never run).
func TestInstallScriptOldTmuxSessions(t *testing.T) {
	skipOnSystem(t)
	bin := imuxBin(t)
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	testutil.Isolate(t)
	tools := installTools(t)
	ran := filepath.Join(t.TempDir(), "brew-ran")
	testutil.WriteFile(t, filepath.Join(tools, "brew"), []byte("#!/bin/sh\ntouch '"+ran+"'\n"))
	testutil.WriteFile(t, filepath.Join(tools, "tmux"), []byte("#!/bin/sh\n[ \"$*\" = \"-L imux has-session\" ]\n"))
	for _, f := range []string{"brew", "tmux"} {
		if err := os.Chmod(filepath.Join(tools, f), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prefix := filepath.Join(t.TempDir(), "bin")
	out, err := runInstall(t, fakeRelease(t, bin, false), tools+":"+prefix, "--prefix", prefix, "--version", "v9.9.9", "--no-modify-path")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "the next imux moves your open projects from tmux to it") {
		t.Fatalf("output:\n%s", out)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("brew ran")
	}
}

// installTools is a folder of the tools install.sh uses, and no others.
func installTools(t *testing.T) string {
	t.Helper()
	tools := t.TempDir()
	for _, name := range []string{"sh", "curl", "tar", "gzip", "sha256sum", "shasum", "mkdir", "cp", "chmod", "mv", "sed", "awk", "cut", "head", "uname", "mktemp", "rm", "id", "cat", "basename", "dirname", "grep", "tr"} {
		if p, err := exec.LookPath(name); err == nil {
			_ = os.Symlink(p, filepath.Join(tools, name))
		}
	}
	return tools
}

// TestInstallScriptOffersFont (M43): after the binary the installer offers
// the Nerd Font the sidebar's icons need. Without a terminal to ask on and
// without --yes it only says how; --yes runs `imux font install --yes`;
// --no-font, or a font already in place (`imux font --check`), skips it.
func TestInstallScriptOffersFont(t *testing.T) {
	skipOnSystem(t)
	imuxBin(t)
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	testutil.Isolate(t)
	dir := t.TempDir()
	calls, ready := filepath.Join(dir, "calls"), filepath.Join(dir, "ready")
	// A fake imux: it logs how it is run; font --check fails until "ready".
	fake := filepath.Join(dir, "imux")
	testutil.WriteFile(t, fake, []byte("#!/bin/sh\necho \"$*\" >>'"+calls+"'\n"+
		"if [ \"$1 $2\" = 'font --check' ] && [ ! -e '"+ready+"' ]; then exit 1; fi\n"))
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	base := fakeRelease(t, fake, false)
	prefix := filepath.Join(dir, "bin")
	install := func(args ...string) (string, string) {
		t.Helper()
		_ = os.Remove(calls)
		cmd := exec.Command("sh", append([]string{
			filepath.Join(testutil.ModuleRoot(t), "install.sh"),
			"--prefix", prefix, "--version", "v9.9.9", "--no-modify-path",
		}, args...)...)
		cmd.Env = append(os.Environ(), "IMUX_INSTALL_BASE_URL=file://"+base, "PATH=/usr/bin:/bin", "IMUX_NO_FONT=")
		cmd.SysProcAttr = noTerminal() // no terminal to ask on
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("install %v: %v\n%s", args, err, out)
		}
		b, _ := os.ReadFile(calls)
		return string(out), string(b)
	}
	out, ran := install()
	if !strings.Contains(out, "Skipped. Later: imux font install") || ran != "font --check\n" {
		t.Fatalf("without --yes:\n%s\nran %q", out, ran)
	}
	out, ran = install("--yes")
	if ran != "font --check\nfont install --yes\n" || strings.Contains(out, "Skipped") {
		t.Fatalf("--yes:\n%s\nran %q", out, ran)
	}
	if out, ran = install("--yes", "--no-font"); ran != "" || strings.Contains(out, "Nerd Font") {
		t.Fatalf("--no-font:\n%s\nran %q", out, ran)
	}
	testutil.WriteFile(t, ready, nil)
	if out, ran = install("--yes"); ran != "font --check\n" || strings.Contains(out, "Nerd Font") {
		t.Fatalf("font in place:\n%s\nran %q", out, ran)
	}
}

// TestInstallScriptReplacesBoth (T88.4, D83): the self-update runs the
// release's installer as update.InstallCmd does; it replaces imux and the
// i-mux-core engine together, and when one cannot be replaced neither
// changes. A release without the engine still installs imux alone.
func TestInstallScriptReplacesBoth(t *testing.T) {
	skipOnSystem(t)
	bin := imuxBin(t)
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	testutil.Isolate(t)
	path := "/usr/bin:/bin"
	prefix := filepath.Join(t.TempDir(), "bin")
	newCore := []byte("#!/bin/sh\necho 'tmux 3.8 (imux-core 9.9.9)'\n")
	base := fakeReleaseCore(t, bin, newCore, false)
	oldImux, oldCore := []byte("#!/bin/sh\necho old imux\n"), []byte("#!/bin/sh\necho old core\n")
	setOld := func() {
		t.Helper()
		_ = os.RemoveAll(filepath.Join(prefix, "imux-core"))
		testutil.WriteFile(t, filepath.Join(prefix, "imux"), oldImux)
		testutil.WriteFile(t, filepath.Join(prefix, "imux-core"), oldCore)
		for _, b := range []string{"imux", "imux-core"} {
			if err := os.Chmod(filepath.Join(prefix, b), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	is := func(name string, want []byte) bool {
		b, err := os.ReadFile(filepath.Join(prefix, name))
		return err == nil && string(b) == string(want)
	}
	leftovers := func() {
		t.Helper()
		ents, _ := os.ReadDir(prefix)
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), ".") {
				t.Errorf("left behind: %s", e.Name())
			}
		}
	}
	update := func(base string) (string, error) {
		return runInstall(t, base, path, "--version", "v9.9.9", "--prefix", prefix, "--no-modify-path", "--yes")
	}

	// Both replaced.
	setOld()
	out, err := update(base)
	if err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}
	if is("imux", oldImux) || !is("imux-core", newCore) || !strings.Contains(out, "Installed "+prefix+"/imux-core") {
		t.Fatalf("not both replaced:\n%s", out)
	}
	if v, err := exec.Command(filepath.Join(prefix, "imux"), "version").Output(); err != nil || !strings.HasPrefix(string(v), "imux ") {
		t.Fatalf("installed imux: %v %q", err, v)
	}
	leftovers()

	// The engine's place is taken by a folder: nothing changes.
	setOld()
	_ = os.Remove(filepath.Join(prefix, "imux-core"))
	if err := os.Mkdir(filepath.Join(prefix, "imux-core"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err = update(base)
	if err == nil || !strings.Contains(out, "nothing was installed") || !is("imux", oldImux) {
		t.Fatalf("a failed engine changed imux: %v\n%s", err, out)
	}
	leftovers()

	// imux cannot be replaced once the engine was: the engine is put back.
	// macOS only: a user can make a file immutable there (chflags uchg).
	if runtime.GOOS == "darwin" {
		setOld()
		if err := exec.Command("chflags", "uchg", filepath.Join(prefix, "imux")).Run(); err != nil {
			t.Fatal(err)
		}
		out, err = update(base)
		_ = exec.Command("chflags", "nouchg", filepath.Join(prefix, "imux")).Run()
		if err == nil || !strings.Contains(out, "cannot replace "+prefix+"/imux") {
			t.Fatalf("an immutable imux was replaced: %v\n%s", err, out)
		}
		if !is("imux", oldImux) || !is("imux-core", oldCore) {
			t.Fatalf("the engine was not put back:\n%s", out)
		}
		leftovers()
	}

	// A release without the engine installs imux and leaves the engine.
	setOld()
	out, err = update(fakeRelease(t, bin, false))
	if err != nil || is("imux", oldImux) || !is("imux-core", oldCore) || strings.Contains(out, "imux-core") {
		t.Fatalf("a release without the engine: %v\n%s", err, out)
	}
	leftovers()
}
