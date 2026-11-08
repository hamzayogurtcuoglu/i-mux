//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestDoctor (T2.12): passes with the fake agents, never starts an agent with
// a prompt, and fails without tmux.
func TestDoctor(t *testing.T) {
	h := newH(t, "")
	out, errOut, err := h.imux("doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s%s", err, out, errOut)
	}
	for _, want := range []string{"✓ tmux ", "Claude Code 9.9.9-fake", "Antigravity CLI 9.9.9-fake", "VS Code", "menu: prefix a"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
	for _, a := range []string{"claude", "antigravity", "copilot", "codex", "opencode"} {
		if h.runs(a) != 0 {
			t.Fatalf("doctor started %s", a)
		}
	}
	if out, _, err := h.imux("doctor", "--ci"); err != nil || strings.Contains(out, "Claude Code") {
		t.Fatalf("doctor --ci: %v\n%s", err, out)
	}
	// Without tmux: a hard failure and a non-zero exit.
	cmd := exec.Command(h.Bin, "doctor", "--ci")
	cmd.Dir = h.Homes.Project
	cmd.Env = append(os.Environ(), "PATH=/nonexistent", "IMUX_TMUX_BIN=tmux")
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "✗ tmux is not installed") {
		t.Fatalf("doctor without tmux: %v\n%s", err, b)
	}
}
