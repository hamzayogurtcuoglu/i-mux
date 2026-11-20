package cli

import (
	"os"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// TestMain pins the UI language: tests compare English output, and imux
// would otherwise follow the developer's LANG (tr_TR selects Turkish).
func TestMain(m *testing.M) {
	if os.Getenv("IMUX_TEST_SELF_UPDATE") == "1" {
		if code, ok := selfUpdateHelper(); ok {
			os.Exit(code)
		}
	}
	os.Setenv("IMUX_LANG", "en")
	// The sh form whatever the system (T90.9), as internal/tmux's tests.
	tmux.ArgvForm = false
	os.Exit(m.Run())
}
