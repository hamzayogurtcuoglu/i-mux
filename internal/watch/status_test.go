package watch

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/clock"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func join(cmds [][]string) string {
	var out []string
	for _, c := range cmds {
		out = append(out, strings.Join(c, " "))
	}
	return strings.Join(out, "\n")
}

func TestStatusKeeper(t *testing.T) {
	a, _, _ := newActor(t, "ask")
	withBins(a)
	clk := clock.NewFixed(testutil.FrozenTime)
	a.W.Now = clk.Now
	k := &StatusKeeper{W: a.W}
	now := testutil.FrozenTime
	reset := now.Add(30 * time.Minute)
	_ = state.Update(func(s *state.State) error {
		s.SetLimit("claude", &state.Limit{At: now, ResetAt: reset, ExpiresAt: reset, Severity: "limit"})
		s.SetLimit("antigravity", &state.Limit{At: now, ExpiresAt: now.Add(time.Hour), Severity: "warn"})
		return nil
	})
	panes := []PaneRow{{ID: "%1", Agent: "claude"}, {ID: "%2", Agent: "antigravity"}, {ID: "%3"}}
	cmds := k.Commands(panes)
	var raw [][]string
	for _, c := range cmds {
		raw = append(raw, c)
	}
	got := join(raw)
	until := reset.Local().Format("15:04")
	for _, want := range []string{
		"set-option -g @imux_status Claude ⛔" + until + " · Antigravity ◐ · Codex ●",
		"set-option -g @imux_lim_claude ⛔" + until,
		"set-option -g @imux_lim_antigravity ◐",
		"set-option -g @imux_lim_codex ",
		"set-option -p -t %1 @imux_flag ⛔ " + until,
		"set-option -p -u -t %2 @imux_flag", // a warning does not flag the pane
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "Copilot") || strings.Contains(got, "%3") {
		t.Errorf("not-installed agent or shell pane in status:\n%s", got)
	}
	// Nothing changed: no commands.
	if c := k.Commands(panes); len(c) != 0 {
		t.Fatalf("unchanged status re-set: %v", c)
	}
	// The limit expires: cleared on the next tick, and dropped from state.
	clk.Set(reset.Add(time.Second))
	raw = nil
	for _, c := range k.Commands(panes) {
		raw = append(raw, c)
	}
	got = join(raw)
	for _, want := range []string{"@imux_status Claude ● · Antigravity ◐ · Codex ●", "set-option -g @imux_lim_claude ", "set-option -p -u -t %1 @imux_flag"} {
		if !strings.Contains(got, want) {
			t.Errorf("after expiry missing %q in\n%s", want, got)
		}
	}
	if st, _ := state.Read(); st.Limits["claude"] != nil {
		t.Fatal("expired limit kept in state.json")
	}
	// Forget re-sends everything (e.g. after a failed exec).
	k.Forget()
	if c := k.Commands(panes); len(c) == 0 {
		t.Fatal("Forget did not reset")
	}
}
