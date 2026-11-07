//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestUnknownAgentPane: a pane left by a version that knew an agent this one
// does not (0.18 had OpenCode) still shows in the sidebar, and switching it
// to a known agent works.
func TestUnknownAgentPane(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	h.mustTmux("set-option", "-p", "-t", agentPane, "@imux_agent", "opencode", ";",
		"set-option", "-p", "-t", agentPane, "@imux_label", "OpenCode", ";",
		"set-option", "-p", "-t", agentPane, "@imux_trail", `[{"a":"opencode","s":1794089033463,"id":"ses_imux0123"}]`)
	h.mustImux("__sidebar-refresh")
	h.waitFor("sidebar lists the pane", 5*time.Second, func() bool {
		return strings.Contains(strings.ToLower(h.screen(bar)), "opencode")
	})

	h.mustImux("switch", "codex", "--pane", agentPane)
	h.run("codex", 1)
	if a := h.paneOpt(agentPane, "#{@imux_agent}"); a != "codex" {
		t.Fatalf("pane agent after switch %q", a)
	}
	h.waitFor("sidebar shows Codex", 5*time.Second, func() bool {
		return strings.Contains(h.screen(bar), "Codex")
	})
}
