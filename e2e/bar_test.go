//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func tmuxVersion(t *testing.T, h *H) tmux.Version {
	t.Helper()
	v, ok := tmux.ParseVersion(h.mustTmux("-V"))
	if !ok {
		t.Fatal("unparseable tmux -V")
	}
	return v
}

// TestAgentBar (T2.8a): the second status line shows the agents, highlights
// the active pane's agent with tmux formats alone, dims missing agents,
// shows limit markers and drops missing agents at 80 columns.
func TestAgentBar(t *testing.T) {
	h := newH(t, "[agents.copilot]\nbin = \"/nonexistent/copilot\"\n")
	pane := h.startSession("claude")
	h.run("claude", 1)
	second := strings.TrimSpace(h.mustImux("split", "antigravity", "-h", "--pane", pane))
	h.run("antigravity", 1)
	h.mustTmux("select-pane", "-t", pane)
	if got := strings.TrimSpace(h.mustTmux("show-options", "-gv", "status")); got != "2" {
		t.Fatalf("status = %q, want 2", got)
	}
	clickable := tmuxVersion(t, h).AtLeast(tmux.ClickMinVersion[0], tmux.ClickMinVersion[1])
	keys := h.mustTmux("list-keys", "-T", "root")
	if strings.Contains(keys, "MouseDown1Status") && strings.Contains(keys, "__click") != clickable {
		t.Fatalf("MouseDown1Status binding (clickable=%v):\n%s", clickable, keys)
	}

	c, _ := h.attach(160, 40)
	c.waitScreen(h, "▶ Claude", 5*time.Second)
	for _, want := range []string{" i-mux ", " Antigravity ", " Codex ", "Copilot (not installed)", "⧉ Sessions", "☰"} {
		c.waitScreen(h, want, 2*time.Second)
	}

	// Focus change: the bar follows, and no imux process runs.
	before := h.uiRecords()
	mark := c.Mark()
	h.mustTmux("select-pane", "-t", second)
	c.waitScreenAfter(h, mark, "▶ Antigravity", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	if after := h.uiRecords(); after != before {
		t.Fatalf("focus change ran imux (%d new tmux invocations)", after-before)
	}

	// Limit markers from the watcher's global options.
	mark = c.Mark()
	h.mustTmux("set-option", "-g", "@imux_lim_codex", "⛔15:00")
	h.mustTmux("refresh-client", "-S")
	c.waitScreenAfter(h, mark, "Codex ⛔15:00", 5*time.Second)
	h.mustTmux("set-option", "-gu", "@imux_lim_codex")

	// 80 columns: entries of missing agents are dropped (Q29).
	c.Close()
	narrow, _ := h.attach(80, 24)
	narrow.waitScreen(h, " Antigravity ", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(narrow.Text(), "not installed") {
		t.Fatal("80 columns still shows the missing agent")
	}
}

// TestAgentBarOff: tmux.agent_bar = false gives one status line and no
// status click binding.
func TestAgentBarOff(t *testing.T) {
	h := newH(t, "[tmux]\nagent_bar = false\nsidebar = false\ntabs = false\n")
	h.startSession("claude")
	if got := strings.TrimSpace(h.mustTmux("show-options", "-gv", "status")); got != "on" {
		t.Fatalf("status = %q, want on", got)
	}
	if keys := h.mustTmux("list-keys", "-T", "root"); strings.Contains(keys, "__click") {
		t.Fatalf("click binding present:\n%s", keys)
	}
}
