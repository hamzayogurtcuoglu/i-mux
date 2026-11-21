package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// startEnv is the environment the test binary started with: the engine is
// built with it, not with a test's isolated HOME, where Go would put its
// module cache (read-only files the test's cleanup cannot remove).
var startEnv = os.Environ()

var (
	engineOnce sync.Once
	enginePath string
	engineErr  error
)

// Engine is the path of an imux-core binary for tests that need a real
// server (D87: i-mux's only engine): IMUX_TMUX_BIN when set, else one built
// once per test binary from go.mod's pin (Go's build cache makes it quick).
// The test is skipped when it cannot be built.
func Engine(t testing.TB) string {
	t.Helper()
	if b := os.Getenv("IMUX_TMUX_BIN"); b != "" {
		return b
	}
	engineOnce.Do(func() {
		dir, err := os.MkdirTemp("", "imux-engine-")
		if err != nil {
			engineErr = err
			return
		}
		enginePath = filepath.Join(dir, Exe("imux-core"))
		cmd := exec.Command("go", "build", "-o", enginePath, "github.com/hamzayogurtcuoglu/i-mux/cmd/imux-core")
		cmd.Dir = ModuleRoot(t)
		cmd.Env = startEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			engineErr = &buildError{err, string(out)}
		}
	})
	if engineErr != nil {
		t.Skipf("no engine to test with: %v", engineErr)
	}
	return enginePath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + ": " + e.out }
