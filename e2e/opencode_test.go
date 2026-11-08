//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestOpenCodeRoundTrip (T25.4): Claude → OpenCode starts OpenCode with a
// session id imux chose and the handoff; back to Claude resumes Claude's own
// session with what happened in OpenCode (read through `opencode session
// export`).
func TestOpenCodeRoundTrip(t *testing.T) {
	h := newH(t, "")
	pane := h.startSession("claude")
	argv := h.run("claude", 1)
	claudeID := ""
	for i, a := range argv {
		if a == "--session-id" && i+1 < len(argv) {
			claudeID = argv[i+1]
		}
	}

	h.mustImux("switch", "opencode", "--pane", pane)
	argv = h.run("opencode", 1)
	if len(argv) != 4 || argv[0] != "--session" || !strings.HasPrefix(argv[1], "ses_imux") || argv[2] != "--prompt" ||
		!strings.HasPrefix(argv[3], "[i-mux handoff · Claude Code → OpenCode") || !strings.Contains(argv[3], "fake claude request 1") {
		t.Fatalf("opencode argv %.300q", argv)
	}
	if got := h.paneOpt(pane, "#{@imux_agent}|#{@imux_label}"); got != "opencode|OpenCode" {
		t.Fatalf("pane %q", got)
	}

	h.mustImux("switch", "claude", "--pane", pane)
	argv = h.run("claude", 2)
	if len(argv) != 3 || argv[0] != "--resume" || argv[1] != claudeID || !strings.Contains(argv[2], "Welcome back") ||
		!strings.Contains(argv[2], "fake opencode reply 1") || !strings.Contains(argv[2], "$ fake test 1") {
		t.Fatalf("claude resume argv %.600q", argv)
	}
}

var handoffFileRe = regexp.MustCompile(`/[^\s"']*/handoffs/[^\s"']+\.md`)

// delivered is everything run n of agent was given: its argv, what was pasted
// into it, and the handoff files those name.
func (h *H) delivered(agent string, n int) string {
	h.t.Helper()
	s := strings.Join(h.run(agent, n), "\n") + "\n" + h.stdinOf(agent, n)
	for _, p := range handoffFileRe.FindAllString(s, -1) {
		if b, err := os.ReadFile(p); err == nil {
			s += "\n" + string(b)
		}
	}
	return s
}

// waitDelivered waits until run n of agent was given every one of want.
func (h *H) waitDelivered(agent string, n int, want ...string) string {
	h.t.Helper()
	var got string
	h.waitFor(agent+" given the context", 30*time.Second, func() bool {
		got = h.delivered(agent, n)
		for _, w := range want {
			if !strings.Contains(got, w) {
				return false
			}
		}
		return true
	})
	return got
}

// TestOpenCodeSwitches (T25.4): each other agent hands its chat to OpenCode
// and takes it back with what OpenCode did; a second switch to OpenCode
// resumes OpenCode's own session with what happened in between.
func TestOpenCodeSwitches(t *testing.T) {
	for _, src := range []string{"antigravity", "copilot", "codex"} {
		t.Run(src, func(t *testing.T) {
			h := newH(t, "")
			pane := h.startSession(src)
			h.run(src, 1)

			h.mustImux("switch", "opencode", "--pane", pane)
			argv := h.run("opencode", 1)
			name := session.DisplayName(session.AgentID(src))
			if len(argv) != 4 || argv[0] != "--session" || !strings.HasPrefix(argv[1], "ses_imux") || argv[2] != "--prompt" ||
				!strings.HasPrefix(argv[3], "[i-mux handoff · "+name+" → OpenCode") {
				t.Fatalf("opencode argv %.300q", argv)
			}
			h.waitDelivered("opencode", 1, "fake "+src+" request 1", "fake "+src+" reply 1")
			ocID := argv[1]

			h.mustImux("switch", src, "--pane", pane)
			h.waitDelivered(src, 2, "fake opencode reply 1", "$ fake test 1")
			if got := h.paneOpt(pane, "#{@imux_agent}"); got != src {
				t.Fatalf("pane agent %q", got)
			}

			h.mustImux("switch", "opencode", "--pane", pane)
			argv = h.run("opencode", 2)
			if len(argv) != 4 || argv[0] != "--session" || argv[1] != ocID || argv[2] != "--prompt" {
				t.Fatalf("opencode resume argv %.300q", argv)
			}
			h.waitDelivered("opencode", 2, "fake "+src+" reply 2")
		})
	}
}

// TestOpenCodeChain (T25.4): Claude → OpenCode → Codex keeps the original
// request and adds what OpenCode did; an OpenCode session hands off from the
// picker too (`--session opencode:<id>`).
func TestOpenCodeChain(t *testing.T) {
	h := newH(t, "")
	// Like the real OpenCode: the handoff it is given is its first message.
	testutil.WriteFile(t, filepath.Join(h.Log, "opencode.handoff-as-request"), nil)
	pane := h.startSession("claude")
	h.run("claude", 1)
	h.mustImux("switch", "opencode", "--pane", pane)
	ocID := h.run("opencode", 1)[1]

	h.mustImux("switch", "codex", "--pane", pane)
	argv := h.run("codex", 1)
	if len(argv) != 1 || !strings.HasPrefix(argv[0], "[i-mux handoff · OpenCode → Codex CLI") {
		t.Fatalf("codex argv %.300q", argv)
	}
	h.waitDelivered("codex", 1, "fake claude request 1", "fake opencode reply 1", "$ fake test 1")

	doc := h.mustImux("handoff", "--session", "opencode:"+ocID, "--cwd", h.Homes.Project, "--print")
	for _, want := range []string{"[i-mux handoff · OpenCode", "fake claude request 1", "fake opencode reply 1"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("handoff from the OpenCode session lacks %q:\n%.800s", want, doc)
		}
	}
}
