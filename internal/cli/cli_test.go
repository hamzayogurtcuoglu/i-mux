package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	code = Run(&Env{Stdin: strings.NewReader(""), Stdout: &o, Stderr: &e}, args)
	return o.String(), e.String(), code
}

func TestVersion(t *testing.T) {
	out, _, code := runCLI(t, "version")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, "ajan ") || !strings.Contains(out, "commit") {
		t.Fatalf("unexpected version output %q", out)
	}
}

func TestHelpListsCommands(t *testing.T) {
	out, _, code := runCLI(t, "help")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	for name, c := range commands {
		if c.Hidden {
			if strings.Contains(out, "  "+name+" ") {
				t.Errorf("hidden command %s listed", name)
			}
			continue
		}
		if !strings.Contains(out, name) {
			t.Errorf("help does not list %s", name)
		}
	}
}

func TestUnknownCommandExit2(t *testing.T) {
	_, errOut, code := runCLI(t, "definitely-not-a-command")
	if code != ExitUsage {
		t.Fatalf("exit %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "unknown command") || !strings.Contains(errOut, "Commands:") {
		t.Fatalf("stderr should contain the error and the help: %q", errOut)
	}
}
