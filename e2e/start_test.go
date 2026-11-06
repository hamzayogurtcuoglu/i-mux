//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// TestStartAttachLs (T2.4): `imux claude` creates the project's session with
// the pane options set and a pre-assigned Claude session id; a second `imux`
// attaches to it; inside the workspace `imux gemini` opens a new window.
func TestStartAttachLs(t *testing.T) {
	h := newH(t, "")
	before := time.Now().UnixMilli()
	out := h.mustImux("claude", "-d")
	name := workspace.SessionName(h.Homes.Project)
	if !strings.Contains(out, name) {
		t.Fatalf("start output %q lacks session %s", out, name)
	}
	pane := strings.TrimSpace(h.mustTmux("list-panes", "-t", "="+name, "-F", "#{pane_id}"))
	argv := h.run("claude", 1)

	if got := h.paneOpt(pane, "#{@imux_agent}|#{@imux_cwd}|#{@imux_label}|#{remain-on-exit}"); got != "claude|"+h.Homes.Project+"|Claude|on" {
		t.Fatalf("pane options: %q", got)
	}
	start, _ := strconv.ParseInt(h.paneOpt(pane, "#{@imux_start}"), 10, 64)
	if start < before || start > time.Now().UnixMilli() {
		t.Fatalf("@imux_start %d not in [%d, now]", start, before)
	}
	trail := workspace.ParseTrail(h.paneOpt(pane, "#{@imux_trail}"))
	if len(trail) != 1 || trail[0].Agent != "claude" || trail[0].SessionID == "" {
		t.Fatalf("trail %+v", trail)
	}
	if len(argv) != 2 || argv[0] != "--session-id" || argv[1] != trail[0].SessionID {
		t.Fatalf("fake claude argv %q, want --session-id %s", argv, trail[0].SessionID)
	}
	env := h.runEnv("claude", 1)
	if !strings.HasPrefix(env["PATH"], h.Fakes+":") {
		t.Fatalf("PATH does not start with the agent's dir: %q", env["PATH"])
	}
	if env["IMUX_PANE"] != pane || env["IMUX_AGENT"] != "claude" {
		t.Fatalf("env %v", env)
	}

	// A second `imux` in the same directory attaches instead of creating.
	out = h.mustImux("-d")
	if !strings.Contains(out, "already exists") {
		t.Fatalf("second start: %q", out)
	}
	c := h.startClient(120, 40, h.Homes.Project, h.Bin)
	h.waitFor("client attached", 5*time.Second, func() bool {
		out, _ := h.tmux("list-clients", "-F", "#{client_session}")
		return strings.TrimSpace(out) == name
	})
	if n := strings.Count(h.mustTmux("list-sessions", "-F", "#{session_name}"), "\n"); n != 1 {
		t.Fatalf("%d sessions, want 1", n)
	}
	if h.runs("claude") != 1 {
		t.Fatal("attaching started another agent")
	}
	_ = c

	// Inside imux: `imux gemini` opens a new window with gemini.
	sock := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", pane, "#{socket_path}"))
	inside := []string{"TMUX=" + sock + ",1,0", "TMUX_PANE=" + pane}
	if o, e, err := h.imuxEnv(inside, "gemini"); err != nil {
		t.Fatalf("imux gemini inside: %v\n%s%s", err, o, e)
	}
	h.run("gemini", 1)
	wins := strings.Fields(h.mustTmux("list-windows", "-t", "="+name, "-F", "#{window_index}:#{pane_id}"))
	if len(wins) != 2 {
		t.Fatalf("windows %q, want 2", wins)
	}
	gp := strings.SplitN(wins[1], ":", 2)[1]
	if got := h.paneOpt(gp, "#{@imux_agent}|#{@imux_label}"); got != "gemini|Gemini" {
		t.Fatalf("new window pane: %q", got)
	}

	// imux ls shows both panes.
	ls := h.mustImux("ls")
	for _, want := range []string{name, pane, gp, "claude", "gemini", "attached"} {
		if !strings.Contains(ls, want) {
			t.Errorf("ls lacks %q:\n%s", want, ls)
		}
	}
	if got := strings.TrimSpace(h.mustImux("ls", "--names")); got != name {
		t.Fatalf("ls --names: %q", got)
	}
}

// TestStartUnknownOrMissingAgent: errors are reported, nothing is started.
func TestStartUnknownOrMissingAgent(t *testing.T) {
	h := newH(t, "[agents.copilot]\nbin = \"/nonexistent/copilot\"\n")
	if _, e, err := h.imux("copilot", "-d"); err == nil || !strings.Contains(e, "not installed") {
		t.Fatalf("missing agent: %v %q", err, e)
	}
	if _, e, err := h.imux("frobnicate", "-d"); err == nil || !strings.Contains(e, "unknown command") {
		t.Fatalf("unknown agent: %v %q", err, e)
	}
	if out, _ := h.tmux("list-sessions"); strings.Contains(out, "proj") {
		t.Fatalf("a session was created: %s", out)
	}
}
