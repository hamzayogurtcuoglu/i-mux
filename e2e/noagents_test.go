//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestNoAgents (T37.2): closing the project's last agent keeps the project:
// its pane says "No agents" and lists the agents, the sidebar says so too; a
// key starts an agent in that pane; a pane that goes another way (killed)
// leaves "No agents" as well; the last agent of another tab still takes its
// tab with it.
func TestNoAgents(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	session := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agentPane, "#{session_name}"))
	alive := func() {
		t.Helper()
		if _, err := h.tmux("has-session", "-t", "="+session); err != nil {
			t.Fatal("the project closed")
		}
	}
	roles := func() string {
		out, _ := h.tmux("list-panes", "-s", "-t", "="+session, "-F", "#{@imux_role}")
		return strings.Join(strings.Fields(out), " ")
	}

	// The agent menu's Close agent (confirmed) on the only agent.
	mark := c.Mark()
	h.mustImux("__close-pane", "--pane", agentPane)
	h.waitFor("the No agents pane", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_role}") == "empty" })
	alive()
	c.waitScreenAfter(h, mark, "No agents in proj", 5*time.Second)
	c.waitScreenAfter(h, mark, "Claude", 5*time.Second)
	h.waitFor("the sidebar says so", 5*time.Second, func() bool { return strings.Contains(h.screen(bar), "No agents · + to start one") })
	if a := h.paneOpt(agentPane, "#{@imux_agent}"); a != "" {
		t.Fatalf("@imux_agent %q in the No agents pane", a)
	}
	if strings.Contains(h.screen(bar), "Continue this chat in") {
		t.Error("the sidebar offers to continue a chat that is gone")
	}

	// Its key starts Claude in that same pane.
	time.Sleep(300 * time.Millisecond)
	c.Write("c")
	h.run("claude", 2)
	h.waitFor("Claude in the pane", 5*time.Second, func() bool {
		return h.paneOpt(agentPane, "#{@imux_agent}") == "claude" && h.paneOpt(agentPane, "#{@imux_role}") == ""
	})
	h.waitFor("the sidebar lists it", 5*time.Second, func() bool { return strings.Contains(h.screen(bar), "✻  Claude") })
	if r := roles(); r != "sidebar" {
		t.Fatalf("panes %q: the agent should take the No agents pane's place", r)
	}

	// A pane that goes another way leaves "No agents" too.
	h.mustTmux("kill-pane", "-t", agentPane)
	h.waitFor("No agents again", 10*time.Second, func() bool { return roles() == "sidebar empty" })
	alive()
	var empty string
	for _, l := range strings.Split(h.mustTmux("list-panes", "-s", "-t", "="+session, "-F", "#{pane_id} #{@imux_role}"), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[1] == "empty" {
			empty = f[0]
		}
	}
	// The sidebar keeps its width beside it.
	h.waitFor("sidebar width", 5*time.Second, func() bool { return h.paneOpt(bar, "#{pane_width}") == "43" })

	// The + of AGENTS (imux split on the target) also starts in its place.
	h.mustImux("split", "codex", "-h", "--pane", bar)
	h.run("codex", 1)
	h.waitFor("Codex in the pane", 5*time.Second, func() bool { return h.paneOpt(empty, "#{@imux_agent}") == "codex" })
	if r := roles(); r != "sidebar" {
		t.Fatalf("panes %q after + of AGENTS", r)
	}

	// The last agent of another tab takes its tab with it.
	h.mustImux("tab", "copilot", "--pane", empty)
	h.run("copilot", 1)
	var copilot string
	h.waitFor("the copilot tab", 10*time.Second, func() bool {
		for _, l := range strings.Split(h.mustTmux("list-panes", "-s", "-t", "="+session, "-F", "#{pane_id} #{@imux_agent}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[1] == "copilot" {
				copilot = f[0]
			}
		}
		return copilot != ""
	})
	h.mustImux("__close-pane", "--pane", copilot)
	h.waitFor("the copilot tab closed", 5*time.Second, func() bool {
		return strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", "="+session+":", "#{session_windows}")) == "1"
	})
	alive()
}
