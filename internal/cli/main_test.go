package cli

import (
	"os"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestMain pins the UI language: tests compare English output, and imux
// would otherwise follow the developer's LANG (tr_TR selects Turkish).
func TestMain(m *testing.M) {
	testutil.FakeMain(nil)
	os.Setenv("IMUX_LANG", "en")
	os.Exit(m.Run())
}
