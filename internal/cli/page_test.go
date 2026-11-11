package cli

import (
	"bytes"
	"strings"
	"testing"
)

// __page runs an imux command and pages what it prints, its errors too.
func TestPageRunsCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(&Env{Stdin: strings.NewReader("\n"), Stdout: &out, Stderr: &errOut}, []string{"__page", "no-such-command"})
	if code != ExitOK || !strings.Contains(out.String(), "no-such-command") {
		t.Fatalf("code %d out %q err %q", code, out.String(), errOut.String())
	}
}
