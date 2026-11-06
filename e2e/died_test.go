//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// exitAgent makes the fake agent in pane exit with status.
func (h *H) exitAgent(pane string, status int) {
	h.t.Helper()
	h.mustTmux("send-keys", "-t", pane, "/exit "+itoa(status), "Enter")
}

// TestPaneDiedFallsBackToShell (T2.10): an agent exit leaves a usable login
// shell in @imux_cwd with @imux_agent cleared and the trail closed.
func TestPaneDiedFallsBackToShell(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	h.exitAgent(pane, 0)
	h.waitFor("shell in the pane", 5*time.Second, func() bool {
		return h.paneOpt(pane, "#{pane_dead}|#{@imux_agent}") == "0|"
	})
	if got := h.paneOpt(pane, "#{pane_current_path}|#{@imux_cwd}|#{remain-on-exit}"); got != h.Homes.Project+"|"+h.Homes.Project+"|off" {
		t.Fatalf("shell pane %q", got)
	}
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))
	if len(trail) != 1 || trail[0].End == 0 {
		t.Fatalf("trail not closed: %+v", trail)
	}
	// The shell works, and the pane can start an agent again from the menu
	// path (imux switch on a plain-shell pane), with context from the trail.
	h.mustTmux("send-keys", "-t", pane, "echo shell-$((40+2))", "Enter")
	h.waitFor("shell output", 5*time.Second, func() bool { return strings.Contains(h.capture(pane), "shell-42") })
	h.mustImux("switch", "gemini", "--pane", pane)
	if argv := h.run("gemini", 1); len(argv) != 2 || !strings.Contains(argv[1], "fake claude request 1") {
		t.Fatalf("restart from shell: %.200q", argv)
	}
}

// TestPaneDiedOnExitClose: on_exit = "close" kills the pane instead.
func TestPaneDiedOnExitClose(t *testing.T) {
	h := newH(t, "on_exit = \"close\"\n")
	pane := h.startSession("claude")
	h.run("claude", 1)
	other := strings.TrimSpace(h.mustImux("split", "gemini", "--pane", pane))
	h.run("gemini", 1)
	h.exitAgent(pane, 0)
	h.waitFor("pane closed", 5*time.Second, func() bool {
		out, _ := h.tmux("list-panes", "-a", "-F", "#{pane_id}")
		return !strings.Contains(out, pane+"\n") && strings.Contains(out, other+"\n")
	})
}

// TestPaneDiedFallbackSpec: a run that exits non-zero within 10 s of its
// start is replaced by @imux_fallback; a clean exit is not.
func TestPaneDiedFallbackSpec(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	fallback := func() string {
		t.Helper()
		p, err := launch.Write(&launch.Spec{
			Agent: "gemini", Bin: filepath.Join(h.Fakes, "fake-gemini"), Args: []string{"-i", "full handoff from the fallback"},
			Dir: h.Homes.Project, Env: map[string]string{"PATH": h.Fakes + ":/usr/bin:/bin"},
		})
		if err != nil {
			t.Fatal(err)
		}
		h.mustTmux("set-option", "-p", "-t", pane, "@imux_fallback", p)
		return p
	}
	spec := fallback()
	h.exitAgent(pane, 1)
	argv := h.run("gemini", 1)
	if len(argv) != 2 || argv[1] != "full handoff from the fallback" {
		t.Fatalf("fallback argv %q", argv)
	}
	h.waitFor("fallback options", 5*time.Second, func() bool {
		return h.paneOpt(pane, "#{@imux_agent}|#{@imux_fallback}|#{pane_dead}") == "gemini||0"
	})
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))
	if len(trail) != 1 || trail[0].Agent != "gemini" {
		t.Fatalf("the failed run should be replaced in the trail: %+v", trail)
	}
	_ = spec

	// A clean exit (status 0) does not use the fallback.
	fb := fallback()
	h.exitAgent(pane, 0)
	h.waitFor("shell", 5*time.Second, func() bool { return h.paneOpt(pane, "#{pane_dead}|#{@imux_agent}") == "0|" })
	if h.runs("gemini") != 1 || h.paneOpt(pane, "#{@imux_fallback}") != "" {
		t.Fatal("clean exit used or kept the fallback")
	}
	if _, err := launch.Read(fb, false); err == nil {
		t.Fatal("unused fallback spec not removed")
	}
}
