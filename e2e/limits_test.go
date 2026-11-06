//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
)

const fastLimits = "[limits]\npoll_interval = \"200ms\"\nidle_interval = \"500ms\"\n"

func limitLine(agent, delay, line string) string {
	return "sleep " + delay + "; echo \"" + line + "\"\n"
}

// watcherMenus counts display-menu calls by the watcher titled "hit its limit".
func (h *H) watcherMenus() int {
	n := 0
	for _, r := range h.records() {
		if strings.Contains(strings.Join(r.Args, " "), "hit its limit") && strings.Contains(strings.Join(r.Args, " "), "display-menu") {
			n++
		}
	}
	return n
}

// TestLimitAuto (T3.8): on_limit = auto switches to the first installed,
// non-limited agent; the handoff names the limit and its reset time.
func TestLimitAuto(t *testing.T) {
	h := newH(t, "on_limit = \"auto\"\n"+fastLimits+"grace = \"1s\"\n")
	h.control("claude", limitLine("claude", "2", "5-hour limit reached ∙ resets 3pm"))
	pane := h.startSession("claude")
	h.attach(120, 40)
	h.run("claude", 1)
	argv := h.run("gemini", 1)
	if len(argv) != 2 || !strings.Contains(argv[1], "because Claude Code hit its usage limit (resets ") {
		t.Fatalf("auto switch handoff: %.400q", argv)
	}
	h.waitFor("pane runs gemini", 5*time.Second, func() bool { return h.paneOpt(pane, "#{@imux_agent}") == "gemini" })
	st, _ := state.Read()
	l := st.Limits["claude"]
	if l == nil || l.Source != "screen" || l.Pane != pane || l.ResetAt.IsZero() || !strings.Contains(l.Text, "5-hour limit reached") {
		t.Fatalf("state record %+v", l)
	}
	h.waitFor("status bar shows the limit", 5*time.Second, func() bool {
		return strings.Contains(h.mustTmux("show-options", "-gv", "@imux_status"), "Claude ⛔")
	})
}

// TestLimitAsk (T3.8, Q24): on_limit = ask shows the prompt on the attached
// client; a repeated line does not prompt again; picking Gemini switches.
func TestLimitAsk(t *testing.T) {
	h := newH(t, fastLimits+"grace = \"1s\"\n")
	line := "5-hour limit reached ∙ resets 3pm"
	h.control("claude", limitLine("claude", "2", line)+"sleep 1; echo working; "+"echo '"+line+"'\n")
	pane := h.startSession("claude")
	c, client := h.attach(120, 40)
	h.run("claude", 1)
	c.waitScreen(h, "Claude hit its limit (resets ", 8*time.Second)
	c.waitScreen(h, "Gemini (with context)", 2*time.Second)
	var found bool
	for _, r := range h.records() {
		a := strings.Join(r.Args, " ")
		if strings.Contains(a, "display-menu -c "+client+" -t "+pane+" -T Claude hit its limit") {
			found = true
		}
	}
	if !found {
		t.Fatal("recorder lacks the watcher's display-menu on the attached client")
	}
	// The line is drawn again while the prompt is open: no second prompt.
	time.Sleep(2 * time.Second)
	if n := h.watcherMenus(); n != 1 {
		t.Fatalf("%d prompts, want 1", n)
	}
	c.Write("g")
	argv := h.run("gemini", 1)
	if !strings.Contains(argv[len(argv)-1], "hit its usage limit") {
		t.Fatalf("handoff reason: %.300q", argv)
	}
}

// TestLimitGrace (T3.8): a limit line printed within the grace period after
// the agent started (a resumed session re-rendering it) does nothing.
func TestLimitGrace(t *testing.T) {
	h := newH(t, "on_limit = \"auto\"\n"+fastLimits+"grace = \"10s\"\n")
	h.control("claude", limitLine("claude", "0.5", "5-hour limit reached ∙ resets 3pm"))
	h.startSession("claude")
	h.attach(120, 40)
	h.run("claude", 1)
	h.noRun("gemini", 1, 3*time.Second)
	if st, _ := state.Read(); st.Limits["claude"] != nil {
		t.Fatal("limit recorded within grace")
	}
}

// TestLimitStatusExpires (T3.8): the status bar and the pane flag show a
// limit, and clear once it expires (default_expiry when no reset time).
func TestLimitStatusExpires(t *testing.T) {
	h := newH(t, "on_limit = \"off\"\n"+fastLimits+"grace = \"0s\"\ndefault_expiry = \"3s\"\n")
	h.control("codex", limitLine("codex", "1", "You've hit your usage limit. Upgrade to Pro."))
	pane := h.startSession("codex")
	c, _ := h.attach(120, 40)
	h.run("codex", 1)
	h.waitFor("limit in the status bar", 6*time.Second, func() bool {
		return strings.Contains(h.mustTmux("show-options", "-gv", "@imux_status"), "Codex ⛔")
	})
	if f := h.paneOpt(pane, "#{@imux_flag}"); !strings.HasPrefix(f, "⛔ ") {
		t.Fatalf("pane flag %q", f)
	}
	c.waitScreen(h, "Codex hit its limit", 3*time.Second) // on_limit = off: a message only
	if h.watcherMenus() != 0 || h.runs("claude") != 0 {
		t.Fatal("off mode prompted or switched")
	}
	h.waitFor("limit cleared after expiry", 8*time.Second, func() bool {
		return !strings.Contains(h.mustTmux("show-options", "-gv", "@imux_status"), "⛔") && h.paneOpt(pane, "#{@imux_flag}") == ""
	})
	if st, _ := state.Read(); st.Limits["codex"] != nil {
		t.Fatal("expired limit kept in state.json")
	}
}
