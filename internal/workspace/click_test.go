package workspace

import (
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestClassifyClick(t *testing.T) {
	w, _, _ := testWS(t)
	now := testutil.FrozenTime
	w.st = &state.State{Limits: map[string]*state.Limit{"gemini": {At: now, ExpiresAt: now.Add(time.Hour)}}}
	p := &Pane{ID: "%1", Agent: "claude"}
	cases := []struct {
		rng    string
		pane   *Pane
		action ClickAction
		agent  session.AgentID
	}{
		{"ajan:cur", p, ClickMenu, ""},
		{"ajan:menu", p, ClickMenu, ""},
		{"ajan:sessions", p, ClickSessions, ""},
		{"ajan:claude", p, ClickMenu, "claude"}, // the current agent
		{"ajan:gemini", p, ClickConfirm, "gemini"},
		{"ajan:codex", p, ClickInstall, "codex"},
		{"ajan:copilot", p, ClickInstall, "copilot"},
		{"ajan:claude", &Pane{ID: "%2"}, ClickSwitch, "claude"},                              // plain shell
		{"ajan:claude", &Pane{ID: "%3", Agent: "claude", Dead: true}, ClickSwitch, "claude"}, // exited agent
		{"ajan:nope", p, ClickNone, ""},
		{"window:1", p, ClickNone, ""},
	}
	for _, c := range cases {
		a, id := w.Classify(c.rng, c.pane)
		if a != c.action || id != c.agent {
			t.Errorf("%s on %+v: got %v %q, want %v %q", c.rng, c.pane, a, id, c.action, c.agent)
		}
	}
	m := w.ConfirmMenu("gemini", p, "c")
	if m.Items[0].Key != "y" || m.Items[1].Cmd != "" || m.Title == "" {
		t.Fatalf("confirm menu %+v", m)
	}
}
