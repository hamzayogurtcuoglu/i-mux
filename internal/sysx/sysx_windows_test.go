//go:build windows

package sysx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file runs on Windows when PATHEXT names its extension: file modes have
// no execute bits there.
func TestExecutableByExtension(t *testing.T) {
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	dir := t.TempDir()
	for name, want := range map[string]bool{"imux.exe": true, "claude.CMD": true, "notes.txt": false, "imux": false} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := Executable(fi); got != want {
			t.Errorf("Executable(%s) = %v, want %v", name, got, want)
		}
	}
}

// The shell an agent exits to is i-mux-core's choice (§9.2a): a
// default-shell that does not exist (tmux's /bin/sh) gives PowerShell with
// -NoLogo, else cmd.exe; one that exists is kept.
func TestShellArgv(t *testing.T) {
	argv := ShellArgv("/bin/sh")
	if !filepath.IsAbs(argv[0]) {
		t.Fatalf("not a path: %q", argv)
	}
	switch strings.TrimSuffix(strings.ToLower(filepath.Base(argv[0])), ".exe") {
	case "pwsh", "powershell":
		if len(argv) != 2 || argv[1] != "-NoLogo" {
			t.Fatalf("%q", argv)
		}
	case "cmd":
		if len(argv) != 2 || argv[1] != "/a" {
			t.Fatalf("%q", argv)
		}
	default:
		t.Fatalf("not a Windows shell: %q", argv)
	}
	// Always a word more than the program (T90.11): one word alone is a shell
	// command line to the engine.
	cmd := filepath.Join(systemRoot(), "System32", "cmd.exe")
	if got := ShellArgv(cmd); len(got) != 2 || got[0] != cmd || got[1] != "/a" {
		t.Fatalf("ShellArgv(cmd.exe) = %q", got)
	}
	sh := filepath.Join(t.TempDir(), "sh.exe")
	if err := os.WriteFile(sh, []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ShellArgv(sh); len(got) != 2 || got[0] != sh || got[1] != "-l" {
		t.Fatalf("ShellArgv(sh.exe) = %q", got)
	}
}
