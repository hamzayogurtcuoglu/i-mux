package watch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

type recRunner struct{ calls [][]string }

func (r *recRunner) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	r.calls = append(r.calls, args)
	return nil, nil, nil
}

func (r *recRunner) Spawn(_ string, args []string) error {
	r.calls = append(r.calls, args)
	return nil
}

func (r *recRunner) find(cmd string) []string {
	for i := len(r.calls) - 1; i >= 0; i-- {
		for _, a := range r.calls[i] {
			if a == cmd {
				return r.calls[i]
			}
		}
	}
	return nil
}

type spawned struct{ args [][]string }

func newActor(t *testing.T, mode string) (*Actor, *recRunner, *spawned) {
	t.Helper()
	testutil.Isolate(t)
	loc := time.Local
	time.Local = time.UTC // reset times below are rendered in local time
	t.Cleanup(func() { time.Local = loc })
	cfg := config.Default()
	cfg.OnLimit = mode
	r := &recRunner{}
	sp := &spawned{}
	w := &workspace.WS{Cfg: cfg, T: &tmux.Client{Bin: "tmux", Socket: "t", Runner: r}, Self: "/opt/imux", Now: testutil.Clock().Now}
	bins := map[session.AgentID]agent.Resolved{
		"claude": {Agent: "claude", Installed: true, Path: "/x/claude"},
		"gemini": {Agent: "gemini", Installed: true, Path: "/x/gemini"},
		"codex":  {Agent: "codex", Installed: true, Path: "/x/codex"},
	}
	a := &Actor{W: w, Spawn: func(self string, args ...string) error {
		sp.args = append(sp.args, append([]string{self}, args...))
		return nil
	}}
	w.SetBins(bins)
	resolve = func(*workspace.WS) {}
	t.Cleanup(func() { resolve = func(*workspace.WS) {} })
	return a, r, sp
}

func limitEvent() Event {
	return Event{
		Pane: "%3", Agent: "claude", Severity: limits.SevLimit, Text: "5-hour limit reached ∙ resets 3pm",
		PatternID: "claude.limit.1", Session: "proj", At: testutil.FrozenTime,
	}
}

var clients = []Client{{Name: "/dev/pts/1", Session: "other", Activity: 9}, {Name: "/dev/pts/2", Session: "proj", Activity: 5}}

// withBins makes OnEvent's Refresh keep the fixture binaries.
func withBins(a *Actor) {
	b := a.W.Bins()
	resolve = func(w *workspace.WS) { w.SetBins(b) }
}

func TestRecordState(t *testing.T) {
	a, _, _ := newActor(t, "off")
	l, err := a.Record(limitEvent())
	if err != nil {
		t.Fatal(err)
	}
	st, _ := state.Read()
	got := st.Limits["claude"]
	if got == nil || got.Source != "screen" || got.Pane != "%3" || got.Severity != "limit" || got.Text != "5-hour limit reached ∙ resets 3pm" ||
		got.ResetAt.IsZero() || !got.ExpiresAt.Equal(got.ResetAt) || !got.At.Equal(testutil.FrozenTime) || !l.ResetAt.Equal(got.ResetAt) {
		t.Fatalf("state record %+v", got)
	}
	// Unparseable reset: expires after default_expiry.
	ev := limitEvent()
	ev.Agent, ev.Text = "codex", "You've hit your usage limit."
	l, _ = a.Record(ev)
	if !l.ResetAt.IsZero() || !l.ExpiresAt.Equal(ev.At.Add(5*time.Hour)) {
		t.Fatalf("default expiry %+v", l)
	}
	// A warning never replaces an active limit and is not blocking.
	ev = limitEvent()
	ev.Severity, ev.Text = limits.SevWarn, "Approaching weekly limit"
	_, _ = a.Record(ev)
	st, _ = state.Read()
	if st.Limits["claude"].Severity != "limit" {
		t.Fatal("warn replaced a limit")
	}
	ev.Agent = "gemini"
	_, _ = a.Record(ev)
	st, _ = state.Read()
	if st.ActiveLimit("gemini", ev.At) != nil || st.ActiveRecord("gemini", ev.At) == nil {
		t.Fatal("warn must be recorded but not blocking")
	}
}

func TestAskMode(t *testing.T) {
	a, r, sp := newActor(t, "ask")
	withBins(a)
	// Codex is limited: it is not offered.
	_ = state.Update(func(s *state.State) error {
		s.SetLimit("codex", &state.Limit{At: testutil.FrozenTime, ExpiresAt: testutil.FrozenTime.Add(time.Hour), Severity: "limit"})
		return nil
	})
	if !a.OnEvent(context.Background(), limitEvent(), clients) {
		t.Fatal("ask should start the cooldown")
	}
	m := r.find("display-menu")
	got := strings.Join(m, "\x00")
	for _, want := range []string{
		"-c\x00/dev/pts/2\x00-t\x00%3", // the client showing the pane's session
		"-T\x00Claude hit its limit (resets 15:00): continue in …",
		"Gemini (with context)\x00g\x00run-shell -b '/opt/imux switch gemini --pane %3 --reason limit --client /dev/pts/2",
		"Dismiss\x00d\x00",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("menu lacks %q:\n%q", want, m)
		}
	}
	if strings.Contains(got, "Codex (with") || strings.Contains(got, "Claude (with") || strings.Contains(got, "Copilot") {
		t.Errorf("limited, current or missing agents offered: %q", m)
	}
	if len(sp.args) != 0 {
		t.Fatal("ask mode spawned a switch")
	}
}

func TestAutoOffAndAllLimited(t *testing.T) {
	a, r, sp := newActor(t, "auto")
	withBins(a)
	a.W.Cfg.Order = []string{"codex", "claude", "gemini", "copilot"}
	a.OnEvent(context.Background(), limitEvent(), clients)
	if len(sp.args) != 1 || strings.Join(sp.args[0], " ") != "/opt/imux switch codex --pane %3 --reason limit --client /dev/pts/2" {
		t.Fatalf("auto spawned %q", sp.args)
	}
	if msg := r.find("display-message"); !strings.Contains(strings.Join(msg, " "), "switching to Codex") {
		t.Fatalf("auto message %q", msg)
	}

	a, r, sp = newActor(t, "off")
	withBins(a)
	a.OnEvent(context.Background(), limitEvent(), nil)
	if len(sp.args) != 0 || r.find("display-menu") != nil || !strings.Contains(strings.Join(r.find("display-message"), " "), "Claude hit its limit (resets 15:00)") {
		t.Fatalf("off mode: %q", r.calls)
	}

	a, r, sp = newActor(t, "auto")
	withBins(a)
	_ = state.Update(func(s *state.State) error {
		for _, id := range []string{"gemini", "codex"} {
			s.SetLimit(id, &state.Limit{At: testutil.FrozenTime, ExpiresAt: testutil.FrozenTime.Add(time.Hour), Severity: "limit"})
		}
		return nil
	})
	a.OnEvent(context.Background(), limitEvent(), clients)
	if len(sp.args) != 0 || !strings.Contains(strings.Join(r.find("display-message"), " "), "every other agent is limited") {
		t.Fatalf("all limited: spawned %q, calls %q", sp.args, r.calls)
	}
	// A warn event records but never acts.
	a, r, _ = newActor(t, "ask")
	withBins(a)
	ev := limitEvent()
	ev.Severity = limits.SevWarn
	if a.OnEvent(context.Background(), ev, clients) || len(r.calls) != 0 {
		t.Fatalf("warn acted: %q", r.calls)
	}
}

func TestPickClient(t *testing.T) {
	if got := pickClient(clients, "proj"); got != "/dev/pts/2" {
		t.Fatal(got)
	}
	if got := pickClient(clients, "nope"); got != "/dev/pts/1" {
		t.Fatal(got)
	}
	if pickClient(nil, "x") != "" {
		t.Fatal("no clients")
	}
}
