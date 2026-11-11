package workspace

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestClassifyClick(t *testing.T) {
	w, _, _ := testWS(t)
	now := testutil.FrozenTime
	w.st = &state.State{Limits: map[string]*state.Limit{"antigravity": {At: now, ExpiresAt: now.Add(time.Hour)}}}
	p := &Pane{ID: "%1", Agent: "claude"}
	cases := []struct {
		rng    string
		pane   *Pane
		action ClickAction
		agent  session.AgentID
	}{
		{"imux:cur", p, ClickMenu, ""},
		{"imux:menu", p, ClickMenu, ""},
		{"imux:sessions", p, ClickSessions, ""},
		{"imux:claude", p, ClickMenu, "claude"}, // the current agent
		{"imux:agy", p, ClickConfirm, "antigravity"},
		{"imux:codex", p, ClickInstall, "codex"},
		{"imux:copilot", p, ClickInstall, "copilot"},
		{"imux:claude", &Pane{ID: "%2"}, ClickSwitch, "claude"},                              // plain shell
		{"imux:claude", &Pane{ID: "%3", Agent: "claude", Dead: true}, ClickSwitch, "claude"}, // exited agent
		{"imux:quit", p, ClickQuit, ""},
		{"imux:nope", p, ClickNone, ""},
		{"window:1", p, ClickNone, ""},
	}
	for _, c := range cases {
		a, id := w.Classify(c.rng, c.pane)
		if a != c.action || id != c.agent {
			t.Errorf("%s on %+v: got %v %q, want %v %q", c.rng, c.pane, a, id, c.action, c.agent)
		}
	}
	m := w.ConfirmMenu("antigravity", p, "c")
	if m.Items[0].Key != "y" || m.Items[1].Cmd != "" || m.Title == "" {
		t.Fatalf("confirm menu %+v", m)
	}
}

// TestQuitMenus (D48): ⏻ Exit offers going back to the terminal (this
// client detaches) or quitting, which asks first with the agents' count.
func TestQuitMenus(t *testing.T) {
	i18n.SetLocale("en")
	w, f, _ := testWS(t)
	m := w.QuitMenu(&Pane{ID: "%1"}, "/dev/pts/3")
	if m.Title != "Leave i-mux" || len(m.Items) != 3 || m.Items[0].Key != "b" || m.Items[0].Cmd != "detach-client -t /dev/pts/3" ||
		m.Items[2].Key != "q" || !strings.Contains(m.Items[2].Cmd, "quit --menu --pane %1 --client /dev/pts/3") {
		t.Fatalf("menu %+v", m)
	}
	f.replies["list-panes"] = "claude\t0\ncodex\t0\n\t0\nclaude\t1\n" // two agents, a shell, an exited agent
	c := w.QuitConfirmMenu(context.Background(), "%1", "c1")
	if c.Title != "Stop 2 agents and close i-mux?" || c.Items[0].Key != "y" || !strings.Contains(c.Items[0].Cmd, "quit --yes --pane %1 --client c1") || c.Items[1].Cmd != "" {
		t.Fatalf("confirm %+v", c)
	}
}
