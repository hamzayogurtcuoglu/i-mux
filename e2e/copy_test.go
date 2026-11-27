//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCopyReachesClipboard (D114): text a program in an agent's pane copies
// with OSC 52 becomes a paste buffer, which the engine passes on to the
// terminal's clipboard (set-clipboard on), and the agent starts with
// COPILOT_MULTIPLEXER=none, so Copilot copies with that plain OSC 52.
func TestCopyReachesClipboard(t *testing.T) {
	t.Setenv("COPILOT_MULTIPLEXER", "")
	if err := os.Unsetenv("COPILOT_MULTIPLEXER"); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "")
	h.control("copilot", "printf '%s' \"${COPILOT_MULTIPLEXER-unset}\" >\"$log/$agent-$n.mux\"\n"+
		"printf '\\033]52;c;Y29waWVkIGJ5IGNvcGlsb3Q=\\007'\n"+ // "copied by copilot"
		"sleep 60\n")
	h.startSession("copilot")
	h.run("copilot", 1)
	mux := filepath.Join(h.Log, "copilot-1.mux")
	h.waitFor("the agent's COPILOT_MULTIPLEXER", 5*time.Second, func() bool {
		return h.readFile(mux) != ""
	})
	if v := h.readFile(mux); v != "none" {
		t.Fatalf("COPILOT_MULTIPLEXER in the agent's pane: %q", v)
	}
	h.waitFor("the copy as a paste buffer", 5*time.Second, func() bool {
		out, err := h.tmux("show-buffer")
		return err == nil && out == "copied by copilot"
	})
}
