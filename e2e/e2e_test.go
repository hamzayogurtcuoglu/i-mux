//go:build e2e

// Package e2e holds end-to-end tests that drive a real tmux server on a
// private socket with fake agents. Run with `make e2e`.
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// imuxBin returns the binary under test (built by `make e2e`).
func imuxBin(t *testing.T) string {
	t.Helper()
	if os.Getenv("IMUX_E2E_BIN") == "" {
		t.Skip("IMUX_E2E_BIN not set (run via make e2e)")
	}
	return stagedBin
}

// stagedBin and stagedFakes are copies of the binary under test and of
// e2e/fakeagents in a private temp dir. Running them from the local file
// system keeps latency measurements honest on slow mounts (WSL's /mnt/c
// costs ~65 ms per exec).
var stagedBin, stagedFakes string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "imux-e2e-")
	if err != nil {
		panic(err)
	}
	// On macOS the temp dir is under a symlink (/var → /private/var), and
	// os.Executable reports the resolved path: resolve it so tests that match
	// the binary's command line (pgrep) see the same path the processes use.
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		panic(err)
	}
	if bin := os.Getenv("IMUX_E2E_BIN"); bin != "" {
		stagedBin = filepath.Join(dir, "imux")
		if err := copyFile(bin, stagedBin, 0o755); err != nil {
			panic(err)
		}
	}
	stagedFakes = filepath.Join(dir, "fakeagents")
	if err := os.Mkdir(stagedFakes, 0o755); err != nil {
		panic(err)
	}
	ents, err := os.ReadDir("fakeagents")
	if err != nil {
		panic(err)
	}
	for _, e := range ents {
		if err := copyFile(filepath.Join("fakeagents", e.Name()), filepath.Join(stagedFakes, e.Name()), 0o755); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func copyFile(src, dst string, mode os.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, mode)
}

func TestBinaryRuns(t *testing.T) {
	bin := imuxBin(t)
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.HasPrefix(string(out), "imux ") {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestTmuxAvailable(t *testing.T) {
	out, err := exec.Command("tmux", "-V").CombinedOutput()
	if err != nil {
		t.Fatalf("tmux not available: %v %s", err, out)
	}
}
