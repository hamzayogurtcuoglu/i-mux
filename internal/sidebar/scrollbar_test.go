package sidebar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

func thumbAt(bar []string) (from, n int) {
	from = -1
	for i, c := range bar {
		if c == barThumb {
			if from < 0 {
				from = i
			}
			n++
		}
	}
	return from, n
}

func TestScrollBar(t *testing.T) {
	if ScrollBar(10, 0, 10) != nil || ScrollBar(3, 0, 10) != nil {
		t.Fatal("a bar for a list that fits")
	}
	// 100 rows, 10 shown: a 1-row thumb, at the top, the middle, the end.
	for _, c := range []struct{ top, at int }{{0, 0}, {44, 4}, {90, 9}, {500, 9}, {-3, 0}} {
		bar := ScrollBar(100, c.top, 10)
		if len(bar) != 10 {
			t.Fatalf("%d cells", len(bar))
		}
		if at, n := thumbAt(bar); at != c.at || n != 1 {
			t.Errorf("top %d: thumb at %d (%d rows), want %d", c.top, at, n, c.at)
		}
	}
	// Half shown: a thumb half the bar.
	if _, n := thumbAt(ScrollBar(20, 0, 10)); n != 5 {
		t.Errorf("thumb of %d rows, want 5", n)
	}
	// A click on the bar's first row is the top, on its last the end.
	if ScrollTo(100, 10, 0) != 0 || ScrollTo(100, 10, 9) != 90 || ScrollTo(100, 10, 5) != 50 || ScrollTo(5, 10, 3) != 0 {
		t.Errorf("ScrollTo %d %d %d", ScrollTo(100, 10, 0), ScrollTo(100, 10, 9), ScrollTo(100, 10, 5))
	}
}

func TestCut(t *testing.T) {
	line := "ab\x1b[31mcdé\x1b[0m界xy"
	for _, c := range []struct {
		from, w int
		want    string
	}{
		{0, 4, "abcd"},
		{2, 3, "cdé"},
		{5, 4, "界xy"},
		{6, 2, " x"}, // the right half of 界
		{0, 20, "abcdé界xy" + strings.Repeat(" ", 11)},
	} {
		got := Cut(line, c.from, c.w)
		if p := Plain(got); p != c.want || DisplayWidth(p) != c.w {
			t.Errorf("Cut(%d, %d) = %q, want %q", c.from, c.w, p, c.want)
		}
	}
	if got := Cut(line, 2, 3); !strings.Contains(got, "\x1b[31m") {
		t.Errorf("the color went: %q", got)
	}
	got := WithBar([]string{"aaaa", "bbbb"}, ScrollBar(4, 0, 2), 4)
	if Plain(got[0]) != "aaa┃" || Plain(got[1]) != "bbb│" {
		t.Errorf("WithBar %q", got)
	}
}

// TestExplorerScrollBar: a tree longer than the panel has a scroll bar in the
// last column; a click on it scrolls there; the keyboard skips it.
func TestExplorerScrollBar(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.View = ViewExplorer
	m.Tree = nil
	for i := 0; i < 60; i++ {
		m.Tree = append(m.Tree, TreeRow{Path: fmt.Sprintf("f%02d.go", i), Name: fmt.Sprintf("f%02d.go", i)})
	}
	w, h := 41, 20
	sc := Render(m, w, h, NoItem)
	var bars, rows int
	for _, l := range sc.Lines {
		p := Plain(l.Text)
		if DisplayWidth(p) != w {
			t.Fatalf("a line of %d columns: %q", DisplayWidth(p), p)
		}
		if strings.HasSuffix(p, "┃") || strings.HasSuffix(p, "│") && strings.Contains(p, ".go") {
			bars++
		}
		if strings.Contains(p, ".go") {
			rows++
		}
	}
	if bars != rows || rows != h-1 {
		t.Fatalf("%d bar cells on %d tree rows (%d shown)", bars, rows, h-1)
	}
	last := sc.Lines[h-1]
	if a := last.ActionAt(w - 1); a.Kind != ActScroll || a.Section != "41" {
		t.Errorf("the bar's last row: %+v", a)
	}
	for _, it := range sc.Items() {
		if sc.ActionOf(it).Kind == ActScroll {
			t.Fatal("the keyboard selects the scroll bar")
		}
	}
	// Scrolled to the end, the last file shows and the thumb is at the bottom.
	m.Scroll = 41
	sc = Render(m, w, h, NoItem)
	if p := Plain(sc.Lines[h-1].Text); !strings.Contains(p, "f59.go") || !strings.HasSuffix(p, "┃") {
		t.Errorf("the end: %q", p)
	}
}
