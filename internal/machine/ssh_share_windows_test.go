//go:build windows

package machine

import (
	"slices"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// Windows' OpenSSH fails at once with any ControlMaster (D96): no ssh i-mux
// starts there may ask for one.
func TestNoMasterOnWindows(t *testing.T) {
	testutil.Isolate(t)
	if SharesConnection {
		t.Fatal("SharesConnection on Windows")
	}
	m := Machine{Name: "s", Target: "ali@s"}
	for _, a := range [][]string{Args(m, Background, "__remote-state"), Args(m, Interactive, "version"), Args(m, Attach, "x"), ShellArgs(m)} {
		if slices.ContainsFunc(a, func(s string) bool { return strings.HasPrefix(s, "Control") }) {
			t.Errorf("a master connection's option: %q", a)
		}
	}
}
