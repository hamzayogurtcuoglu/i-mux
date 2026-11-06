//go:build e2e

package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stdinOf returns what fake agent run n read from its terminal.
func (h *H) stdinOf(agent string, n int) string {
	b, _ := os.ReadFile(filepath.Join(h.Log, agent+"-"+itoa(n)+".stdin"))
	return string(b)
}

// TestBusExchange (T5.6): two fake agents exchange messages through the bus;
// a multi-line message arrives intact; wait reports idle and times out on a
// busy pane; the rate limit stops a ping-pong loop.
func TestBusExchange(t *testing.T) {
	h := newH(t, "[bus]\nrate_per_minute = 4\nidle_after = \"1s\"\n")
	h.control("codex", "while :; do printf '\\r%s' \"$(date +%N)\"; sleep 0.1; done\n") // a busy pane
	a := h.startSession("claude")
	h.run("claude", 1)
	b := strings.TrimSpace(h.mustImux("split", "antigravity", "-h", "--pane", a))
	h.run("antigravity", 1)
	busy := strings.TrimSpace(h.mustImux("split", "codex", "-v", "--pane", a))
	h.run("codex", 1)
	fromA := append(h.inside(a), "IMUX_AGENT=claude")
	fromB := append(h.inside(b), "IMUX_AGENT=antigravity")

	msg := "please review src/a.ts\nfocus on the error paths\nthanks"
	if out, e, err := h.imuxEnv(fromA, "send", "antigravity", msg); err != nil || !strings.Contains(out, "sent to "+b) {
		t.Fatalf("send: %v %q %q", err, out, e)
	}
	want := "[from claude " + a + "] please review src/a.ts\nfocus on the error paths\nthanks\n"
	h.waitFor("message at antigravity", 5*time.Second, func() bool { return strings.HasPrefix(h.stdinOf("antigravity", 1), want) })

	if _, e, err := h.imuxEnv(fromB, "send", "claude", "-"); err == nil {
		t.Fatalf("empty stdin message accepted: %q", e)
	}
	cmd := exec.Command(h.Bin, "send", "claude", "-")
	cmd.Dir = h.Homes.Project
	cmd.Env = append(os.Environ(), fromB...)
	cmd.Stdin = strings.NewReader("ack: looking at it")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("send from stdin: %v %s", err, out)
	}
	h.waitFor("reply at claude", 5*time.Second, func() bool {
		return strings.Contains(h.stdinOf("claude", 1), "[from antigravity "+b+"] ack: looking at it")
	})

	// wait: idle → 0 (with --print), busy → 124.
	out, _, err := h.imuxEnv(fromA, "wait", "antigravity", "--timeout", "10", "--print")
	if err != nil || !strings.Contains(out, "fake antigravity ready") {
		t.Fatalf("wait idle: %v %q", err, out)
	}
	_, _, err = h.imuxEnv(fromA, "wait", busy, "--timeout", "2")
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 124 {
		t.Fatalf("wait on a busy pane: %v", err)
	}

	// Ping-pong: the sender's rate limit (4/min; 2 sent above by these
	// panes) stops the loop.
	var sent, refused int
	for i := 0; i < 6; i++ {
		_, e, err := h.imuxEnv(fromA, "send", b, "ping "+itoa(i))
		switch {
		case err == nil:
			sent++
		case strings.Contains(e, "rate_per_minute"):
			refused++
		default:
			t.Fatalf("send %d: %v %s", i, err, e)
		}
	}
	if sent != 3 || refused != 3 {
		t.Fatalf("sent %d, refused %d; want 3 and 3", sent, refused)
	}
	if _, e, err := h.imuxEnv(fromA, "send", a, "me"); err == nil || !strings.Contains(e, "this pane") {
		t.Fatalf("self send: %v %q", err, e)
	}
}

// TestBusConfirm: with bus.confirm the user approves delivery in a menu.
func TestBusConfirm(t *testing.T) {
	h := newH(t, "[bus]\nconfirm = true\n")
	a := h.startSession("claude")
	h.run("claude", 1)
	b := strings.TrimSpace(h.mustImux("split", "antigravity", "-h", "--pane", a))
	h.run("antigravity", 1)
	c, _ := h.attach(120, 40)
	out, _, err := h.imuxEnv(append(h.inside(a), "IMUX_AGENT=claude"), "send", "antigravity", "run the tests")
	if err != nil || !strings.Contains(out, "waiting for the user") {
		t.Fatalf("send: %v %q", err, out)
	}
	c.waitScreen(h, "From claude "+a+" to antigravity "+b+":", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	if h.stdinOf("antigravity", 1) != "" {
		t.Fatal("delivered before confirmation")
	}
	c.Write("y")
	h.waitFor("delivery after confirmation", 5*time.Second, func() bool {
		return strings.Contains(h.stdinOf("antigravity", 1), "[from claude "+a+"] run the tests")
	})
	if m, _ := filepath.Glob(filepath.Join(h.Homes.Imux, "bus", "*.txt")); len(m) != 0 {
		t.Fatalf("parked messages left: %v", m)
	}
}
