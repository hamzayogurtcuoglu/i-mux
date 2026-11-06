package cli

import (
	"os"
	"testing"
)

// TestMain pins the UI language: tests compare English output, and ajan
// would otherwise follow the developer's LANG (tr_TR selects Turkish).
func TestMain(m *testing.M) {
	os.Setenv("AJAN_LANG", "en")
	os.Exit(m.Run())
}
