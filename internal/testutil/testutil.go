// Package testutil provides isolated homes, fake agent stores and a frozen
// clock for tests. Tests must never read the developer's real ~/.claude,
// ~/.gemini, ~/.copilot, ~/.codex or ~/.ajan.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/clock"
)

// FrozenTime is the default instant used by tests (a Thursday afternoon UTC).
var FrozenTime = time.Date(2026, 11, 5, 14, 32, 0, 0, time.UTC)

// Homes are the temp directories a test runs with.
type Homes struct {
	Root    string // temp root
	Home    string // $HOME
	Ajan    string // $AJAN_HOME
	Claude  string // $CLAUDE_CONFIG_DIR
	Gemini  string // ~/.gemini
	Copilot string // $COPILOT_HOME
	Codex   string // $CODEX_HOME
	Project string // a project directory to use as cwd
}

// Isolate points HOME, AJAN_HOME and every agent home at fresh temp dirs and
// returns them. Agent store directories are created empty.
func Isolate(t testing.TB) *Homes {
	t.Helper()
	root := t.TempDir()
	// Resolve symlinks (macOS /var → /private/var) so that cwd comparisons
	// against os.Getwd() results hold.
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	h := &Homes{
		Root:    root,
		Home:    filepath.Join(root, "home"),
		Project: filepath.Join(root, "home", "proj"),
	}
	h.Ajan = filepath.Join(h.Home, ".ajan")
	h.Claude = filepath.Join(h.Home, ".claude")
	h.Gemini = filepath.Join(h.Home, ".gemini")
	h.Copilot = filepath.Join(h.Home, ".copilot")
	h.Codex = filepath.Join(h.Home, ".codex")
	for _, d := range []string{
		h.Home, h.Project,
		filepath.Join(h.Claude, "projects"),
		filepath.Join(h.Gemini, "tmp"),
		filepath.Join(h.Copilot, "session-state"),
		filepath.Join(h.Codex, "sessions"),
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", h.Home)
	t.Setenv("AJAN_HOME", h.Ajan)
	t.Setenv("CLAUDE_CONFIG_DIR", h.Claude)
	t.Setenv("CODEX_HOME", h.Codex)
	t.Setenv("COPILOT_HOME", h.Copilot)
	t.Setenv("GEMINI_CLI_HOME", "")
	t.Setenv("AJAN_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h.Home, ".config"))
	return h
}

// Clock returns a clock frozen at FrozenTime.
func Clock() *clock.Fixed { return clock.NewFixed(FrozenTime) }

// WriteFile writes data to path, creating parent directories.
func WriteFile(t testing.TB, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Touch sets a file's modification time.
func Touch(t testing.TB, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// ModuleRoot returns the directory containing go.mod.
func ModuleRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
