//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/watch"
)

// TestAutoApprove (D112): with on_permission = "approve" the watcher answers
// a permission prompt at the bottom of an agent's screen with the CLI's own
// key — here Claude Code's "1" — and counts it on the pane. The same
// question back on the screen is a new one and is answered again; one that
// stays is left to the user, its key never sent twice.
//
// A real CLI reads its keys with the terminal in raw mode; the fake agent
// puts its pty in that mode too (stty -icanon), so the digit the watcher
// sends — without a newline — reaches it at once.
func TestAutoApprove(t *testing.T) {
	t.Setenv("IMUX_APPROVE_GIVEUP", "2s")
	h := newH(t, "on_permission = \"approve\"\n"+fastLimits)
	raw := "stty -icanon -echo min 1 time 0\n"
	prompt := "printf 'Do you want to proceed?\\n" +
		"❯ 1. Yes\\n" +
		"  2. Yes, and do not ask again for this session\\n" +
		"  3. No, and tell Claude what to do differently (esc)\\n'\n"
	h.control("claude", raw+prompt+
		"ch=$(head -c 1); printf 'first key: %s\\n' \"$ch\"; printf '%s' \"$ch\" >\"$log/$agent-$n.first\"\n"+
		prompt+
		"ch=$(head -c 1); printf 'second key: %s\\n' \"$ch\"; printf '%s' \"$ch\" >\"$log/$agent-$n.second\"\n"+
		// The third is never read: it stays, and its key is sent once.
		prompt+
		"sleep 60\n")
	pane := h.startSession("claude")
	h.attach(120, 40)
	h.run("claude", 1)
	key := func(n string) string {
		return h.readFile(filepath.Join(h.Log, "claude-1."+n))
	}
	h.waitFor("the watcher answers the first prompt", 10*time.Second, func() bool {
		return key("first") == "1"
	})
	h.waitFor("the question back on the screen is answered again", 10*time.Second, func() bool {
		return key("second") == "1"
	})
	h.waitFor("the count on the pane", 5*time.Second, func() bool {
		return h.paneOpt(pane, "#{@imux_approved}") == "3"
	})
	// The third prompt stays: its key was sent once and never again.
	time.Sleep(watch.DefaultGiveUp)
	if k := key("first") + key("second"); k != "11" {
		t.Fatalf("keys read by the agent: %q", k)
	}
	if n := h.paneOpt(pane, "#{@imux_approved}"); n != "3" {
		t.Fatalf("the count after a prompt that stayed: %q", n)
	}
	_, pid := watch.Status()
	sends := 0
	for _, r := range h.records() {
		a := " " + strings.Join(r.Args, " ") + " "
		if r.Pid == pid && strings.Contains(a, " send-keys -t "+pane+" 1 ") {
			sends++
		}
	}
	if sends != 3 {
		t.Fatalf("the watcher sent the key %d times, want 3", sends)
	}
}

// Without the setting nothing is ever answered (D112): the prompt is the
// user's.
func TestAutoApproveOff(t *testing.T) {
	h := newH(t, fastLimits)
	h.control("claude", "stty -icanon -echo min 1 time 0\n"+
		"printf 'Do you want to proceed?\\n❯ 1. Yes\\n  2. No\\n'\nsleep 60\n")
	pane := h.startSession("claude")
	h.attach(120, 40)
	h.run("claude", 1)
	time.Sleep(2 * time.Second)
	if n := h.paneOpt(pane, "#{@imux_approved}"); n != "" {
		t.Fatalf("an off watcher counted %q", n)
	}
	_, pid := watch.Status()
	for _, r := range h.records() {
		a := " " + strings.Join(r.Args, " ") + " "
		if r.Pid == pid && strings.Contains(a, " send-keys -t "+pane+" ") {
			t.Fatal("an off watcher answered a prompt")
		}
	}
}
