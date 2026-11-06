//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestClick (T2.8b): every agent-bar region does what §9.3 says.
func TestClick(t *testing.T) {
	h := newH(t, "[agents.copilot]\nbin = \"/nonexistent/copilot\"\n")
	pane := h.startSession("claude")
	h.run("claude", 1)
	c, client := h.attach(140, 40)
	click := func(rng, p string) {
		t.Helper()
		h.mustImux("__click", rng, "--pane", p, "--client", client)
	}

	// Installed, not limited: switch in-process with context.
	click("imux:agy", pane)
	if argv := h.run("antigravity", 1); len(argv) != 2 || !strings.HasPrefix(argv[1], "[i-mux handoff · Claude Code → Antigravity CLI") {
		t.Fatalf("click switch argv %.200q", argv)
	}
	h.waitFor("pane agent antigravity", 5*time.Second, func() bool { return h.paneOpt(pane, "#{@imux_agent}") == "antigravity" })

	// The current agent, imux:menu and imux:cur open the pane menu.
	for _, rng := range []string{"imux:agy", "imux:menu", "imux:cur"} {
		mark := c.Mark()
		click(rng, pane)
		c.waitScreenAfter(h, mark, "i-mux · Antigravity", 5*time.Second)
		c.Write("q")
		time.Sleep(50 * time.Millisecond)
	}
	// imux:sessions opens the picker.
	mark := c.Mark()
	click("imux:sessions", pane)
	c.waitScreenAfter(h, mark, "Sessions · ", 5*time.Second)
	c.Write("q")

	// Not installed: the install hint.
	mark = c.Mark()
	click("imux:copilot", pane)
	c.waitScreenAfter(h, mark, "npm i -g @github/copilot", 5*time.Second)

	// Limited: a confirm menu first; "n" does nothing, "y" switches.
	if err := state.Update(func(s *state.State) error {
		s.SetLimit("codex", &state.Limit{At: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Severity: "limit", Source: "screen"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mark = c.Mark()
	click("imux:codex", pane)
	c.waitScreenAfter(h, mark, "Codex is limited until", 5*time.Second)
	c.Write("n")
	h.noRun("codex", 1, 500*time.Millisecond)
	mark = c.Mark()
	click("imux:codex", pane)
	c.waitScreenAfter(h, mark, "Switch anyway?", 5*time.Second)
	c.Write("y")
	h.run("codex", 1)

	// A plain-shell pane: the agent starts there, fresh without a trail...
	plain := strings.TrimSpace(h.mustTmux("split-window", "-h", "-t", pane, "-c", h.Homes.Project, "-P", "-F", "#{pane_id}", "sh"))
	click("imux:claude", plain)
	if argv := h.run("claude", 2); len(argv) != 2 || argv[0] != "--session-id" {
		t.Fatalf("fresh start argv %q", argv)
	}
	// ...and with context from its trail when the last run left a session.
	claudeTrail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))[0] // closed claude run of the first pane
	trail := workspace.Trail{claudeTrail}
	shell := strings.TrimSpace(h.mustTmux("split-window", "-v", "-t", pane, "-c", h.Homes.Project, "-P", "-F", "#{pane_id}", "sh"))
	h.mustTmux("set-option", "-p", "-t", shell, "@imux_trail", trail.String(), ";", "set-option", "-p", "-t", shell, "@imux_cwd", h.Homes.Project)
	click("imux:agy", shell)
	argv := h.run("antigravity", 2)
	if len(argv) != 2 || !strings.Contains(argv[1], "fake claude request 1") {
		t.Fatalf("shell pane with trail: %.300q", argv)
	}
}
