package commitmsg

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestMain runs the fake agents of TestExecute on Windows, where they are
// this test binary (testutil.Script) instead of shell scripts.
func TestMain(m *testing.M) {
	testutil.FakeMain(map[string]testutil.FakeFunc{
		// "ok" reads stdin and replies with a preamble and the message.
		"ok": func(string, []string) int {
			_, _ = io.Copy(io.Discard, os.Stdin)
			fmt.Print("Here is the message:\n\nfeat: from stdout\n")
			return 0
		},
		// "file" prints progress and writes the message to the file in $1.
		"file": func(_ string, args []string) int {
			fmt.Println("progress")
			if len(args) == 0 || os.WriteFile(args[0], []byte("fix: from the file\n"), 0o600) != nil {
				return 1
			}
			return 0
		},
		// "bad" fails with an error on stderr.
		"bad": func(string, []string) int {
			fmt.Fprintln(os.Stderr, "Error: not logged in")
			return 1
		},
		// "empty" replies with an empty code block.
		"empty": func(string, []string) int {
			fmt.Print("```\n```\n")
			return 0
		},
		// "slow" never replies in time.
		"slow": func(string, []string) int {
			time.Sleep(5 * time.Second)
			return 0
		},
	})
	os.Exit(m.Run())
}
