//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestSwitchClaudeToGemini (T2.6): the pane restarts with the target, whose
// argv carries the handoff; options and trail follow; the result is shown.
func TestSwitchClaudeToGemini(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	out := h.mustImux("switch", "gemini", "--pane", pane)
	if !strings.Contains(out, "Switched to Gemini") {
		t.Fatalf("switch output %q", out)
	}
	argv := h.run("gemini", 1)
	if len(argv) != 2 || argv[0] != "-i" {
		t.Fatalf("gemini argv %q", argv)
	}
	doc := argv[1]
	if !strings.HasPrefix(doc, "[i-mux handoff · Claude Code → Gemini CLI · ") {
		t.Fatalf("no marker:\n%.300s", doc)
	}
	for _, want := range []string{
		"The user switched to you manually.",
		"## Original request", "fake claude request 1 from pane " + pane,
		"## Conversation", "fake claude reply 1",
		"## Where it stopped", "## What to do now",
		"## Repository state",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("handoff lacks %q", want)
		}
	}
	if got := h.paneOpt(pane, "#{@imux_agent}|#{@imux_label}|#{pane_dead}|#{@imux_lock}"); got != "gemini|Gemini|0|" {
		t.Fatalf("pane after switch: %q", got)
	}
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))
	if len(trail) != 2 || trail[0].Agent != "claude" || trail[0].End == 0 || trail[0].File == "" ||
		trail[1].Agent != "gemini" || trail[1].End != 0 || trail[1].Handoff == "" {
		t.Fatalf("trail %+v", trail)
	}
	fi, err := os.Stat(trail[1].Handoff)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("handoff file: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(trail[1].Handoff); string(b) != doc {
		t.Fatal("argv and handoff file differ")
	}
	start, _ := strconv.ParseInt(h.paneOpt(pane, "#{@imux_start}"), 10, 64)
	if start != trail[1].Start {
		t.Fatalf("@imux_start %d != trail start %d", start, trail[1].Start)
	}
	// The result was shown with display-message.
	shown := false
	for _, r := range h.records() {
		if strings.Contains(strings.Join(r.Args, " "), "display-message -t "+pane+" Switched to Gemini") {
			shown = true
		}
	}
	if !shown {
		t.Fatal("no display-message with the result")
	}

	// A concurrent switch (live pid in @imux_lock) is refused.
	h.mustTmux("set-option", "-p", "-t", pane, "@imux_lock", strconv.Itoa(os.Getpid()))
	if _, e, err := h.imux("switch", "codex", "--pane", pane); err == nil || !strings.Contains(e, "already in progress") {
		t.Fatalf("locked switch: %v %q", err, e)
	}
	h.mustTmux("set-option", "-p", "-u", "-t", pane, "@imux_lock")
	if h.runs("codex") != 0 {
		t.Fatal("locked switch started codex")
	}

	// A failure to write the handoff leaves the old agent running.
	hd := filepath.Join(h.Homes.Imux, "handoffs")
	if err := os.Chmod(hd, 0o500); err != nil {
		t.Fatal(err)
	}
	_, e, err := h.imux("switch", "codex", "--pane", pane)
	os.Chmod(hd, 0o700)
	if err == nil {
		t.Fatal("switch should fail when the handoff cannot be written")
	}
	time.Sleep(200 * time.Millisecond)
	if got := h.paneOpt(pane, "#{@imux_agent}|#{pane_dead}"); got != "gemini|0" || h.runs("codex") != 0 {
		t.Fatalf("old agent not kept after a failed switch (%q): %s", got, e)
	}
}

// TestSwitchReasonLimit: --reason limit puts the limit reason in the handoff.
func TestSwitchReasonLimit(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	h.mustImux("switch", "codex", "--pane", pane, "--reason", "limit")
	argv := h.run("codex", 1)
	if len(argv) != 1 || !strings.Contains(argv[0], "because Claude Code hit its usage limit") {
		t.Fatalf("codex argv %.400q", argv)
	}
}
