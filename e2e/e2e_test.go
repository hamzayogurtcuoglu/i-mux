//go:build e2e

// Package e2e holds end-to-end tests that drive a real tmux server on a
// private socket with fake agents. Run with `make e2e`.
package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBinaryRuns(t *testing.T) {
	bin := os.Getenv("AJAN_E2E_BIN")
	if bin == "" {
		t.Skip("AJAN_E2E_BIN not set (run via make e2e)")
	}
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.HasPrefix(string(out), "ajan ") {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestTmuxAvailable(t *testing.T) {
	out, err := exec.Command("tmux", "-V").CombinedOutput()
	if err != nil {
		t.Fatalf("tmux not available: %v %s", err, out)
	}
}
