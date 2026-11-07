//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// TestOpenCodeRoundTrip (T24.4): Claude → OpenCode starts OpenCode with a
// session id imux chose and the handoff; back to Claude resumes Claude's own
// session with what happened in OpenCode (read through `opencode session
// export`).
func TestOpenCodeRoundTrip(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	argv := h.run("claude", 1)
	claudeID := ""
	for i, a := range argv {
		if a == "--session-id" && i+1 < len(argv) {
			claudeID = argv[i+1]
		}
	}

	h.mustImux("switch", "opencode", "--pane", pane)
	argv = h.run("opencode", 1)
	if len(argv) != 4 || argv[0] != "--session" || !strings.HasPrefix(argv[1], "ses_imux") || argv[2] != "--prompt" ||
		!strings.HasPrefix(argv[3], "[i-mux handoff · Claude Code → OpenCode") || !strings.Contains(argv[3], "fake claude request 1") {
		t.Fatalf("opencode argv %.300q", argv)
	}
	if got := h.paneOpt(pane, "#{@imux_agent}|#{@imux_label}"); got != "opencode|OpenCode" {
		t.Fatalf("pane %q", got)
	}

	h.mustImux("switch", "claude", "--pane", pane)
	argv = h.run("claude", 2)
	if len(argv) != 3 || argv[0] != "--resume" || argv[1] != claudeID || !strings.Contains(argv[2], "Welcome back") ||
		!strings.Contains(argv[2], "fake opencode reply 1") || !strings.Contains(argv[2], "$ fake test 1") {
		t.Fatalf("claude resume argv %.600q", argv)
	}
}
