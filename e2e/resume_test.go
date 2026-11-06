//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestResumeBackWithDelta (T6.5): claude → antigravity → claude resumes Claude's
// session with a delta that holds only Antigravity's part.
func TestResumeBackWithDelta(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	first := h.run("claude", 1)
	claudeID := first[1]
	h.mustImux("switch", "antigravity", "--pane", pane)
	h.run("antigravity", 1)
	time.Sleep(1100 * time.Millisecond) // whole-second timestamps in the fakes
	out := h.mustImux("switch", "claude", "--pane", pane)
	if !strings.Contains(out, "Resumed Claude") {
		t.Fatalf("switch output %q", out)
	}
	argv := h.run("claude", 2)
	if len(argv) != 3 || argv[0] != "--resume" || argv[1] != claudeID {
		t.Fatalf("resume argv %.200q (want --resume %s <delta>)", argv, claudeID)
	}
	delta := argv[2]
	if !strings.Contains(delta, "Welcome back") || !strings.Contains(delta, "fake antigravity request 1") {
		t.Fatalf("delta lacks Antigravity's part:\n%s", delta)
	}
	if strings.Contains(delta, "fake claude request 1") || strings.Contains(delta, "## Original request") {
		t.Fatalf("delta repeats Claude's own part:\n%s", delta)
	}
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))
	last := trail.Last()
	if len(trail) != 3 || last.Agent != "claude" || !last.Resumed || last.SessionID != claudeID {
		t.Fatalf("trail %+v", trail)
	}
	if h.paneOpt(pane, "#{@imux_fallback}") == "" {
		t.Fatal("no fallback spec for the resume")
	}
}

// TestResumeFailsFast (T6.5): a resume that exits 1 at once is replaced by
// the fallback: a new session with the full handoff.
func TestResumeFailsFast(t *testing.T) {
	h := newH(t, "")
	h.control("claude", "case \"$1\" in --resume) echo 'session not found'; exit 1;; esac\n")
	pane := h.startSession("claude")
	h.run("claude", 1)
	h.mustImux("switch", "antigravity", "--pane", pane)
	h.run("antigravity", 1)
	time.Sleep(1100 * time.Millisecond)
	h.mustImux("switch", "claude", "--pane", pane)
	if argv := h.run("claude", 2); argv[0] != "--resume" {
		t.Fatalf("expected a resume first: %q", argv)
	}
	argv := h.run("claude", 3)
	if len(argv) != 3 || argv[0] != "--session-id" || !strings.HasPrefix(argv[2], "[i-mux handoff · Antigravity CLI → Claude Code") {
		t.Fatalf("fallback argv %.200q", argv)
	}
	h.waitFor("fallback run settled", 5*time.Second, func() bool {
		return h.paneOpt(pane, "#{@imux_agent}|#{@imux_fallback}|#{pane_dead}") == "claude||0"
	})
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))
	if l := trail.Last(); l.Resumed || l.SessionID != argv[1] {
		t.Fatalf("trail after fallback %+v", trail)
	}
}
