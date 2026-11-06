//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestSwitchPasteMode (T2.7): with inject = "paste" the target starts without
// the prompt in argv; once its screen is stable the handoff is pasted and
// submitted, and the fake agent reads the whole document from its terminal.
func TestSwitchPasteMode(t *testing.T) {
	h := newH(t, "[handoff]\ninject = \"paste\"\n")
	pane := h.startSession("claude")
	h.run("claude", 1)
	start := time.Now()
	h.mustAjan("switch", "gemini", "--pane", pane)
	if d := time.Since(start); d < workspace.StableFor {
		t.Fatalf("pasted after %v, before the screen was stable for %v", d, workspace.StableFor)
	}
	if argv := h.run("gemini", 1); len(argv) != 0 {
		t.Fatalf("prompt leaked into argv: %.200q", argv)
	}
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@ajan_trail}"))
	doc, err := os.ReadFile(trail.Last().Handoff)
	if err != nil {
		t.Fatal(err)
	}
	stdin := filepath.Join(h.Log, "gemini-1.stdin")
	want := strings.TrimRight(string(doc), "\n")
	var got string
	h.waitFor("pasted handoff on stdin", 5*time.Second, func() bool {
		b, _ := os.ReadFile(stdin)
		got = strings.TrimRight(string(b), "\n")
		return got == want
	})
	if !strings.HasPrefix(got, "[ajan handoff · Claude Code → Gemini CLI") {
		t.Fatalf("stdin %.200q", got)
	}
	// The paste file and the tmux buffer are gone.
	if m, _ := filepath.Glob(filepath.Join(h.Homes.Ajan, "launch", "ajan-*.txt")); len(m) != 0 {
		t.Fatalf("paste files left: %v", m)
	}
	if out, _ := h.tmux("list-buffers", "-F", "#{buffer_name}"); strings.Contains(out, "ajan-") {
		t.Fatalf("buffer left: %s", out)
	}
}
