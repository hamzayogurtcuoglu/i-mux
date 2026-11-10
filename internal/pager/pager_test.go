package pager

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

func lines(n int) []string {
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("\x1b[32mline %d\x1b[0m", i))
	}
	return out
}

func TestPagerScrolls(t *testing.T) {
	i18n.SetLocale("en")
	p := &Pager{Header: []string{" title"}, Lines: lines(100)}
	sc := p.Render(30, 12) // 10 rows of text
	if len(sc) != 12 || sidebar.Plain(sc[0]) != " title"+strings.Repeat(" ", 24) || !strings.HasPrefix(sidebar.Plain(sc[1]), "line 1 ") {
		t.Fatalf("screen %q", sc)
	}
	for _, l := range sc[1:11] {
		if w := sidebar.DisplayWidth(sidebar.Plain(l)); w != 30 {
			t.Fatalf("row of %d columns: %q", w, l)
		}
	}
	if !strings.HasSuffix(sidebar.Plain(sc[1]), "┃") || !strings.HasSuffix(sidebar.Plain(sc[10]), "│") {
		t.Errorf("no scroll bar: %q / %q", sc[1], sc[10])
	}
	if f := sidebar.Plain(sc[11]); !strings.Contains(f, "1–10 of 100 (10%)") {
		t.Errorf("footer %q", f)
	}
	for _, c := range []struct {
		key string
		top int
	}{{"down", 1}, {"pgdown", 10}, {"G", 90}, {"k", 89}, {"g", 0}, {"up", 0}} {
		p.Key(c.key)
		if p.Top != c.top {
			t.Errorf("%s: top %d, want %d", c.key, p.Top, c.top)
		}
	}
	// A click on the bar's last row goes to the end; the wheel scrolls.
	p.Mouse(sidebar.Event{Kind: sidebar.EvClick, X: 30, Y: 11})
	if p.Top != 90 {
		t.Errorf("click on the bar: top %d", p.Top)
	}
	p.Mouse(sidebar.Event{Kind: sidebar.EvScroll, Key: "up"})
	if p.Top != 87 {
		t.Errorf("wheel: top %d", p.Top)
	}
	if !p.Key("q") {
		t.Error("q did not close")
	}
	// Text that fits: no bar.
	short := &Pager{Lines: lines(3)}
	if sc := short.Render(20, 10); strings.Contains(sidebar.Plain(sc[0]), "┃") {
		t.Errorf("a bar on a short text: %q", sc[0])
	}
}

func TestPagerSearch(t *testing.T) {
	i18n.SetLocale("en")
	p := &Pager{Lines: lines(100)}
	p.Render(30, 11)
	for _, k := range []string{"/", "l", "i", "n", "e", " ", "4", "2", "enter"} {
		p.Key(k)
	}
	if p.Top != 41 || !strings.Contains(p.note, `1 of 1 for "line 42"`) {
		t.Fatalf("search: top %d note %q", p.Top, p.note)
	}
	row := p.mark(41)
	if !strings.Contains(row, hit+"line 42") || !strings.Contains(row, "\x1b[32m") {
		t.Errorf("the match is not marked, or the line lost its color: %q", row)
	}
	p.Key("/")
	for _, k := range []string{"l", "i", "n", "e", " ", "9", "enter"} {
		p.Key(k)
	}
	// From where it is (line 42): "line 90"; then on, around the end.
	first := p.Top
	p.Key("n")
	if first != 89 || p.Top != 90 {
		t.Errorf("line 9 at %d, next at %d", first, p.Top)
	}
	p.Key("N")
	p.Key("N")
	if p.Top != 8 {
		t.Errorf("previous twice at %d, want 8 (line 9)", p.Top)
	}
	p.Key("N")
	if p.Top != 98-8 && p.Top != 90 { // around: the last match, clamped to the end
		t.Errorf("around the top at %d", p.Top)
	}
}

func TestText(t *testing.T) {
	got := Text("a\tb\n\x1b[31mab\tc\x1b[0m\n")
	if len(got) != 2 || got[0] != "a   b" || sidebar.Plain(got[1]) != "ab  c" {
		t.Fatalf("%q", got)
	}
}

// v closes an editable pager with Edit set; elsewhere it does nothing.
func TestPagerEdit(t *testing.T) {
	p := &Pager{Lines: lines(50), Top: 7}
	p.Render(30, 11)
	if p.Key("v") || p.Edit {
		t.Fatal("v on a pager without a file")
	}
	p.Editable = true
	if !p.Key("v") || !p.Edit || p.Top != 7 {
		t.Fatalf("v: closed=%v edit=%v top=%d", p.Edit, p.Edit, p.Top)
	}
}
