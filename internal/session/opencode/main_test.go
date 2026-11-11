package opencode

import (
	"os"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestMain lets the test binary act as the fake opencode CLI on Windows
// (testutil.Script).
func TestMain(m *testing.M) {
	testutil.FakeMain(nil)
	os.Exit(m.Run())
}
