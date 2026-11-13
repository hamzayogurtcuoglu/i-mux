package pager

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

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

// TestPagerMarks (T69.1): a diff's changes in two lanes left of the bar
// over the whole text, red where lines were removed and green where they
// were added; a click on a mark goes to its change, a few lines below the
// top, elsewhere as the bar; ] and [ go to the next and previous change
// around the ends and say which; rows keep their width.
func TestPagerMarks(t *testing.T) {
	i18n.SetLocale("en")
	marks := make([]Mark, 100)
	marks[40], marks[41], marks[42] = MarkAdded, MarkAdded, MarkAdded
	marks[80] = MarkRemoved | MarkAdded // a line replaced, side by side
	marks[90] = MarkRemoved
	p := &Pager{Lines: lines(100), Marks: marks}
	sc := p.Render(30, 12) // 11 rows of text, each a slice of about 9 lines
	for i, l := range sc[:11] {
		pl := sidebar.Plain(l)
		if w := sidebar.DisplayWidth(pl); w != 30 {
			t.Fatalf("row %d of %d columns: %q", i, w, pl)
		}
		lanes := string([]rune(pl)[27:29])
		want := map[int]string{4: " ▌", 8: "▐▌", 10: "▐ "}[i]
		if want == "" {
			want = "  "
		}
		if lanes != want {
			t.Errorf("row %d lanes %q, want %q", i, lanes, want)
		}
	}
	if !strings.Contains(sc[4], markGreen+"▌") || !strings.Contains(sc[10], markRed+"▐") {
		t.Errorf("lane colors %q %q", sc[4], sc[10])
	}
	if !strings.HasPrefix(sidebar.Plain(sc[0]), "line 1"+strings.Repeat(" ", 21)+"  ") {
		t.Errorf("text not 27 columns: %q", sidebar.Plain(sc[0]))
	}
	for _, c := range []struct {
		key  string
		top  int
		note string
	}{
		{"]", 37, "change 1 of 3"},
		{"]", 77, "change 2 of 3"},
		{"]", 87, "change 3 of 3"},
		{"]", 37, "change 1 of 3"},
		{"[", 87, "change 3 of 3"},
		{"g", 0, ""},
		{"[", 87, "change 3 of 3"},
	} {
		p.Key(c.key)
		if p.Top != c.top || !strings.HasPrefix(p.note, c.note) {
			t.Errorf("%s: top %d (%q), want %d (%q)", c.key, p.Top, p.note, c.top, c.note)
		}
	}
	// From where it was scrolled to, not from the change gone to last.
	p.Top = 50
	if p.Key("]"); p.Top != 77 {
		t.Errorf("] from 50: top %d", p.Top)
	}
	p.Top = 50
	if p.Key("["); p.Top != 37 {
		t.Errorf("[ from 50: top %d", p.Top)
	}
	// A click on a mark goes to its change; a row without one as the bar.
	for _, c := range []struct{ x, y, top int }{{29, 9, 77}, {28, 5, 37}, {28, 1, 0}, {29, 7, 53}, {28, 11, 87}} {
		p.Mouse(sidebar.Event{Kind: sidebar.EvClick, X: c.x, Y: c.y})
		if p.Top != c.top {
			t.Errorf("click %d,%d: top %d, want %d", c.x, c.y, p.Top, c.top)
		}
	}
	if !strings.HasPrefix(p.note, "change 3 of 3") {
		t.Errorf("note after a click on a mark: %q", p.note)
	}
	// No marks, or text that fits: no lanes.
	plainPager := &Pager{Lines: lines(100)}
	if pl := sidebar.Plain(plainPager.Render(30, 12)[4]); !strings.HasPrefix(pl, "line 5"+strings.Repeat(" ", 23)) {
		t.Errorf("lanes without marks: %q", pl)
	}
	short := &Pager{Lines: lines(3), Marks: []Mark{0, MarkAdded, 0}}
	if pl := sidebar.Plain(short.Render(20, 10)[1]); strings.ContainsAny(pl, "▐▌") {
		t.Errorf("lanes on a text that fits: %q", pl)
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

// TestPagerLive (T66.6): a growing text is asked for again while more is
// to come; at its end the pager follows it, scrolled up it stays; a
// resize asks again; Keys and Hint.
func TestPagerLive(t *testing.T) {
	n, more := 5, true
	asked := 0
	p := &Pager{Live: func(w int) ([]string, []string, bool) {
		asked++
		lines := make([]string, n)
		for i := range lines {
			lines[i] = "line " + strconv.Itoa(i)
		}
		return []string{"head " + strconv.Itoa(w)}, lines, more
	}}
	now := time.Unix(100, 0)
	if !p.due(80, now) {
		t.Fatal("not asked at the start")
	}
	p.refresh(80, now)
	p.Render(80, 10) // 8 rows of text
	if p.due(80, now.Add(100*time.Millisecond)) || !p.due(80, now.Add(DefaultLiveEvery)) || !p.due(60, now) {
		t.Error("when to ask again")
	}
	n = 20
	p.refresh(80, now)
	if got := p.Render(80, 10); p.Top != 12 || !strings.Contains(got[len(got)-2], "line 19") || !strings.HasPrefix(got[0], "head 80") {
		t.Errorf("not following: top %d %q", p.Top, got)
	}
	p.Key("up")
	p.Key("up")
	n = 25
	p.refresh(80, now)
	if p.Render(80, 10); p.Top != 10 {
		t.Errorf("scrolled up, moved to %d", p.Top)
	}
	more = false
	p.refresh(80, now)
	if p.due(80, now.Add(time.Hour)) || asked != 4 {
		t.Errorf("asked again after the end (%d)", asked)
	}
	stopped := false
	p.Keys = map[string]func() string{"s": func() string { stopped = true; return "stopping" }}
	p.Hint = "s stop"
	if p.Key("s") || !stopped || !strings.Contains(p.Render(80, 10)[9], "stopping") {
		t.Error("an extra key")
	}
	p.Key("j")
	if !strings.Contains(p.Render(80, 10)[9], "s stop") {
		t.Error("the hint")
	}
}
