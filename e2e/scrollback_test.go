//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestScrollbackFallback (T2.11): with no session file, the handoff is
// built from the pane's scrollback, marked as reconstructed, and the user is
// told.
func TestScrollbackFallback(t *testing.T) {
	h := newH(t, "")
	testutil.WriteFile(t, filepath.Join(h.Log, "claude.nosession"), nil)
	h.control("claude", `echo "working on the payment webhook retry logic"; echo "step 3 of 5 done"`+"\n")
	pane := h.startSession("claude")
	h.run("claude", 1)
	h.waitFor("screen output", 5*time.Second, func() bool { return strings.Contains(h.capture(pane), "step 3 of 5") })
	h.mustImux("switch", "antigravity", "--pane", pane)
	argv := h.run("antigravity", 1)
	if len(argv) != 2 {
		t.Fatalf("argv %q", argv)
	}
	doc := argv[1]
	for _, want := range []string{"[i-mux handoff · Claude Code → Antigravity CLI", "payment webhook retry logic", "step 3 of 5 done", workspace.ReconstructedNote} {
		if !strings.Contains(doc, want) {
			t.Errorf("handoff lacks %q:\n%s", want, doc)
		}
	}
	shown := false
	for _, r := range h.records() {
		if strings.Contains(strings.Join(r.Args, " "), "context reconstructed from the screen") {
			shown = true
		}
	}
	if !shown {
		t.Fatal("the user was not told about the reconstruction")
	}
}
