package sidebar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
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
	now := testutil.FrozenTime
	cur := []PaneRow{
		{ID: "%1", Agent: "claude", Name: "backend fix", Label: "Claude · opus", State: StateWorking, Since: now.Add(-30 * time.Second), Target: true},
		{ID: "%2", Agent: "codex", Title: "og tags", Label: "Codex", State: StateIdle, Since: now.Add(-4 * time.Minute), Limited: "15:00"},
		{ID: "%4", Agent: "antigravity", Label: "Antigravity", State: StateBlocked, Since: now.Add(-2 * time.Hour)},
		{ID: "%3", State: StateShell},
	}
	return Model{
		Project: "acme-api", CWD: "/home/test/acme-api", Branch: "main", Changed: 3, Git: true, Target: "%1", Now: now,
		Changes: []ChangeRow{{"M", "src/auth/redirect.ts"}, {"A", "test/auth.test.ts"}, {"?", "notes.md"}},
		Panes:   cur,
		Projects: []ProjectRow{
			{Session: "acme-api-1a2b", Name: "acme-api", Branch: "main", Dirty: true, Current: true, Panes: cur},
			{Session: "blog-3c4d", Name: "blog", Branch: "master", Panes: []PaneRow{
				{ID: "%7", Agent: "antigravity", Title: "blog pass", State: StateDone, Since: now.Add(-9 * time.Minute)},
				{ID: "%8", Agent: "claude", State: StateIdle, Since: now.Add(-3 * 24 * time.Hour)},
			}},
		},
		Agents: []AgentRow{
			{ID: "claude", Name: "Claude", Vendor: "Anthropic", Key: "c", Installed: true, Current: true},
			{ID: "antigravity", Name: "Antigravity", Vendor: "Google", Key: "a", Installed: true},
			{ID: "copilot", Name: "Copilot", Vendor: "GitHub", Key: "p"},
			{ID: "codex", Name: "Codex", Vendor: "OpenAI", Key: "x", Installed: true, Limited: "15:00"},
		},
	}
}

func TestRender(t *testing.T) {
	i18n.SetLocale("en")
	for _, w := range []int{22, 34, 44} {
		for _, h := range []int{0, 12, 20, 50} {
			sc := Render(model(), w, h, NoItem)
			if h > 0 && len(sc.Lines) != h {
				t.Errorf("width %d height %d: %d rows", w, h, len(sc.Lines))
			}
			for _, l := range sc.Lines {
				if n := utf8.RuneCountInString(Plain(l.Text)); n != w {
					t.Errorf("width %d height %d: line is %d columns: %q", w, h, n, Plain(l.Text))
				}
			}
		}
	}
	sc := Render(model(), 34, 0, NoItem)
	text := screenText(sc)
	t.Logf("\n%s", text)
	for _, want := range []string{
		"Projects", " A  acme-api", "main ●", " B  blog", "master ●",
		"Agents", "acme-api", "✻  backend fix", "<1m ⠋", "◎  og tags", "4m  15:00", "▲  Antigravity", "2h  Needs you", "›  Shell",
		"blog", "▲  blog pass", "9m ●", "✻  Claude", "3d ○",
		"Continue this chat in", "▲ Antigravity", "◉ Copilot", "◎ Codex",
		"✎ Rename", "± Changes 3", "⧉ History", "◔ Limits", "? Help",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("sidebar lacks %q:\n%s", want, text)
		}
	}
	// The current agent is not a place to continue in.
	cont := text[strings.Index(text, "Continue this chat in"):]
	if strings.Contains(cont[:strings.Index(cont, "─")], "Claude") {
		t.Errorf("the current agent is listed under Continue:\n%s", cont)
	}
	if strings.Contains(text, "⛔") {
		t.Error("the sidebar must not use wide characters")
	}
	find := func(sc Screen, sub string) (Line, int) {
		for _, l := range sc.Lines {
			if i := strings.Index(Plain(l.Text), sub); i >= 0 {
				return l, utf8.RuneCountInString(Plain(l.Text)[:i])
			}
		}
		t.Fatalf("no row with %q", sub)
		return Line{}, 0
	}
	for _, c := range []struct {
		row  string
		want Action
	}{
		{"og tags", Action{Kind: ActFocus, Pane: "%2"}},
		{"blog pass", Action{Kind: ActFocus, Pane: "%7"}},
		{" B  blog", Action{Kind: ActProject, Session: "blog-3c4d"}},
		{"▲ Antigravity", Action{Kind: ActSwitch, Agent: "antigravity"}},
		{"◎ Codex", Action{Kind: ActSwitch, Agent: "codex"}},
		{"✎ Rename", Action{Kind: ActRename}},
		{"± Changes", Action{Kind: ActChanges}},
		{"⧉ History", Action{Kind: ActSessions}},
		{"◔ Limits", Action{Kind: ActLimits}},
		{"? Help", Action{Kind: ActKeys}},
	} {
		// Hit-test at the text's column: some rows hold several buttons.
		l, x := find(sc, c.row)
		if got := l.ActionAt(x); got != c.want {
			t.Errorf("%q: %+v, want %+v", c.row, got, c.want)
		}
	}
	// The "+" of the headers.
	if l, _ := find(sc, "Projects"); l.ActionAt(33) != (Action{Kind: ActOpen}) || l.ActionAt(3).Kind != ActNone {
		t.Error("Projects +")
	}
	if l, _ := find(sc, "Agents"); l.ActionAt(32) != (Action{Kind: ActNewAgent}) {
		t.Error("Agents +")
	}
	if l, _ := find(sc, " A  acme-api"); l.Action.Kind != ActNone || !strings.Contains(l.Text, selBg) {
		t.Error("the current project is highlighted, not an action")
	}
	if l, _ := find(sc, "backend fix"); !strings.HasPrefix(l.Text, selBg) {
		t.Error("the target agent is highlighted")
	}
	// The spinner turns.
	m := model()
	m.Tick = 3
	if !m.Animated() || !strings.Contains(screenText(Render(m, 34, 0, NoItem)), "<1m ⠸") {
		t.Error("spinner frame")
	}

	// Short: blank rows go, then other projects fold; the bottom blocks stay.
	short := Render(model(), 34, 18, NoItem)
	stext := screenText(short)
	t.Logf("\n%s", stext)
	for _, want := range []string{"Agents", "backend fix", "Continue this chat in", "✎ Rename", "? Help"} {
		if !strings.Contains(stext, want) {
			t.Errorf("short sidebar lacks %q:\n%s", want, stext)
		}
	}
	if strings.Contains(stext, "blog pass") {
		t.Errorf("the other project should be folded:\n%s", stext)
	}
	// Tall: the bottom blocks are anchored to the bottom.
	tall := Render(model(), 34, 50, NoItem)
	if !strings.Contains(Plain(tall.Lines[49].Text), "? Help") || strings.TrimSpace(Plain(tall.Lines[30].Text)) != "" {
		t.Errorf("toolbar not at the bottom:\n%s", screenText(tall))
	}
	// Narrow: pills become "!".
	if n := screenText(Render(model(), 22, 0, NoItem)); strings.Contains(n, "Needs you") {
		t.Errorf("narrow:\n%s", n)
	}

	// The keyboard selection is drawn in reverse video, a button on its own.
	items := sc.Items()
	if a := sc.ActionOf(items[0]); a.Kind != ActOpen {
		t.Fatalf("first item %+v", a)
	}
	row := -1
	for _, it := range items {
		if it.Cell < 0 && sc.Lines[it.Row].Action.Kind == ActFocus {
			row = it.Row
			break
		}
	}
	sel := Render(model(), 34, 0, Item{row, -1})
	if !strings.HasPrefix(sel.Lines[row].Text, rev) {
		t.Error("selection not highlighted")
	}
	last := items[len(items)-1]
	if last.Cell != 2 || sc.ActionOf(last).Kind != ActKeys {
		t.Fatalf("last item %+v", last)
	}
	sel = Render(model(), 34, 0, last)
	lt := sel.Lines[last.Row].Text
	if strings.HasPrefix(lt, rev) || !strings.Contains(lt, rev) || utf8.RuneCountInString(Plain(lt)) != 34 {
		t.Errorf("button selection %q", lt)
	}
	f := Frame(sc, 10)
	if !strings.HasPrefix(f, "\x1b[H") || !strings.HasSuffix(f, "\x1b[J") || strings.Count(f, "\r\n") != 9 {
		t.Errorf("frame %q", f)
	}
}

func TestCleanTitleAndAge(t *testing.T) {
	for title, want := range map[string]string{
		"✳ Fix login bug": "Fix login bug", "⠂ Refactor the parser": "Refactor the parser",
		"macbook": "", "macbook.local": "", "MacBook": "", "claude": "", "Claude Code": "", "node": "",
		"~/code/acme": "", "": "", "✳": "",
	} {
		if got := CleanTitle(title, "macbook.local", "claude"); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", title, got, want)
		}
	}
	now := testutil.FrozenTime
	for d, want := range map[time.Duration]string{
		10 * time.Second: "<1m", 9 * time.Minute: "9m", 2 * time.Hour: "2h", 74 * time.Hour: "3d",
	} {
		if got := age(now, now.Add(-d)); got != want {
			t.Errorf("age %v = %q, want %q", d, got, want)
		}
	}
	if age(now, time.Time{}) != "" {
		t.Error("unknown age")
	}
}

func screenText(sc Screen) string {
	var plain []string
	for _, l := range sc.Lines {
		plain = append(plain, strings.TrimRight(Plain(l.Text), " "))
	}
	return strings.Join(plain, "\n")
}

func TestParseStatus(t *testing.T) {
	got := ParseStatus([]string{" M src/a.go", "A  b.go", "?? notes.md", "R  old.go -> new.go", "MM both.go", `?? "with space.md"`, "x"})
	want := []ChangeRow{{"M", "src/a.go"}, {"A", "b.go"}, {"?", "notes.md"}, {"R", "new.go"}, {"M", "both.go"}, {"?", "with space.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestMoveWraps(t *testing.T) {
	i18n.SetLocale("en")
	s := &sidebar{screen: Render(model(), 34, 0, NoItem), sel: NoItem}
	rows := s.screen.Items()
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
	// Another project (session) with a named agent pane is listed after
	// the current one; this project's pane name is read.
	other := filepath.Join(h.Home, "blog")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err = c.Run(ctx, tmux.Cmd{"new-session", "-d", "-s", "blog-1234", "-c", other, "-P", "-F", "#{pane_id}", "sh"})
	if err != nil {
		t.Fatal(err)
	}
	blogPane := strings.TrimSpace(out)
	// It finished work nobody has looked at (Done) and set its title.
	ended := strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)
	_, _ = c.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", blogPane, "@imux_agent", "antigravity"},
		tmux.Cmd{"set-option", "-p", "-t", blogPane, OptState, StateIdle},
		tmux.Cmd{"set-option", "-p", "-t", blogPane, OptStateAt, ended},
		tmux.Cmd{"set-option", "-p", "-t", blogPane, OptDoneAt, ended},
		tmux.Cmd{"select-pane", "-t", blogPane, "-T", "✳ Fix login bug"},
		tmux.Cmd{"set-option", "-p", "-t", agentPane, OptName, "backend fix"})
	m, _, _ = src.Gather(ctx, bar)
	if len(m.Projects) != 2 || !m.Projects[0].Current || m.Projects[0].Name != "proj" ||
		m.Projects[1].Name != "blog" || m.Projects[1].Session != "blog-1234" || len(m.Projects[1].Panes) != 1 ||
		m.Projects[1].Panes[0].Agent != "antigravity" {
		t.Fatalf("projects %+v", m.Projects)
	}
	if m.Panes[0].Name != "backend fix" || m.Projects[0].Panes[0].Name != "backend fix" {
		t.Errorf("pane name %+v", m.Panes[0])
	}
	if p := m.Projects[1].Panes[0]; p.State != StateDone || p.Title != "Fix login bug" || !p.Since.Equal(now.Add(-time.Minute).Truncate(time.Second)) {
		t.Errorf("done pane %+v", p)
	}
	if m.Projects[1].Branch != "main" || !m.Projects[1].Dirty {
		t.Errorf("other project's git %+v", m.Projects[1])
	}
	// Looked at (seen after it ended): idle.
	_, _ = c.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", blogPane, OptSeen, strconv.FormatInt(now.Unix(), 10)})
	if m, _, _ = src.Gather(ctx, bar); m.Projects[1].Panes[0].State != StateIdle {
		t.Errorf("seen pane %+v", m.Projects[1].Panes[0])
	}
	_, _ = c.Run(ctx, tmux.Cmd{"kill-session", "-t", "blog-1234"})
	// Only the sidebar left: alone.
	_, _ = c.Run(ctx, tmux.Cmd{"kill-pane", "-t", shell}, tmux.Cmd{"kill-pane", "-t", agentPane})
	if _, win, _ = src.Gather(ctx, bar); !win.Alone {
		t.Error("a window with only the sidebar should be alone")
	}
}
