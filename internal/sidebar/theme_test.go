package sidebar

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
)

// TestParseTerminalReplies (T73.4): the terminal's answers are events,
// never keys: OSC 11 ended by BEL or ST (or cut between two reads), and
// DEC 2031's ?997;1n / ?997;2n.
func TestParseTerminalReplies(t *testing.T) {
	evs, rest := Parse([]byte("a\x1b]11;rgb:1e1e/1e1e/2e2e\x07b\x1b]11;rgb:ffff/ffff/ffff\x1b\\\x1b[?997;2n\x1b[?997;1n"), true)
	var got []string
	for _, e := range evs {
		got = append(got, string(rune('0'+e.Kind))+":"+e.Key+":"+e.Text)
	}
	want := []string{"0:a:", "7:11:rgb:1e1e/1e1e/2e2e", "0:b:", "7:11:rgb:ffff/ffff/ffff", "8:light:", "8:dark:"}
	if strings.Join(got, " ") != strings.Join(want, " ") || len(rest) != 0 {
		t.Fatalf("events %q, rest %q", got, rest)
	}
	evs, rest = Parse([]byte("\x1b]11;rgb:ffff/ff"), true)
	if len(evs) != 0 || string(rest) != "\x1b]11;rgb:ffff/ff" {
		t.Fatalf("a cut reply: %v %q", evs, rest)
	}
	if evs, _ = Parse(append(rest, []byte("ff/ffff\x07x")...), true); len(evs) != 2 || evs[0].Kind != EvColor || evs[1].Key != "x" {
		t.Fatalf("the reply's end: %+v", evs)
	}
}

// TestPaletteColors (T73.3): the sidebar draws with the theme's colors and
// follows it when it changes.
func TestPaletteColors(t *testing.T) {
	defer theme.Set(theme.Builtin(theme.Default))
	m := model()
	// The default: i-mux's colors before themes, exactly.
	var classic strings.Builder
	for _, l := range Render(m, 41, 20, NoItem).Lines {
		classic.WriteString(l.Text)
	}
	if c := classic.String(); !strings.Contains(c, "\x1b[1m\x1b[38;5;252m") || !strings.Contains(c, "\x1b[38;5;247m") || strings.Contains(c, "\x1b[38;2;") {
		t.Errorf("not the classic colors:\n%q", c)
	}
	theme.Set(theme.Builtin("catppuccin"))
	text := func() string {
		var b strings.Builder
		for _, l := range Render(m, 41, 20, NoItem).Lines {
			b.WriteString(l.Text)
		}
		return b.String()
	}
	mocha := text()
	if !strings.Contains(mocha, theme.Hex("#cdd6f4").Fg()) || strings.Contains(mocha, "\x1b[38;5;") {
		t.Errorf("not Catppuccin's text, or a 256-color code left:\n%q", mocha)
	}
	theme.Set(theme.Builtin("gruvbox-light"))
	if light := text(); !strings.Contains(light, theme.Hex("#3c3836").Fg()) || strings.Contains(light, theme.Hex("#cdd6f4").Fg()) {
		t.Errorf("the theme change did not reach the sidebar:\n%q", light)
	}
	theme.Set(theme.Builtin("terminal"))
	if term := text(); strings.Contains(term, "\x1b[38;2;") {
		t.Errorf("24-bit colors with the terminal theme:\n%q", term)
	}
}

// TestAgentsInProjectColor (T95.1): AGENTS shows each project's avatar on
// its group row (the name stays muted) and its color as a bar on every agent
// row, in classic and in a theme; a row's width does not change, and agents
// on other machines keep their plain row.
func TestAgentsInProjectColor(t *testing.T) {
	defer theme.Set(theme.Builtin(theme.Default))
	for _, name := range []string{theme.Default, "catppuccin"} {
		theme.Set(theme.Builtin(name))
		m := model()
		colors := projectColors(m.Projects)
		lines := Render(m, 41, 0, NoItem).Lines
		for i, pr := range m.Projects {
			bar := projectFg(colors[i]) + "▎" + reset
			group, rows := -1, 0
			for j, l := range lines {
				if strings.Contains(l.Text, " "+avatar(pr.Name, colors[i])+" "+muted+pr.Name) {
					group = j
				}
				if group >= 0 && j > group && j <= group+len(pr.Panes) && strings.Contains(l.Text, bar) {
					rows++
				}
			}
			if group < 0 {
				t.Errorf("%s: no group row with %s's avatar", name, pr.Name)
			}
			if rows < len(pr.Panes) {
				t.Errorf("%s: %d of %s's %d agents carry its color", name, rows, pr.Name, len(pr.Panes))
			}
		}
		for _, l := range lines {
			if n := DisplayWidth(Plain(l.Text)); n != 41 {
				t.Errorf("%s: %d columns: %q", name, n, Plain(l.Text))
			}
		}
		if l := agentRow(m.Panes[0], 30, m.Now, 0, ""); strings.Contains(l.Text, "▎") {
			t.Errorf("%s: a row without a project has a bar: %q", name, l.Text)
		}
	}
}

// TestProjectColors (T95.1): open projects never share a color while there
// are colors enough, a project keeps the color its name gives it unless one
// above it has it, and a worktree takes its repository's.
func TestProjectColors(t *testing.T) {
	defer theme.Set(theme.Builtin(theme.Default))
	for _, name := range []string{theme.Default, "catppuccin"} {
		theme.Set(theme.Builtin(name))
		// Names whose hashes collide (blog and api, web and i-mux, ...).
		var ps []ProjectRow
		for _, n := range []string{"acme-api", "blog", "web", "api", "docs", "i-mux", "site", "app"}[:len(avatars)] {
			ps = append(ps, ProjectRow{Name: n})
		}
		seen := map[string]bool{}
		for _, c := range projectColors(ps) {
			if seen[projectFg(c)] {
				t.Errorf("%s: two of %d projects share %q", name, len(ps), projectFg(c))
			}
			seen[projectFg(c)] = true
		}
		alone := projectColors(ps[3:4])[0]
		if got := projectColors([]ProjectRow{ps[3], ps[0]})[0]; got != alone {
			t.Errorf("%s: the first project's color moved: %d, alone %d", name, got, alone)
		}
		wt := projectColors([]ProjectRow{ps[1], {Name: "feature-x", Worktree: true}, ps[2]})
		if wt[1] != wt[0] || wt[2] == wt[0] {
			t.Errorf("%s: worktree colors %v", name, wt)
		}
		// More projects than colors: every one still has one.
		many := projectColors(append(ps, ps...))
		if len(many) != 2*len(ps) {
			t.Fatalf("%s: %d colors for %d projects", name, len(many), 2*len(ps))
		}
	}
}
