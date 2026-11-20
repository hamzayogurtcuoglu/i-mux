package autoui

import (
	"os"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// TestMain runs the tests in the sh form whatever the system (T90.9), as
// internal/tmux's do: the menus' commands are compared as sh lines, and the
// argv form of Windows has tests of its own.
func TestMain(m *testing.M) {
	tmux.ArgvForm = false
	os.Exit(m.Run())
}
