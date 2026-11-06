//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// TestSplit (T2.9): -h / -v, same cwd, pane options set, label in the border.
func TestSplit(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	for i, c := range []struct {
		flag, agent, layoutOp string
	}{{"-h", "antigravity", "left"}, {"-v", "codex", "top"}} {
		out := h.mustImux("split", c.agent, c.flag, "--pane", pane)
		np := strings.TrimSpace(out)
		h.run(c.agent, 1)
		got := h.paneOpt(np, "#{@imux_agent}|#{@imux_cwd}|#{remain-on-exit}|#{pane_current_path}")
		want := c.agent + "|" + h.Homes.Project + "|on|" + h.Homes.Project
		if got != want {
			t.Fatalf("split %d: %q, want %q", i, got, want)
		}
		border := h.paneOpt(np, "#{E:pane-border-format}")
		if label := map[string]string{"antigravity": "Antigravity", "codex": "Codex"}[c.agent]; !strings.Contains(border, " "+label+" ") {
			t.Fatalf("border %q lacks label %q", border, label)
		}
	}
	// -h places the antigravity pane beside claude, -v the codex pane below it.
	layout := h.mustTmux("list-panes", "-t", pane, "-F", "#{pane_id} #{pane_left} #{pane_top} #{@imux_agent}")
	rows := map[string][2]string{}
	for _, l := range strings.Split(strings.TrimSpace(layout), "\n") {
		f := strings.Fields(l)
		rows[f[3]] = [2]string{f[1], f[2]}
	}
	if rows["antigravity"][0] == rows["claude"][0] || rows["antigravity"][1] != rows["claude"][1] {
		t.Errorf("-h did not split side by side: %v", rows)
	}
	if rows["codex"][1] == rows["claude"][1] {
		t.Errorf("-v did not split vertically: %v", rows)
	}
	if _, e, err := h.imux("split", "antigravity", "-h", "-v", "--pane", pane); err == nil || !strings.Contains(e, "usage") {
		t.Fatalf("-h -v together: %v %q", err, e)
	}
}
