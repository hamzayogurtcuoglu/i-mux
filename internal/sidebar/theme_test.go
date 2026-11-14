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
