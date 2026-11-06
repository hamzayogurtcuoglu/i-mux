//go:build e2e

package e2e

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestDiscoverTwoPanesSameCWD (T2.5): two panes running the same agent in the
// same directory each resolve to their own session, both for Claude
// (pre-assigned ids) and Gemini (start-time matching).
func TestDiscoverTwoPanesSameCWD(t *testing.T) {
	h := newH(t, "")
	a := h.startSession("claude")
	h.run("claude", 1)
	b := h.newPane(a, "claude")
	h.run("claude", 2)
	// Fake CLIs write whole-second timestamps: keep the Gemini runs apart.
	g1 := h.newPane(a, "gemini")
	h.run("gemini", 1)
	time.Sleep(2100 * time.Millisecond)
	g2 := h.newPane(a, "gemini")
	h.run("gemini", 2)
	// Make the first chat the newest by mtime, as if pane 1 kept talking:
	// newest-file-wins alone would now hand it to pane 2.
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(h.sessionPath("gemini", 1), future, future); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ pane, want, notWant string }{
		{a, "fake claude request 1 from pane " + a, "from pane " + b},
		{b, "fake claude request 2 from pane " + b, "from pane " + a},
		{g1, "fake gemini request 1 from pane " + g1, "from pane " + g2},
		{g2, "fake gemini request 2 from pane " + g2, "from pane " + g1},
	}
	for _, c := range cases {
		out, errOut, err := h.ajan("handoff", "--pane", c.pane, "--print", "--to", "codex")
		if err != nil {
			t.Fatalf("handoff --pane %s: %v\n%s", c.pane, err, errOut)
		}
		if !strings.Contains(out, c.want) || strings.Contains(out, c.notWant) {
			t.Errorf("pane %s resolved the wrong session; want %q:\n%.1500s", c.pane, c.want, out)
		}
	}
}
