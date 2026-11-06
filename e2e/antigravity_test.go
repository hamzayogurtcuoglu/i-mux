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

// TestSwitchThroughAntigravity: a full round trip with Antigravity CLI.
//
//  1. Claude → Antigravity: agy gets the handoff with -i.
//  2. Antigravity → Codex: the handoff is built from agy's transcript
//     (brain/<id>/.system_generated/logs/transcript_full.jsonl): user and
//     assistant turns, the commands it ran, and the task list from task.md;
//     not from the scrollback.
//  3. Codex → Antigravity: agy resumes its own conversation with
//     --conversation <id> and gets only what happened in Codex.
func TestSwitchThroughAntigravity(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)

	// 1. Claude → Antigravity.
	if out := h.mustImux("switch", "antigravity", "--pane", pane); !strings.Contains(out, "Switched to Antigravity") {
		t.Fatalf("switch output %q", out)
	}
	argv := h.run("antigravity", 1)
	if len(argv) != 2 || argv[0] != "-i" || !strings.HasPrefix(argv[1], "[i-mux handoff · Claude Code → Antigravity CLI · ") {
		t.Fatalf("antigravity argv %.200q", argv)
	}
	for _, want := range []string{"## Original request", "fake claude request 1 from pane " + pane, "fake claude reply 1"} {
		if !strings.Contains(argv[1], want) {
			t.Errorf("handoff to Antigravity lacks %q", want)
		}
	}
	sessionFile := filepath.Join(h.Log, "antigravity-1.session")
	h.waitFor("agy's transcript", 10*time.Second, func() bool {
		_, err := os.Stat(sessionFile)
		return err == nil
	})

	// 2. Antigravity → Codex, from agy's transcript.
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
		"**User:** fake antigravity request 1 from pane " + pane,
		"**Assistant:** fake antigravity reply 1",
		"$ fake test 1",
		"fake antigravity step done 1", "fake antigravity step in progress 1",
		"transcript_full.jsonl",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("handoff from Antigravity lacks %q:\n%.1800s", want, doc)
		}
	}
	if strings.Contains(doc, workspace.ReconstructedNote) {
		t.Error("the handoff from Antigravity fell back to the scrollback")
	}
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))
	if len(trail) != 3 || trail[1].Agent != "antigravity" || trail[1].SessionID == "" {
		t.Fatalf("trail %+v", trail)
	}
	agyID := trail[1].SessionID

	// 3. Codex → Antigravity resumes agy's own conversation with a delta.
	if out := h.mustImux("switch", "antigravity", "--pane", pane); !strings.Contains(out, "Resumed Antigravity") {
		t.Fatalf("switch back output %q", out)
	}
	argv = h.run("antigravity", 2)
	if len(argv) != 4 || argv[0] != "--conversation" || argv[1] != agyID || argv[2] != "-i" {
		t.Fatalf("resume argv %.300q (want --conversation %s -i <delta>)", argv, agyID)
	}
	delta := argv[3]
	if !strings.Contains(delta, "Welcome back") || !strings.Contains(delta, "fake codex request 1") ||
		strings.Contains(delta, "fake antigravity reply 1") {
		t.Fatalf("delta should hold only Codex's part:\n%.1500s", delta)
	}
	if got := h.paneOpt(pane, "#{@imux_agent}|#{@imux_label}"); got != "antigravity|Antigravity" {
		t.Fatalf("pane after switching back: %q", got)
	}
}
