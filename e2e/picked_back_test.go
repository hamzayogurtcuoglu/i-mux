//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestPickedThenBack: a Claude conversation picked in History (X) and
// continued in Antigravity goes back to X when the pane switches to Claude
// again, with what happened in Antigravity; not to the pane's own earlier
// Claude run (Y). The same holds in a new pane.
func TestPickedThenBack(t *testing.T) {
	h := newH(t, "")
	x := h.synthetic("claude", 1, "synthetic picked task", time.Now().Add(-3*time.Hour))
	pane := h.startSession("claude")
	argv := h.run("claude", 1)
	if len(argv) < 2 || argv[0] != "--session-id" {
		t.Fatalf("claude run 1 argv %q", argv)
	}
	y := argv[1]

	h.mustImux("switch", "antigravity", "--pane", pane, "--from-session", "claude:"+x)
	if a := strings.Join(h.run("antigravity", 1), " "); !strings.Contains(a, "synthetic picked task") {
		t.Fatalf("antigravity was not given the picked conversation: %.300s", a)
	}
	h.mustImux("switch", "claude", "--pane", pane)
	argv = h.run("claude", 2)
	if len(argv) != 3 || argv[0] != "--resume" || argv[1] != x || !strings.Contains(argv[2], "fake antigravity reply 1") {
		t.Fatalf("back to Claude resumed %q (picked %s, pane's own %s): %.300q", argv, x, y, argv)
	}

	// In a new pane: the picked conversation continues in Codex there, and
	// Claude in that pane is the picked one too.
	np := strings.TrimSpace(h.mustImux("split", "codex", "-h", "--pane", pane, "--from-session", "claude:"+x))
	h.run("codex", 1)
	h.mustImux("switch", "claude", "--pane", np)
	argv = h.run("claude", 3)
	if len(argv) != 3 || argv[0] != "--resume" || argv[1] != x || !strings.Contains(argv[2], "fake codex reply 1") {
		t.Fatalf("new pane back to Claude %q (picked %s)", argv, x)
	}
}
