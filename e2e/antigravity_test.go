//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestSwitchThroughAntigravity: Claude → Antigravity gets the handoff with
// -i; Antigravity → Codex rebuilds the conversation from the pane's
// scrollback (agy keeps no readable session file) and carries the task list
// from brain/<id>/task.md.
func TestSwitchThroughAntigravity(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)

	if out := h.mustImux("switch", "antigravity", "--pane", pane); !strings.Contains(out, "Switched to Antigravity") {
		t.Fatalf("switch output %q", out)
	}
	argv := h.run("antigravity", 1)
	if len(argv) != 2 || argv[0] != "-i" || !strings.HasPrefix(argv[1], "[i-mux handoff · Claude Code → Antigravity CLI · ") {
		t.Fatalf("antigravity argv %.200q", argv)
	}
	if !strings.Contains(argv[1], "fake claude reply 1") {
		t.Fatal("the handoff to Antigravity lacks Claude's conversation")
	}
	h.waitFor("antigravity's reply on screen", 10*time.Second, func() bool {
		return strings.Contains(h.mustTmux("capture-pane", "-p", "-t", pane), "fake antigravity reply 1")
	})

	if out := h.mustImux("switch", "codex", "--pane", pane); !strings.Contains(out, "Switched to Codex") {
		t.Fatalf("switch output %q", out)
	}
	argv = h.run("codex", 1)
	if len(argv) != 1 {
		t.Fatalf("codex argv %.200q", argv)
	}
	doc := argv[0]
	for _, want := range []string{
		"[i-mux handoff · Antigravity CLI → Codex CLI · ",
		"fake antigravity request 1 from pane " + pane, "fake antigravity reply 1",
		workspace.ReconstructedNote,
		"fake antigravity step done 1", "fake antigravity step in progress 1",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("handoff from Antigravity lacks %q:\n%.1500s", want, doc)
		}
	}
	if got := h.paneOpt(pane, "#{@imux_agent}|#{@imux_label}"); got != "codex|Codex" {
		t.Fatalf("pane after switch: %q", got)
	}
}
