//go:build e2e

package e2e

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// TestPaneMenu (T2.8): `imux __menu` shows the pane menu on the client with
// one display-menu; "Close pane" works; opening is fast (§17).
func TestPaneMenu(t *testing.T) {
	h := newH(t, "[agents.copilot]\nbin = \"/nonexistent/copilot\"\n")
	pane := h.startSession("claude")
	h.run("claude", 1)
	second := strings.TrimSpace(h.mustImux("split", "antigravity", "-h", "--pane", pane))
	h.run("antigravity", 1)
	c, client := h.attach(120, 40)

	h.mustImux("__menu", "--pane", second, "--client", client)
	c.waitScreen(h, "Switch to Claude (with context)", 5*time.Second)
	for _, want := range []string{"i-mux · Antigravity", "Switch to Codex (with context)", "Copilot (with context) (not installed)", "Sessions…", "Close pane"} {
		c.waitScreen(h, want, 2*time.Second)
	}
	shows := h.menuShows()
	for _, r := range shows {
		a := strings.Join(r.Args, " ")
		menu := strings.Contains(a, "-c "+client+" -t "+second+" -T i-mux · Antigravity -x C -y C")
		popup := strings.Contains(a, "-c "+client+" -t "+second+" -E") && strings.Contains(a, "-x C -y C")
		if !menu && !popup {
			t.Errorf("menu args %q", r.Args)
		}
	}
	if len(shows) != 1 {
		t.Fatalf("%d menus shown, want 1", len(shows))
	}

	// Choosing "Close pane" with its mnemonic kills that pane only.
	c.Write("k")
	h.waitFor("pane closed", 5*time.Second, func() bool {
		out, _ := h.tmux("list-panes", "-a", "-F", "#{pane_id}")
		return !strings.Contains(out, second+"\n") && strings.Contains(out, pane+"\n")
	})

	// Latency: process start → menu shown, 50 runs.
	var ds []time.Duration
	for i := 0; i < 50; i++ {
		start := time.Now()
		h.mustImux("__menu", "--pane", pane, "--client", client)
		ds = append(ds, time.Since(start))
		c.Write("q") // close the menu
		time.Sleep(20 * time.Millisecond)
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	p95 := ds[len(ds)*95/100-1]
	t.Logf("menu open latency: p50 %v, p95 %v, max %v", ds[len(ds)/2], p95, ds[len(ds)-1])
	if p95 > 100*time.Millisecond {
		t.Errorf("menu p95 %v exceeds 100 ms", p95)
	}
}
