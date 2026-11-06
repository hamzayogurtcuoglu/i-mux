//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestBusReadAndPanes (T5.1, T5.2): an agent lists the panes and reads
// another pane's screen by agent name.
func TestBusReadAndPanes(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	h.run("claude", 1)
	g := strings.TrimSpace(h.mustAjan("split", "gemini", "--pane", pane))
	h.run("gemini", 1)
	h.waitFor("gemini screen", 5*time.Second, func() bool { return strings.Contains(h.capture(g), "fake gemini ready") })
	inside := h.inside(pane)
	out, errOut, err := h.ajanEnv(inside, "read", "gemini", "-n", "5")
	if err != nil || !strings.Contains(out, "fake gemini ready (run 1, pane "+g+")") {
		t.Fatalf("read: %v %q %s", err, out, errOut)
	}
	out, _, err = h.ajanEnv(inside, "panes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var ps []struct {
		Pane, Agent, State string
	}
	if err := json.Unmarshal([]byte(out), &ps); err != nil || len(ps) != 2 {
		t.Fatalf("panes --json: %v %s", err, out)
	}
	for _, p := range ps {
		if p.State != "idle" && p.State != "busy" {
			t.Fatalf("state %q", p.State)
		}
	}
	if _, e, err := h.ajanEnv(inside, "read", "codex"); err == nil || !strings.Contains(e, "no pane runs codex") {
		t.Fatalf("missing target: %v %q", err, e)
	}
}
