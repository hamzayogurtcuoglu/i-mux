package gitinfo

import (
	"os"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestMain runs the fake git of TestTimeout on Windows, where it is this
// test binary (testutil.Script) instead of a shell script.
func TestMain(m *testing.M) {
	testutil.FakeMain(map[string]testutil.FakeFunc{
		// "sleep" waits for the duration in data.
		"sleep": func(data string, _ []string) int {
			d, err := time.ParseDuration(data)
			if err != nil {
				return 2
			}
			time.Sleep(d)
			return 0
		},
	})
	os.Exit(m.Run())
}
