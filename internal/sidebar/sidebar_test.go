package sidebar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/skills"
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
		// Left press at column 5, row 7; its release and wheel events are
		// ignored; a right press is a right click.
		{"\x1b[<0;5;7M\x1b[<0;5;7m\x1b[<64;5;7M\x1b[<2;1;1M", true, []Event{{Kind: EvClick, X: 5, Y: 7}, {Kind: EvScroll, Key: "up", X: 5, Y: 7}, {Kind: EvRightClick, X: 1, Y: 1}}, ""},
		// Motion with no button (35) and with the left one held (32) moves
		// the hover; a report at column 0 is the pointer leaving.
		{"\x1b[<35;9;3M\x1b[<32;10;3M\x1b[<35;0;0M", true, []Event{{Kind: EvMove, X: 9, Y: 3}, {Kind: EvMove, X: 10, Y: 3}, {Kind: EvMove}}, ""},
		// An incomplete sequence is kept for the next read.
		{"x\x1b[<0;12", true, []Event{{Key: "x"}}, "\x1b[<0;12"},
		{"\x1b[", true, nil, "\x1b["},
		// A lone ESC is Esc only at the end of input.
		{"\x1b", true, []Event{{Key: "esc"}}, ""},
		{"\x1b", false, nil, "\x1b"},
		{"\x1bq", true, []Event{{Key: "esc"}, {Key: "q"}}, ""},
		{"\x03\x7f\x08\x15", true, []Event{{Key: "ctrl-c"}, {Key: "backspace"}, {Key: "backspace"}, {Key: "ctrl-u"}}, ""},
		// Home, End and Delete in their CSI and SS3 forms; SS3 arrows; the
		// line-editing control keys.
		{"\x1b[H\x1b[F\x1b[1~\x1b[4~\x1b[3~\x1bOH\x1bOF\x1bOC\x1bOD\x01\x05\x17", true, []Event{{Key: "home"}, {Key: "end"}, {Key: "home"}, {Key: "end"}, {Key: "delete"}, {Key: "home"}, {Key: "end"}, {Key: "right"}, {Key: "left"}, {Key: "ctrl-a"}, {Key: "ctrl-e"}, {Key: "ctrl-w"}}, ""},
		// A bracketed paste is one event, its newlines kept; one still
		// arriving waits for the rest.
		{"a\x1b[200~x\ry\x1b[201~b", true, []Event{{Key: "a"}, {Kind: EvPaste, Key: "paste", Text: "x\ry"}, {Key: "b"}}, ""},
		{"\x1b[200~half\r", true, nil, "\x1b[200~half\r"},
		// UTF-8 characters, and one cut in the middle.
		{"Masaüstü", true, []Event{{Key: "M"}, {Key: "a"}, {Key: "s"}, {Key: "a"}, {Key: "ü"}, {Key: "s"}, {Key: "t"}, {Key: "ü"}}, ""},
		{"a\xc3", false, []Event{{Key: "a"}}, "\xc3"},
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
		Changes: []ChangeRow{{Status: "M", Path: "src/auth/redirect.ts", Added: 12, Removed: 3}, {Status: "A", Path: "test/auth.test.ts", Added: 40}, {Status: "?", Path: "notes.md"}},
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

// TestAgentsWorkingCount: the AGENTS title counts the agents at work (not a
// limited one), in every project; folded, it also counts the panes.
func TestAgentsWorkingCount(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.Projects[1].Panes[1].State = StateWorking
	m.Projects[0].Panes[1].State = StateWorking // limited: not counted
	agents := func(m Model, w int) string {
		for _, l := range Render(m, w, 0, NoItem).Lines {
			if p := Plain(l.Text); strings.Contains(p, "AGENTS") {
				return p
			}
		}
		return ""
	}
	if got := agents(m, 41); !strings.Contains(got, "⠋ 2 working") {
		t.Fatalf("AGENTS title %q", got)
	}
	if got := agents(m, 29); !strings.Contains(got, "⠋ 2 ") || strings.Contains(got, "working") {
		t.Fatalf("narrow AGENTS title %q", got)
	}
	m.Fold = map[string]bool{SecAgents: true}
	if got := agents(m, 41); !strings.Contains(got, "⠋ 2 working · 6") {
		t.Fatalf("folded AGENTS title %q", got)
	}
	for _, pr := range m.Projects {
		for i := range pr.Panes {
			pr.Panes[i].State = StateIdle
		}
	}
	if got := agents(m, 41); strings.Contains(got, "working") || !strings.Contains(got, " 6 ") {
		t.Fatalf("nothing at work: %q", got)
	}
}

// TestAgentsBadge (D46): the rail's Agents icon has the number of working
// agents on blue, "9+" above nine, nothing at zero; on any view, with the
// panel closed, with either icon theme, and the rail keeps its width.
func TestAgentsBadge(t *testing.T) {
	i18n.SetLocale("en")
	iconRow := func(m Model) Line {
		for _, l := range Render(m, 41, 30, NoItem).Lines {
			if strings.Contains(l.Text, railIcons[m.Icons][0]) {
				return l
			}
		}
		t.Fatalf("no Agents icon (%s)", m.Icons)
		return Line{}
	}
	m := model() // one working agent (a limited one is not counted)
	for _, theme := range []string{IconsEmoji, IconsNerd} {
		m.Icons = theme
		row := iconRow(m)
		plain := Plain(row.Text)
		col := DisplayWidth(plain[:strings.Index(plain, " 1 ")+1]) // the badge's column
		if col != badgeAt || !strings.Contains(row.Text, badgeStyle+"1") {
			t.Errorf("%s: badge at column %d in %q", theme, col, row.Text)
		}
		if n := DisplayWidth(Plain(row.Text)); n != 41 {
			t.Errorf("%s: %d columns", theme, n)
		}
	}
	m.View, m.Icons = ViewExplorer, IconsEmoji
	if !strings.Contains(iconRow(m).Text, badgeStyle+"1") {
		t.Error("no badge with the explorer open")
	}
	m.Closed = true
	if row := iconRow(m); !strings.Contains(row.Text, badgeStyle+"1") || DisplayWidth(Plain(row.Text)) > 41 {
		t.Errorf("closed panel: %q", Plain(row.Text))
	}
	m.Closed = false
	var many []PaneRow
	for i := 0; i < 12; i++ {
		many = append(many, PaneRow{ID: "%9" + strconv.Itoa(i), Agent: "claude", State: StateWorking})
	}
	m.Projects = append(m.Projects, ProjectRow{Session: "big-1", Name: "big", Panes: many})
	if row := iconRow(m); !strings.Contains(row.Text, badgeStyle+"9+") || DisplayWidth(Plain(row.Text)) != 41 {
		t.Errorf("13 working: %q", Plain(row.Text))
	}
	m = model()
	m.Projects[0].Panes[0].State = StateIdle
	if row := iconRow(m); strings.Contains(row.Text, badgeStyle) {
		t.Errorf("a badge with nothing at work: %q", Plain(row.Text))
	}
}

// TestTabState: the tab bar's glyph is the most urgent pane's; a finished
// agent is idle there too (D56: no green "done").
func TestTabState(t *testing.T) {
	for _, c := range []struct {
		panes []PaneRow
		want  string
	}{
		{[]PaneRow{{State: StateIdle}, {State: StateDone}}, ""},
		{[]PaneRow{{State: StateDone}, {State: StateWorking}}, "#[fg=colour75]●"},
		{[]PaneRow{{State: StateWorking}, {State: StateBlocked}}, "#[fg=colour214]!"},
		{[]PaneRow{{State: StateIdle, Limited: "15:00"}}, "#[fg=colour203]◆"},
	} {
		if got := TabState(c.panes); got != c.want {
			t.Errorf("%+v: %q", c.panes, got)
		}
	}
}

func TestRender(t *testing.T) {
	i18n.SetLocale("en")
	for _, w := range []int{29, 41, 51} {
		for _, h := range []int{0, 12, 20, 50} {
			sc := Render(model(), w, h, NoItem)
			if h > 0 && len(sc.Lines) != h {
				t.Errorf("width %d height %d: %d rows", w, h, len(sc.Lines))
			}
			for _, l := range sc.Lines {
				if n := DisplayWidth(Plain(l.Text)); n != w {
					t.Errorf("width %d height %d: line is %d columns: %q", w, h, n, Plain(l.Text))
				}
			}
		}
	}
	sc := Render(model(), 41, 0, NoItem)
	text := screenText(sc)
	t.Logf("\n%s", text)
	for _, want := range []string{
		"▾ PROJECTS", " A  acme-api  main  Needs you", " B  blog", "master",
		"▾ AGENTS", "acme-api", "✻  backend fix", "<1m ⠋", "◎  og tags", "4m  15:00", "▲  Antigravity", "2h  Needs you", "›  Terminal",
		"blog", "▲  blog pass", "9m ○", "✻  Claude", "3d ○",
		"Continue this chat in", "▲ Antigravity", "◉ Copilot", "◎ Codex",
		"📜 History", "▌  🤖", "Agents", "📁", "Files", "🔍", "Search", "🔀", "Git 3", "📊", "Limits", "❓", "Help",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("sidebar lacks %q:\n%s", want, text)
		}
	}
	// No dot for uncommitted changes (blog has some) or a finished agent (D56).
	if strings.Contains(text, "●") {
		t.Errorf("a dot in the sidebar:\n%s", text)
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
				return l, DisplayWidth(Plain(l.Text)[:i])
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
		{"📜 History", Action{Kind: ActSessions}},
	} {
		// Hit-test at the text's column: some rows hold several buttons.
		l, x := find(sc, c.row)
		if got := l.ActionAt(x); got != c.want {
			t.Errorf("%q: %+v, want %+v", c.row, got, c.want)
		}
	}
	// History is alone on its row (D57: no Image button).
	if l, _ := find(sc, "📜 History"); strings.Contains(Plain(l.Text), "📎") || strings.Contains(Plain(l.Text), "Image") {
		t.Errorf("an Image button: %q", Plain(l.Text))
	}
	// The "+" of the headers.
	if l, _ := find(sc, "PROJECTS"); l.ActionAt(40) != (Action{Kind: ActOpen}) || l.ActionAt(10) != (Action{Kind: ActFold, Section: SecProjects}) {
		t.Error("Projects +")
	}
	if l, _ := find(sc, "AGENTS"); l.ActionAt(39) != (Action{Kind: ActNewAgent}) {
		t.Error("Agents +")
	}
	if l, _ := find(sc, " A  acme-api"); l.ActionAt(10) != (Action{Kind: ActProjectMenu, Session: "acme-api-1a2b"}) || !strings.Contains(l.Text, selBg) {
		t.Error("the current project is highlighted and opens its menu")
	}
	if l, _ := find(sc, "backend fix"); !strings.Contains(l.Text, selBg) || l.ActionAt(10) != (Action{Kind: ActAgentMenu, Pane: "%1"}) {
		t.Error("the target agent is highlighted and opens its menu")
	}
	// The rail: views and the bottom buttons.
	if l, _ := find(sc, "PROJECTS"); l.ActionAt(1) != (Action{Kind: ActView, Section: ViewAgents}) || !strings.Contains(l.Text, cyan+"▌") {
		t.Error("the Agents icon is active")
	}
	// The spinner turns.
	m := model()
	m.Tick = 3
	if !m.Animated() || !strings.Contains(screenText(Render(m, 41, 0, NoItem)), "<1m ⠸") {
		t.Error("spinner frame")
	}

	// Short: blank rows go, then other projects fold; the bottom blocks stay.
	short := Render(model(), 41, 16, NoItem)
	stext := screenText(short)
	t.Logf("\n%s", stext)
	for _, want := range []string{"AGENTS", "backend fix", "Continue this chat in", "📜 History"} {
		if !strings.Contains(stext, want) {
			t.Errorf("short sidebar lacks %q:\n%s", want, stext)
		}
	}
	if strings.Contains(stext, "blog pass") {
		t.Errorf("the other project should be folded:\n%s", stext)
	}
	// Tall: the toolbar and the rail's Help are on the last row.
	tall := Render(model(), 41, 50, NoItem)
	if last := Plain(tall.Lines[49].Text); !strings.Contains(last, "📜 History") || !strings.HasPrefix(last, "  Help  │") ||
		strings.Trim(Plain(tall.Lines[30].Text), " │") != "" {
		t.Errorf("toolbar not at the bottom:\n%s", screenText(tall))
	}
	// Narrow: pills become "!".
	if n := screenText(Render(model(), 25, 0, NoItem)); strings.Contains(n, "Needs you") {
		t.Errorf("narrow:\n%s", n)
	}

	// The keyboard moves over the rail and the rows; a selection is drawn in
	// reverse video, a button on its own.
	items := sc.Items()
	if a, b := sc.ActionOf(items[0]), sc.ActionOf(items[1]); a != (Action{Kind: ActView, Section: ViewAgents}) || b != (Action{Kind: ActFold, Section: SecProjects}) {
		t.Fatalf("first items %+v %+v", a, b)
	}
	var focus Item
	for _, it := range items {
		if sc.ActionOf(it).Kind == ActFocus {
			focus = it
			break
		}
	}
	sel := Render(model(), 41, 0, focus)
	if lt := sel.Lines[focus.Row].Text; !strings.Contains(lt, rev) || DisplayWidth(Plain(lt)) != 41 {
		t.Errorf("selection %q", lt)
	}
	last := items[len(items)-1]
	if sc.ActionOf(last).Kind != ActKeys {
		t.Fatalf("last item %+v", sc.ActionOf(last))
	}
	f := Frame(sc, 41, 10)
	if !strings.HasPrefix(f, "\x1b[H") || !strings.HasSuffix(f, "\x1b[J") || strings.Count(f, "\r\n") != 9 {
		t.Errorf("frame %q", f)
	}
}

// The Explorer view: the tree with folders first, open folders listed
// inside, git colors and folder marks; files and folders act on click.
func TestExplorer(t *testing.T) {
	i18n.SetLocale("en")
	root := t.TempDir()
	for _, d := range []string{"src/auth", "node_modules/x", ".git", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"src/auth/redirect.ts", "src/main.ts", "README.md", ".env", "docs/a.md"} {
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	changes := []ChangeRow{{Status: "M", Path: "src/auth/redirect.ts"}, {Status: "?", Path: "README.md"}}
	tree := BuildTree(root, map[string]bool{"src": true}, changes)
	var names []string
	for _, r := range tree {
		names = append(names, strings.Repeat(">", r.Depth)+r.Name+r.Git)
	}
	if strings.Join(names, " ") != "docs src* >auth* >main.ts README.md?" {
		t.Fatalf("tree %v", names)
	}
	m := model()
	m.View, m.Tree = ViewExplorer, tree
	for _, w := range []int{25, 38} {
		for _, l := range Render(m, w, 12, NoItem).Lines {
			if n := DisplayWidth(Plain(l.Text)); n != w {
				t.Errorf("width %d: %d columns: %q", w, n, Plain(l.Text))
			}
		}
	}
	sc := Render(m, 41, 12, NoItem)
	text := screenText(sc)
	t.Logf("\n%s", text)
	for _, want := range []string{"EXPLORER · acme-api", "🔍", "▸ 📁 docs", "▾ 📂 src", "│ ▸ 📁 auth", "│   🟦 main.ts", "  📖 README.md", "▌  📁"} {
		if !strings.Contains(text, want) {
			t.Errorf("explorer lacks %q", want)
		}
	}
	find := func(sub string) Action {
		for _, l := range sc.Lines {
			if i := strings.Index(Plain(l.Text), sub); i >= 0 {
				return l.ActionAt(DisplayWidth(Plain(l.Text)[:i]))
			}
		}
		t.Fatalf("no %q", sub)
		return Action{}
	}
	for _, l := range sc.Lines {
		if p := strings.TrimRight(Plain(l.Text), " "); strings.Contains(p, "README.md") && !strings.HasSuffix(p, "?") {
			t.Errorf("untracked mark %q", p)
		}
	}
	if a := find("auth"); a != (Action{Kind: ActToggleDir, Path: filepath.FromSlash("src/auth")}) {
		t.Errorf("folder %+v", a)
	}
	if a := find("main.ts"); a != (Action{Kind: ActOpenFile, Path: filepath.FromSlash("src/main.ts")}) {
		t.Errorf("file %+v", a)
	}
	if a := find("🔍"); a.Kind != ActView && a.Kind != ActSearch {
		t.Errorf("search %+v", a)
	}
	if a := find("📄"); a.Kind != ActGoto {
		t.Errorf("go to file %+v", a)
	}
	// Scrolling keeps the title and moves the list.
	m.Scroll = 2
	if text := screenText(Render(m, 41, 3, NoItem)); !strings.Contains(text, "EXPLORER") || !strings.Contains(text, "auth") || strings.Contains(text, "docs") {
		t.Errorf("scrolled:\n%s", text)
	}
}

// The Source Control view: branch and sync, Commit, staged and unstaged
// changes with their buttons, history; without git it says so.
func TestChangesView(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.View = ViewChanges
	m.SCM = &gitops.Status{
		Branch: "main", Upstream: "origin/main", Ahead: 1,
		Staged:  []gitops.Change{{Status: "M", Path: "a.go", Added: 3, Removed: 1}},
		Changes: []gitops.Change{{Status: "M", Path: "b.go", Added: 1}, {Status: "?", Path: "c.txt", Untracked: true}},
		Graph:   []gitops.GraphRow{{Hash: "a1b2c3d", Subject: "feat: login", When: m.Now.Add(-2 * time.Hour)}},
	}
	for _, l := range Render(m, 41, 30, NoItem).Lines {
		if n := DisplayWidth(Plain(l.Text)); n != 41 {
			t.Errorf("%d columns: %q", n, Plain(l.Text))
		}
	}
	sc := Render(m, 41, 0, NoItem)
	text := screenText(sc)
	t.Logf("\n%s", text)
	for _, want := range []string{
		"SOURCE CONTROL", "🔄", "🌿 main ▾", "↑1 ↓0", "🔃", "✅ Commit…", "▾ STAGED CHANGES  1", "M  a.go", "+3 −1",
		"▾ CHANGES  2", "M  b.go", "U  c.txt", "▾ HISTORY  1", "feat: login", "2h", "▌  🔀",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q", want)
		}
	}
	// Each button acts on its file.
	at := func(sub string, dx int) Action {
		for _, l := range sc.Lines {
			p := Plain(l.Text)
			if i := strings.Index(p, sub); i >= 0 {
				return l.ActionAt(DisplayWidth(p[:i]) + dx)
			}
		}
		t.Fatalf("no %q", sub)
		return Action{}
	}
	lineEnd := func(sub string) Line {
		for _, l := range sc.Lines {
			if strings.Contains(Plain(l.Text), sub) {
				return l
			}
		}
		t.Fatalf("no %q", sub)
		return Line{}
	}
	if a := at("a.go", 0); a != (Action{Kind: ActDiff, Path: "a.go", Status: "M", Section: "staged"}) {
		t.Errorf("staged row %+v", a)
	}
	if a := lineEnd("a.go").ActionAt(39); a != (Action{Kind: ActGit, Section: "unstage", Path: "a.go"}) {
		t.Errorf("unstage button %+v", a)
	}
	if a := lineEnd("b.go").ActionAt(37); a != (Action{Kind: ActGit, Section: "stage", Path: "b.go"}) {
		t.Errorf("stage button %+v", a)
	}
	if a := lineEnd("c.txt").ActionAt(33); a != (Action{Kind: ActGitConfirm, Section: "discard", Path: "c.txt", Status: "?"}) {
		t.Errorf("discard button %+v", a)
	}
	if a := lineEnd("CHANGES  2").ActionAt(39); a != (Action{Kind: ActGit, Section: "stage-all"}) {
		t.Errorf("stage all %+v", a)
	}
	if a := at("Commit…", 0); a.Kind != ActCommit {
		t.Errorf("commit %+v", a)
	}
	if a := at("🌿 main", 0); a != (Action{Kind: ActSCM, Section: "pick", Path: "checkout"}) {
		t.Errorf("branch %+v", a)
	}
	if a := at("🔃", 0); a != (Action{Kind: ActGitPopup, Section: "sync"}) {
		t.Errorf("sync %+v", a)
	}
	if a := at("feat: login", 0); a != (Action{Kind: ActGitPopup, Section: "show", Path: "a1b2c3d"}) {
		t.Errorf("history %+v", a)
	}
	// Folding a section hides its rows.
	m.Fold = map[string]bool{SecUnstaged: true}
	if text := screenText(Render(m, 41, 0, NoItem)); strings.Contains(text, "b.go") || !strings.Contains(text, "▸ CHANGES  2") {
		t.Errorf("folded:\n%s", text)
	}
	m.SCM = nil
	if text := screenText(Render(m, 41, 0, NoItem)); !strings.Contains(text, "Not a git repository") {
		t.Errorf("no git:\n%s", text)
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
	want := []ChangeRow{{Status: "M", Path: "src/a.go"}, {Status: "A", Path: "b.go"}, {Status: "?", Path: "notes.md"}, {Status: "R", Path: "new.go"}, {Status: "M", Path: "both.go"}, {Status: "?", Path: "with space.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestMoveWraps(t *testing.T) {
	i18n.SetLocale("en")
	s := &sidebar{screen: Render(model(), 41, 0, NoItem), sel: NoItem}
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

// TestClickAtOnce: a click acts at once, also when it shows another window
// or project or opens a popup; one that opens a tmux menu waits where menus
// take no mouse (before tmux 3.5), one that opens the command prompt always.
func TestClickAtOnce(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	defer func(d time.Duration) { OpenAfter = d }(OpenAfter)
	OpenAfter = time.Hour
	ctx := context.Background()
	tc := &tmux.Client{Bin: "tmux", Runner: tmux.ExecRunner{}}
	menus := !tc.MenuTakesMouse(ctx)
	s := &sidebar{t: tc, sel: NoItem, clicks: make(chan Action, 4), model: Model{Panes: []PaneRow{{ID: "%1"}}}}
	for i, c := range []struct {
		a    Action
		wait bool
	}{
		{Action{Kind: ActView, Section: ViewExplorer}, false},
		{Action{Kind: ActToggleDir, Path: "src"}, false},
		{Action{Kind: ActFocus, Pane: "%1"}, false},
		{Action{Kind: ActFocus, Pane: "%9"}, false}, // in another window
		{Action{Kind: ActProject, Session: "other"}, false},
		{Action{Kind: ActSwitch, Agent: "codex"}, false},
		{Action{Kind: ActOpenFile, Path: "a.go"}, false}, // a popup
		{Action{Kind: ActSearch}, false},
		{Action{Kind: ActNewAgent}, menus},
		{Action{Kind: ActSettings}, menus},
		{Action{Kind: ActRename}, true}, // the command prompt
	} {
		s.screen = Screen{Lines: []Line{{Text: "row", Action: c.a}}}
		s.lastClick = time.Time{}
		s.handle(ctx, Event{Kind: EvClick, X: 1, Y: 1})
		wait := 5 * time.Second
		if c.wait {
			wait = 100 * time.Millisecond
		}
		select {
		case a := <-s.clicks:
			if c.wait || a != c.a {
				t.Errorf("%d: %+v ran at once", i, a)
			}
		case <-time.After(wait):
			if !c.wait {
				t.Errorf("%d: %+v waited", i, c.a)
			}
		}
	}
}

// cmdRunner records the tmux calls it is given.
type cmdRunner struct{ calls []string }

func (r *cmdRunner) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	return nil, nil, nil
}

// TestSwitchMarksTheClient (D53): a click that shows the client another
// project or tab marks the client (@imux_moved, cleared by tmux 0.5 s
// later) in the same tmux call, before the switch; a pane of this window
// does not.
func TestSwitchMarksTheClient(t *testing.T) {
	ctx := context.Background()
	mark := "set-option -g @imux_moved /dev/pts/3 ; run-shell -b -C -d 0.5 set-option -gu @imux_moved ; switch-client -c /dev/pts/3 -t "
	for _, c := range []struct {
		a    Action
		want string // "" for no mark
	}{
		{Action{Kind: ActProject, Session: "beta-1"}, mark + "=beta-1"},
		{Action{Kind: ActFocus, Pane: "%9"}, mark + "%9 ; select-window -t %9 ; select-pane -t %9"},
		{Action{Kind: ActFocus, Pane: "%1"}, ""},
	} {
		r := &cmdRunner{}
		s := &sidebar{t: &tmux.Client{Bin: "tmux", Runner: r}, sel: NoItem, model: Model{Panes: []PaneRow{{ID: "%1"}}}, win: Window{Client: "/dev/pts/3"}}
		s.do(ctx, c.a)
		got := strings.Join(r.calls, "\n")
		if c.want == "" && strings.Contains(got, "@imux_moved") || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%+v ran %q", c.a, got)
		}
	}
}

// realTmux starts a private tmux server for the test.
func realTmux(t *testing.T) *tmux.Client {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("real tmux tests: Windows runs psmux, covered by the tmux package's psmux tests")
	}
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
	if strings.Join(ids, ",") != "claude,antigravity,copilot,codex,opencode" {
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

// A folded section shows only its title with a count; the title folds and
// unfolds it.
func TestFold(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.Fold = map[string]bool{SecProjects: true, SecChanges: true}
	text := screenText(Render(m, 41, 0, NoItem))
	t.Logf("\n%s", text)
	lineWith := func(sub string) string {
		for _, l := range strings.Split(text, "\n") {
			if strings.Contains(l, sub) {
				return l
			}
		}
		return ""
	}
	if l := lineWith("▸ PROJECTS"); !strings.HasSuffix(l, "2  ➕") {
		t.Errorf("projects title %q", l)
	}
	if lineWith("▾ AGENTS") == "" {
		t.Error("agents stay open")
	}
	for _, gone := range []string{" A  acme-api"} {
		if strings.Contains(text, gone) {
			t.Errorf("folded section shows %q", gone)
		}
	}
}

func TestParseNumstat(t *testing.T) {
	got := ParseNumstat("12\t3\tsrc/a.go\n-\t-\timg.png\n4\t0\told.go => new.go\n1\t1\tsrc/{x => y}/z.go\nbad\n")
	want := map[string][2]int{"src/a.go": {12, 3}, "img.png": {0, 0}, "new.go": {4, 0}, "src/y/z.go": {1, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}

// The rail: a line between it and the view on every row; the editor's
// icons with a Nerd Font.
func TestRailThemes(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	for _, l := range Render(m, 41, 30, NoItem).Lines {
		if p := Plain(l.Text); !strings.Contains(p, "│") {
			t.Errorf("no line between the rail and the view: %q", p)
		}
	}
	m.Icons = IconsNerd
	text := screenText(Render(m, 41, 30, NoItem))
	for _, ic := range railIcons[IconsNerd] {
		if !strings.Contains(text, ic) {
			t.Errorf("nerd icon %U missing", []rune(ic)[0])
		}
	}
	if strings.Contains(text, "🤖") {
		t.Error("emoji with the nerd theme")
	}
	for _, l := range Render(m, 41, 30, NoItem).Lines {
		if n := DisplayWidth(Plain(l.Text)); n != 41 {
			t.Errorf("nerd: %d columns: %q", n, Plain(l.Text))
		}
	}

	s := &Source{Cfg: config.Default()}
	s.Cfg.Tmux.Icons = "emoji"
	if s.iconTheme("xterm-ghostty", "") != IconsEmoji {
		t.Error("configured emoji wins")
	}
	s.Cfg.Tmux.Icons = "auto"
	if s.iconTheme("xterm-ghostty", "") != IconsNerd || s.iconTheme("xterm-kitty", "") != IconsNerd {
		t.Error("terminals with Nerd Font symbols")
	}
	// Terminal.app: the icons follow its font, asked once.
	old := NerdFontFor
	defer func() { NerdFontFor = old }()
	asked := 0
	NerdFontFor = func(p string) bool { asked++; return p == "Apple_Terminal" }
	first := s.iconTheme("xterm-256color", "Apple_Terminal")
	if first != IconsNerd || s.iconTheme("xterm-256color", "Apple_Terminal") != IconsNerd || asked != 1 {
		t.Errorf("Terminal.app with a Nerd Font (asked %d)", asked)
	}
	if s.iconTheme("xterm-256color", "vscode") != IconsEmoji || s.iconTheme("xterm-256color", "") != IconsEmoji {
		t.Error("no Nerd Font: emoji")
	}
}

// TestClosedPanel: with the panel closed the sidebar is the rail alone, 6
// columns with no line of its own; its icons still open views, none marked
// active; the bottom buttons stay.
func TestClosedPanel(t *testing.T) {
	i18n.SetLocale("en")
	for _, theme := range []string{IconsEmoji, IconsNerd} {
		m := model()
		m.Icons, m.Closed, m.View = theme, true, ViewExplorer
		sc := Render(m, ClosedWidth, 30, NoItem)
		if len(sc.Lines) != 30 {
			t.Fatalf("%s: %d lines", theme, len(sc.Lines))
		}
		views := map[string]bool{}
		bottom := map[ActionKind]bool{}
		for _, l := range sc.Lines {
			p := Plain(l.Text)
			if n := DisplayWidth(p); n != ClosedWidth {
				t.Errorf("%s: %d columns: %q", theme, n, p)
			}
			if strings.Contains(p, "│") || strings.Contains(p, "PROJECTS") || strings.Contains(l.Text, "▌") {
				t.Errorf("%s: more than the rail, or an active mark: %q", theme, p)
			}
			for x := 0; x < ClosedWidth; x++ {
				switch a := l.ActionAt(x); a.Kind {
				case ActView:
					views[a.Section] = true
				case ActLimits, ActKeys, ActSearch:
					bottom[a.Kind] = true
				}
			}
		}
		for _, v := range []string{ViewAgents, ViewExplorer, ViewChanges, ViewSkills} {
			if !views[v] {
				t.Errorf("%s: no icon opens %s", theme, v)
			}
		}
		if !bottom[ActLimits] || !bottom[ActKeys] || !bottom[ActSearch] {
			t.Errorf("%s: search, limits or help gone: %v", theme, bottom)
		}
	}
}

// TestButtonSizes: in both themes every view keeps its width and every
// button takes at least 3 columns; the nerd theme draws the editor's icons.
func TestButtonSizes(t *testing.T) {
	i18n.SetLocale("en")
	for _, theme := range []string{IconsEmoji, IconsNerd} {
		for _, view := range []string{ViewAgents, ViewExplorer, ViewChanges, ViewSkills} {
			m := model()
			m.Icons, m.View = theme, view
			m.Tree = []TreeRow{{Path: "src", Name: "src", Dir: true}, {Path: "src/a.go", Name: "a.go", Depth: 1, Git: "M"}}
			m.Skills = []skills.Skill{{Name: "release-notes", Folder: "release-notes", Description: "Writes release notes.", Dir: "/home/test/acme-api/.agents/skills/release-notes", Shared: true}}
			m.SCM = &gitops.Status{
				Branch: "main", Upstream: "origin/main",
				Staged:  []gitops.Change{{Status: "M", Path: "a.go", Added: 3}},
				Changes: []gitops.Change{{Status: "?", Path: "c.txt", Untracked: true}},
			}
			sc := Render(m, 41, 40, NoItem)
			for _, l := range sc.Lines {
				if n := DisplayWidth(Plain(l.Text)); n != 41 {
					t.Errorf("%s/%s: %d columns: %q", theme, view, n, Plain(l.Text))
				}
				for _, c := range l.Cells {
					if c.Action.Kind != ActNone && c.W < 3 {
						t.Errorf("%s/%s: a %d-column button %q", theme, view, c.W, Plain(c.Text))
					}
				}
			}
			if theme == IconsNerd && view == ViewChanges {
				text := screenText(sc)
				for _, ic := range []string{"", "", "", "", "", "", "", "", "", ""} {
					if !strings.Contains(text, ic) {
						t.Errorf("nerd: icon %U missing", []rune(ic)[0])
					}
				}
				for _, old := range []string{"⟳", "↶", "⇅", "⎇"} {
					if strings.Contains(text, old) {
						t.Errorf("nerd: old sign %q", old)
					}
				}
			}
		}
	}
}

// TestNoToolbarRename: renaming is on the agent's row, not in the toolbar.
func TestNoToolbarRename(t *testing.T) {
	i18n.SetLocale("en")
	sc := Render(model(), 41, 40, NoItem)
	for _, l := range sc.Lines {
		for _, c := range l.Cells {
			if c.Action.Kind == ActRename {
				t.Fatalf("a Rename button: %q", Plain(l.Text))
			}
		}
		if l.Action.Kind == ActRename {
			t.Fatalf("a Rename row: %q", Plain(l.Text))
		}
	}
}

// TestWideText: wide emoji and CJK in commit subjects, file and agent names
// never make a row wider than the sidebar (a ✨ in a subject wrapped the
// row and broke the rail).
func TestWideText(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.View = ViewChanges
	m.SCM = &gitops.Status{
		Branch:  "main",
		Changes: []gitops.Change{{Status: "M", Path: "文档/说明书.md", Added: 3}},
		Graph: []gitops.GraphRow{
			{Hash: "c7c81ba", Subject: "feat: ✨ Commit message suggestions from the user's agents", When: m.Now},
			{Hash: "a1b2c3d", Subject: "修复：标签页的关闭按钮 🎉🎉🎉 and more", When: m.Now},
		},
	}
	for _, theme := range []string{IconsEmoji, IconsNerd} {
		m.Icons = theme
		for _, w := range []int{43, 30} {
			for _, l := range Render(m, w, 30, NoItem).Lines {
				if n := DisplayWidth(Plain(l.Text)); n != w {
					t.Errorf("%s %d: %d columns: %q", theme, w, n, Plain(l.Text))
				}
			}
		}
	}
	if got := fit("ab✨cd", 4); DisplayWidth(got) != 4 || got != "ab …" {
		t.Errorf("fit cuts by columns: %q", got)
	}
	if got := fit("漢字テスト", 5); DisplayWidth(got) != 5 {
		t.Errorf("fit CJK: %q (%d)", got, DisplayWidth(got))
	}
	line := "\x1b[33mabc\x1b[0m ✨✨✨ end"
	if c := clip(line, 6); DisplayWidth(Plain(c)) != 6 || !strings.HasPrefix(c, "\x1b[33mabc\x1b[0m ") {
		t.Errorf("clip %q (%d)", c, DisplayWidth(Plain(c)))
	}
	if clip("short", 10) != "short" {
		t.Error("clip changed a short line")
	}
	if DisplayWidth("é") != 1 || DisplayWidth("👍🏽") != 2 || DisplayWidth("❤️") != 1 {
		t.Errorf("combining marks, skin tones, variation selectors: %d %d %d", DisplayWidth("é"), DisplayWidth("👍🏽"), DisplayWidth("❤️"))
	}
}

// TestFileIcons (M44): the explorer marks entries by type like the editor:
// whole names first (agent files, manifests, lock files), README and
// LICENSE, tests, then extensions, any case; folders open and closed. The
// editor's icons are colored glyphs one column wide, emoji two, so names
// stay aligned; indent guides mark the levels.
func TestFileIcons(t *testing.T) {
	for name, want := range map[string]string{
		"main.go": "🐹", "main_test.go": "🧪", "app.spec.ts": "🧪", "test_app.py": "🧪", "README.md": "📖", "readme": "📖",
		"README.tr.md": "📖", "LICENSE": "📜", "AGENTS.md": "🤖", "CLAUDE.md": "🤖", "SKILL.md": "🧩", "go.mod": "📦",
		"go.sum": "🔒", "Makefile": "🔨", "Dockerfile": "🐳", "notes.md": "📝", "default.toml": "🔧", "data.JSON": "📋",
		"photo.PNG": "📷", "install.sh": "🐚", "noext": "📄", "x.unknown": "📄",
	} {
		if got := iconOf(name, false, false).emoji; got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
	if iconOf("src", true, false).emoji != "📁" || iconOf("src", true, true).emoji != "📂" {
		t.Error("folders")
	}
	if g, w := iconOf("main.go", false, false).draw(IconsNerd); g != icCyan+"\ue627"+reset || w != 1 {
		t.Errorf("the editor's Go icon: %q %d", g, w)
	}
	if ic := iconOf("x_test.go", false, false); ic.glyph != gBeaker || ic.color != icCyan {
		t.Errorf("a test keeps its language's color: %+v", ic)
	}
	for _, m := range []map[string]fileIcon{fileNames, fileExts} {
		for k, ic := range m {
			if DisplayWidth(ic.glyph) != 1 || DisplayWidth(ic.emoji) != 2 {
				t.Errorf("%s: %q %q", k, ic.glyph, ic.emoji)
			}
		}
	}
	for _, theme := range []string{IconsNerd, IconsEmoji} {
		l := treeRow(TreeRow{Path: "a/b/c.go", Name: "c.go", Depth: 2, Git: "M"}, 41, buttonIcons[theme], theme, false)
		p := Plain(l.Text)
		icon, _ := iconOf("c.go", false, false).draw(theme)
		if !strings.HasPrefix(p, " │ │   "+Plain(icon)+" c.go") || !strings.HasSuffix(p, " M ") || DisplayWidth(p) != 41 {
			t.Errorf("%s row: %q (%d columns)", theme, p, DisplayWidth(p))
		}
	}
}

// TestHoverAndSelection (T34.1): the item under the pointer gets the hover
// background, a rail item's icon and label together; the edges are no
// item; the selected explorer row keeps the selection background.
func TestHoverAndSelection(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.View = ViewExplorer
	m.Tree = []TreeRow{{Path: "docs", Name: "docs", Dir: true}, {Path: "main.go", Name: "main.go"}, {Path: "x.go", Name: "x.go"}}
	m.Selected = "main.go"
	w, h := 41, 30
	sc := Render(m, w, h, NoItem)
	rowOf := func(sub string) int {
		for i, l := range sc.Lines {
			if strings.Contains(Plain(l.Text), sub) {
				return i
			}
		}
		t.Fatalf("no %q", sub)
		return -1
	}
	if r := rowOf("main.go"); !strings.Contains(sc.Lines[r].Text, selectBg) {
		t.Errorf("selected row without the selection background: %q", sc.Lines[r].Text)
	}
	if r := rowOf("x.go"); strings.Contains(sc.Lines[r].Text, selectBg) {
		t.Error("an unselected row selected")
	}

	// The x.go row: hovered from its middle, nothing on the edges.
	y := rowOf("x.go") + 1
	it := sc.HoverAt(20, y, w)
	if it == NoItem {
		t.Fatal("no item under the pointer")
	}
	for _, x := range []int{1, w} {
		if sc.HoverAt(x, y, w) != NoItem {
			t.Errorf("an item on edge column %d", x)
		}
	}
	if sc.HoverAt(20, h+5, w) != NoItem {
		t.Error("an item below the screen")
	}
	Paint(sc, it, hoverBg, w)
	if l := sc.Lines[y-1].Text; !strings.Contains(l, hoverBg) || DisplayWidth(Plain(l)) != w {
		t.Errorf("hovered row %q", l)
	}
	if strings.Contains(sc.Lines[rowOf("docs")].Text, hoverBg) {
		t.Error("the hover spilled onto another row")
	}

	// A rail item: its icon and label light up together.
	sc = Render(m, w, h, NoItem)
	icon := -1
	for i, l := range sc.Lines {
		if a := l.ActionAt(3); a.Kind == ActView && a.Section == ViewChanges {
			icon = i
			break
		}
	}
	if icon < 0 {
		t.Fatal("no Source Control item on the rail")
	}
	Paint(sc, sc.HoverAt(4, icon+1, w), hoverBg, w)
	lit := 0
	for _, l := range sc.Lines {
		if strings.Contains(l.Text, hoverBg) {
			lit++
		}
	}
	if lit != 2 {
		t.Errorf("%d rail rows lit, want the icon and its label", lit)
	}
}

// TestLeaving (T34.1): a pointer that stopped right after a step that,
// repeated, leaves the sidebar was on its way out; one at rest inside was
// not.
func TestLeaving(t *testing.T) {
	cases := []struct {
		px, py, x, y int
		want         bool
	}{
		{27, 5, 37, 5, true},   // fast to the right, 10 columns a report
		{38, 5, 40, 5, true},   // slowly, at the last columns
		{20, 5, 21, 5, false},  // slowly, in the middle
		{12, 5, 4, 5, true},    // to the left, out of the terminal
		{20, 3, 20, 1, true},   // up, to a status line on top
		{20, 30, 20, 34, true}, // down
		{20, 5, 20, 5, false},  // a report without a move
		{0, 0, 20, 5, false},   // the first report
	}
	for _, c := range cases {
		if got := Leaving(c.px, c.py, c.x, c.y, 41, 35); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

// TestFileMenus (T34.2): the explorer's menus have the editor's items, only
// those that apply, and no path in their commands.
func TestFileMenus(t *testing.T) {
	i18n.SetLocale("en")
	names := func(items []string) string {
		var out []string
		for i := 0; i < len(items); {
			if items[i] == "" {
				out = append(out, "-")
				i++
				continue
			}
			out = append(out, items[i])
			if strings.Contains(items[i+2], "/home/") {
				t.Errorf("a path in the command of %q: %s", items[i], items[i+2])
			}
			i += 3
		}
		return strings.Join(out, "|")
	}
	st := MenuState{Kind: MenuFile, Agent: true, Path: "/home/u/p/a.go", Reveal: "file.reveal_finder", VSCode: true}
	got := names(FileMenuItems("/bin/imux", "/dev/ttys1", "%3", st))
	want := "Open|Open to the Side|Open in VS Code|View|-|Reveal in Finder|Open in Terminal|Add to Agent|-|Select for Compare|-|Cut|Copy|-|Copy Path|Copy Relative Path|-|Rename…|Delete"
	if got != want {
		t.Errorf("file menu\n got %s\nwant %s", got, want)
	}
	// A web page or an SVG: Open Preview, the live preview (D47).
	for _, p := range []string{"/home/u/p/index.html", "/home/u/p/logo.svg"} {
		web := MenuState{Kind: MenuFile, Path: p}
		if got := names(FileMenuItems("/bin/imux", "", "%3", web)); !strings.HasPrefix(got, "Open|Open to the Side|View|Open Preview|-") {
			t.Errorf("%s menu %s", p, got)
		}
	}
	if got := names(FileMenuItems("/bin/imux", "", "%3", st)); strings.Contains(got, "Preview") {
		t.Errorf("a .go file previews: %s", got)
	}
	st.Clip, st.Compare = true, "/home/u/p/b.go"
	if got := names(FileMenuItems("/bin/imux", "", "%3", st)); !strings.Contains(got, "Compare with 'b.go'") || !strings.Contains(got, "Paste") {
		t.Errorf("with a clip and a compare: %s", got)
	}
	st = MenuState{Kind: MenuFolder, Path: "/home/u/p/src"}
	if got := names(FileMenuItems("/bin/imux", "", "%3", st)); got != "New File…|New Folder…|-|Open in Terminal|Find in Folder…|-|Cut|Copy|-|Copy Path|Copy Relative Path|-|Rename…|Delete" {
		t.Errorf("folder menu %s", got)
	}
	st = MenuState{Kind: MenuRoot, Clip: true, Agent: true, Path: "/home/u/p"}
	items := FileMenuItems("/bin/imux", "", "", st)
	if got := names(items); got != "New File…|New Folder…|-|Find in Folder…|-|Paste|-|Copy Path|-|Collapse Folders" {
		t.Errorf("root menu %s", got)
	}
	// A click on an HTML file asks how to open it (D51).
	html := HTMLMenuItems("/bin/imux", "/dev/ttys1", "%3")
	if got := names(html); got != "Open in browser|Open in a new tab|-|Edit|View" {
		t.Errorf("html menu %s", got)
	}
	for i, op := range map[int]string{2: "preview-window", 5: "preview-tab", 9: "open", 12: "view"} {
		if !strings.Contains(html[i], "__file "+op+" --client /dev/ttys1 --pane %3") {
			t.Errorf("html item %d: %s", i, html[i])
		}
	}
	if x, y := MenuAt(5, 7); x != "#{e|+:#{popup_pane_left},4}" || y != "#{e|+:#{popup_pane_top},6}" {
		t.Errorf("position %s %s", x, y)
	}
	for _, it := range FileMenuItems("/bin/imux", "/dev/ttys1", "%3", MenuState{Kind: MenuFile}) {
		if strings.Contains(it, "__file") && !strings.Contains(it, "--client /dev/ttys1 --pane %3") {
			t.Errorf("command without its client and pane: %s", it)
		}
	}
}

// TestSkillsViewAndMenu (M40, M42): the Skills view shows the empty hint, one
// full-width ✨ New skill… button, the skills with their agent badges and
// problems, and the skill menus carry no path or pane id in their commands.
func TestSkillsViewAndMenu(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.View = ViewSkills
	sc := Render(m, 41, 30, NoItem)
	text := screenText(sc)
	for _, want := range []string{"SKILLS · acme-api", "✨ New skill…", "No skills in this project", ".agents/skills/<id>/SKILL.md", "Click ✨ above to write one"} {
		if !strings.Contains(text, want) {
			t.Errorf("empty skills view lacks %q:\n%s", want, text)
		}
	}
	// One button, the panel's whole width: no second (➕) button next to it.
	for _, l := range sc.Lines {
		if !strings.Contains(Plain(l.Text), "New skill…") {
			continue
		}
		for x := railW; x < 41; x++ {
			if a := l.ActionAt(x); a.Kind != ActSkillNew {
				t.Errorf("column %d of the New skill row: %+v", x, a)
			}
		}
	}

	all := map[session.AgentID]bool{session.Claude: true, session.Antigravity: true, session.Copilot: true, session.Codex: true, session.OpenCode: true}
	some := map[session.AgentID]bool{session.Claude: true, session.Copilot: true, session.OpenCode: true}
	m.Skills = []skills.Skill{
		{Name: "release-notes", Folder: "release-notes", Description: "Writes release notes.", Dir: "/home/test/acme-api/.agents/skills/release-notes", Shared: true, Agents: all},
		{Name: "draft", Folder: "draft", Dir: "/home/test/acme-api/.claude/skills/draft", Problem: skills.ProbNoDesc, Agents: some},
	}
	m.Selected = "/home/test/acme-api/.agents/skills/release-notes"
	sc = Render(m, 41, 30, NoItem)
	text = screenText(sc)
	for _, want := range []string{"SKILLS · 2", "● release-notes", "✻▲◉◎▣", "Writes release notes.", "⚠ draft", "✻·◉·▣", "Missing 'description'"} {
		if !strings.Contains(text, want) {
			t.Errorf("skills view lacks %q:\n%s", want, text)
		}
	}
	var sawOpen, sawMenu, sawNew, sawSel bool
	for _, l := range sc.Lines {
		if strings.Contains(Plain(l.Text), "release-notes") && strings.Contains(l.Text, selectBg) {
			sawSel = true
		}
		for x := railW; x < 41; x++ {
			switch a := l.ActionAt(x); a.Kind {
			case ActOpenFile:
				if a.Path == filepath.FromSlash(".agents/skills/release-notes/SKILL.md") {
					sawOpen = true
				}
			case ActSkillMenu:
				if a.Path == "/home/test/acme-api/.agents/skills/release-notes" {
					sawMenu = true
				}
			case ActSkillNew:
				sawNew = true
			}
		}
	}
	if !sawOpen || !sawMenu || !sawNew || !sawSel {
		t.Errorf("actions open=%v menu=%v new=%v sel=%v", sawOpen, sawMenu, sawNew, sawSel)
	}

	names := func(items []string) string {
		var out []string
		for i := 0; i < len(items); {
			if items[i] == "" {
				out = append(out, "-")
				i++
				continue
			}
			out = append(out, items[i])
			if strings.Contains(items[i+2], "/home/") || strings.Contains(items[i+2], "%") {
				t.Errorf("path or pane id in %q: %s", items[i], items[i+2])
			}
			i += 3
		}
		return strings.Join(out, "|")
	}
	if got := names(SkillMenuItems("/bin/imux", "/dev/ttys1", SkillMenuState{Agent: true, Shared: false})); got != "Open|View|-|Add to Agent|Improve with Agent|Share with all agents (.agents/skills)|-|Delete" {
		t.Errorf("unshared skill menu: %s", got)
	}
	if got := names(SkillMenuItems("/bin/imux", "", SkillMenuState{Agent: false, Shared: true})); got != "Open|View|-|Delete" {
		t.Errorf("shared skill menu without agent: %s", got)
	}
	// The empty space's menu opens the dialog, at its first step or at the
	// blank template's name.
	root := SkillMenuItems("/bin/imux", "", SkillMenuState{Root: true})
	if got := names(root); got != "New skill…|Blank SKILL.md…" {
		t.Errorf("root menu: %s", got)
	}
	if !strings.Contains(root[2], "__skill dialog") || strings.Contains(root[2], "--template") || !strings.Contains(root[5], "__skill dialog --template") {
		t.Errorf("root menu commands: %q, %q", root[2], root[5])
	}
}
