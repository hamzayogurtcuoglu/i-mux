package sidebar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in    string
		final bool
		want  []Event
		rest  string
	}{
		{"a\r", true, []Event{{Kind: EvKey, Key: "a"}, {Kind: EvKey, Key: "enter"}}, ""},
		{"\x1b[A\x1b[B\x1bOA\t\x1b[Z", true, []Event{{Key: "up"}, {Key: "down"}, {Key: "up"}, {Key: "tab"}, {Key: "backtab"}}, ""},
		// Left press at column 5, row 7; its release and wheel events are ignored.
		{"\x1b[<0;5;7M\x1b[<0;5;7m\x1b[<64;5;7M\x1b[<2;1;1M", true, []Event{{Kind: EvClick, X: 5, Y: 7}}, ""},
		// An incomplete sequence is kept for the next read.
		{"x\x1b[<0;12", true, []Event{{Key: "x"}}, "\x1b[<0;12"},
		{"\x1b[", true, nil, "\x1b["},
		// A lone ESC is Esc only at the end of input.
		{"\x1b", true, []Event{{Key: "esc"}}, ""},
		{"\x1b", false, nil, "\x1b"},
		{"\x1bq", true, []Event{{Key: "esc"}, {Key: "q"}}, ""},
		{"\x03\x7f", true, nil, ""}, // Ctrl+C and Backspace do nothing
	}
	for _, c := range cases {
		got, rest := Parse([]byte(c.in), c.final)
		if len(got) == 0 && len(c.want) == 0 {
			got = nil
		}
		if !reflect.DeepEqual(got, c.want) || string(rest) != c.rest {
			t.Errorf("Parse(%q) = %+v, rest %q; want %+v, %q", c.in, got, rest, c.want, c.rest)
		}
	}
}

func model() Model {
	return Model{
		Project: "acme-api", Branch: "main", Changed: 3, Git: true, Target: "%1",
		Panes: []PaneRow{
			{ID: "%1", Agent: "claude", Label: "Claude · opus", State: StateWorking, Target: true},
			{ID: "%2", Agent: "codex", Label: "Codex", State: StateIdle, Limited: "15:00"},
			{ID: "%3", State: StateShell},
		},
		Agents: []AgentRow{
			{ID: "claude", Name: "Claude", Key: "c", Installed: true, Current: true},
			{ID: "antigravity", Name: "Antigravity", Key: "a", Installed: true},
			{ID: "copilot", Name: "Copilot", Key: "p"},
			{ID: "codex", Name: "Codex", Key: "x", Installed: true, Limited: "15:00"},
		},
	}
}

func TestRender(t *testing.T) {
	i18n.SetLocale("en")
	for _, w := range []int{22, 30, 44} {
		sc := Render(model(), w, -1)
		for i, l := range sc.Lines {
			if n := utf8.RuneCountInString(Plain(l.Text)); n != w {
				t.Errorf("width %d: line %d is %d columns: %q", w, i, n, Plain(l.Text))
			}
		}
	}
	sc := Render(model(), 30, -1)
	var plain []string
	byText := map[string]Action{}
	for _, l := range sc.Lines {
		p := strings.TrimRight(Plain(l.Text), " ")
		plain = append(plain, p)
		byText[strings.TrimSpace(p)] = l.Action
	}
	text := strings.Join(plain, "\n")
	for _, want := range []string{
		" acme-api", "on main · 3 changed", "PANES", "▌● Claude · opus", "working",
		"◆ Codex", "limited 15:00", "· Shell", "CONTINUE HERE IN", "c Claude", "current",
		"a Antigravity", "Copilot", "not installed", "Split right", "Split down", "Sessions…", "Limits", "Keys",
		"^b b hide",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("sidebar lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "⛔") {
		t.Error("the sidebar must not use wide characters")
	}
	// Rows map to actions.
	find := func(sub string) Action {
		for _, l := range sc.Lines {
			if strings.Contains(Plain(l.Text), sub) {
				return l.Action
			}
		}
		t.Fatalf("no row with %q", sub)
		return Action{}
	}
	if a := find("◆ Codex"); a.Kind != ActFocus || a.Pane != "%2" {
		t.Errorf("codex pane row %+v", a)
	}
	if a := find("a Antigravity"); a.Kind != ActSwitch || a.Agent != "antigravity" {
		t.Errorf("antigravity row %+v", a)
	}
	if a := find("Split right"); a.Kind != ActSplitH {
		t.Errorf("split right %+v", a)
	}
	if a := find("Split down"); a.Kind != ActSplitV {
		t.Errorf("split down %+v", a)
	}
	if find("PANES").Kind != ActNone || find("acme-api").Kind != ActNone {
		t.Error("headings must not be actions")
	}
	// The keyboard selection is drawn in reverse video.
	rows := sc.Selectable()
	sel := Render(model(), 30, rows[1])
	if !strings.HasPrefix(sel.Lines[rows[1]].Text, rev) || strings.HasPrefix(sel.Lines[rows[0]].Text, rev) {
		t.Error("selection not highlighted")
	}
	// Frame draws from the top and clears the rest.
	f := Frame(sc, 10)
	if !strings.HasPrefix(f, "\x1b[H") || !strings.HasSuffix(f, "\x1b[J") || strings.Count(f, "\r\n") != 9 {
		t.Errorf("frame %q", f)
	}
}

func TestMoveWraps(t *testing.T) {
	i18n.SetLocale("en")
	s := &sidebar{screen: Render(model(), 30, -1), sel: -1}
	rows := s.screen.Selectable()
	s.move(1)
	if s.sel != rows[0] {
		t.Fatalf("first down selects the first row: %d", s.sel)
	}
	s.move(-1)
	if s.sel != rows[len(rows)-1] {
		t.Fatalf("up from the first wraps to the last: %d", s.sel)
	}
}

// realTmux starts a private tmux server for the test.
func realTmux(t *testing.T) *tmux.Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	c := &tmux.Client{Bin: "tmux", Socket: "imux-sidebar-test-" + hex.EncodeToString(b), Config: "/dev/null", Runner: tmux.ExecRunner{}}
	t.Cleanup(func() { _, _ = c.Do(tmux.Cmd{"kill-server"}) })
	return c
}

// TestGatherRealTmux: in a window with a sidebar, an agent pane and a shell,
// the sidebar is not listed, the active pane is the target (and becomes
// @imux_last), agents come in config order with install and limit state, and
// a window left with only the sidebar is reported alone.
func TestGatherRealTmux(t *testing.T) {
	h := testutil.Isolate(t)
	i18n.SetLocale("en")
	loc := time.Local
	time.Local = time.UTC // reset clocks are shown in local time
	t.Cleanup(func() { time.Local = loc })
	c := realTmux(t)
	ctx := context.Background()
	out, err := c.Run(ctx, tmux.Cmd{"new-session", "-d", "-x", "120", "-y", "40", "-c", h.Project, "-P", "-F", "#{pane_id}", "sh"})
	if err != nil {
		t.Fatal(err)
	}
	agentPane := strings.TrimSpace(out)
	out, _ = c.Run(ctx, tmux.Cmd{"split-window", "-h", "-b", "-f", "-d", "-l", "30", "-t", agentPane, "-P", "-F", "#{pane_id}", "sh"})
	bar := strings.TrimSpace(out)
	out, _ = c.Run(ctx, tmux.Cmd{"split-window", "-v", "-d", "-t", agentPane, "-P", "-F", "#{pane_id}", "sh"})
	shell := strings.TrimSpace(out)
	_, err = c.Run(ctx,
		tmux.Cmd{"set-option", "-p", "-t", bar, OptRole, RoleSidebar},
		tmux.Cmd{"set-option", "-p", "-t", agentPane, "@imux_agent", "claude"},
		tmux.Cmd{"set-option", "-p", "-t", agentPane, "@imux_label", "Claude"},
		tmux.Cmd{"set-option", "-p", "-t", agentPane, OptState, StateWorking},
		tmux.Cmd{"select-pane", "-t", agentPane})
	if err != nil {
		t.Fatal(err)
	}
	now := testutil.FrozenTime
	if err := state.Update(func(s *state.State) error {
		s.SetLimit("codex", &state.Limit{At: now, ResetAt: now.Add(time.Hour), ExpiresAt: now.Add(time.Hour), Severity: "limit"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	src := &Source{
		T: c, Cfg: config.Default(), Now: func() time.Time { return now },
		Resolve: func(*config.Config, *state.State) map[session.AgentID]agent.Resolved {
			return map[session.AgentID]agent.Resolved{"claude": {Installed: true}, "codex": {Installed: true}}
		},
		Git: func(context.Context, string) gitinfo.Info {
			return gitinfo.Info{State: gitinfo.OK, Branch: "main", StatusTotal: 2}
		},
	}
	m, win, err := src.Gather(ctx, bar)
	if err != nil {
		t.Fatal(err)
	}
	if win.Alone || win.Width != 30 || m.Target != agentPane || len(m.Panes) != 2 {
		t.Fatalf("window %+v model %+v", win, m)
	}
	if p := m.Panes[0]; p.ID != agentPane || p.State != StateWorking || !p.Target || p.Label != "Claude" {
		t.Errorf("agent row %+v", p)
	}
	if p := m.Panes[1]; p.ID != shell || p.State != StateShell {
		t.Errorf("shell row %+v", p)
	}
	var ids []string
	for _, a := range m.Agents {
		ids = append(ids, string(a.ID))
	}
	if strings.Join(ids, ",") != "claude,antigravity,copilot,codex" {
		t.Errorf("agents %v", ids)
	}
	if !m.Agents[0].Current || m.Agents[1].Installed || m.Agents[3].Limited != "15:32" {
		t.Errorf("agent rows %+v", m.Agents)
	}
	if !m.Git || m.Branch != "main" || m.Changed != 2 || m.Project != "proj" {
		t.Errorf("project %+v", m)
	}
	if got, _ := c.Run(ctx, tmux.Cmd{"show-options", "-wqv", "-t", bar, OptLast}); strings.TrimSpace(got) != agentPane {
		t.Errorf("@imux_last %q", got)
	}
	// Focusing the sidebar keeps the last agent pane as the target.
	_, _ = c.Run(ctx, tmux.Cmd{"select-pane", "-t", bar})
	if m, _, _ = src.Gather(ctx, bar); m.Target != agentPane {
		t.Errorf("target with the sidebar focused: %q", m.Target)
	}
	// Only the sidebar left: alone.
	_, _ = c.Run(ctx, tmux.Cmd{"kill-pane", "-t", shell}, tmux.Cmd{"kill-pane", "-t", agentPane})
	if _, win, _ = src.Gather(ctx, bar); !win.Alone {
		t.Error("a window with only the sidebar should be alone")
	}
}
