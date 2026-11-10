package searchview

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/search"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

var esc = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func model() *Model {
	return &Model{Title: "Search in acme", Query: "foo", Sel: 1, Results: []search.Match{
		{Path: "a.go", Line: 2, Col: 5, Len: 3, Text: "func foo() {}"},
		{Path: "a.go", Line: 9, Col: 7, Len: 3, Text: "\t\treturn foo"},
		{Path: "sub/b.txt", Line: 4, Col: 0, Len: 3, Text: "FOO upper"},
	}}
}

func TestKeysAndRows(t *testing.T) {
	m := model()
	rows := m.Rows()
	if len(rows) != 5 || rows[0].Match != nil || rows[0].Count != 2 || rows[3].File != "sub/b.txt" || rows[3].Count != 1 {
		t.Fatalf("rows %+v", rows)
	}
	// The selection starts on the first match; Enter on a match opens it.
	m.Key("down")
	m.Key("down")
	m.Key("down")
	if got, done, _ := m.Key("enter"); !done || got == nil || got.Path != "sub/b.txt" || got.Line != 4 {
		t.Fatalf("enter %+v %v", got, done)
	}
	// ← folds the file of the selection, → unfolds it; Enter on a file's
	// row toggles it.
	m.Sel = 2
	m.Key("left")
	if r := m.Rows(); len(r) != 3 || m.Sel != 0 || !m.Folded["a.go"] {
		t.Fatalf("fold %d rows, sel %d", len(r), m.Sel)
	}
	m.Key("right")
	if len(m.Rows()) != 5 {
		t.Fatal("unfold")
	}
	if _, done, _ := m.Key("enter"); done || len(m.Rows()) != 3 {
		t.Fatal("enter on a file folds it")
	}
	// Tab folds every file, then unfolds them all.
	m.Key("tab")
	if len(m.Rows()) != 2 {
		t.Fatalf("fold all %d", len(m.Rows()))
	}
	m.Key("tab")
	if len(m.Rows()) != 5 {
		t.Fatalf("unfold all %d", len(m.Rows()))
	}
	m.Sel = 1
	if _, _, changed := m.Key("x"); !changed || m.Query != "foox" {
		t.Fatal("typing changes the query")
	}
	if _, _, changed := m.Key("backspace"); !changed || m.Query != "foo" {
		t.Fatal("backspace")
	}
	if got, done, _ := m.Key("esc"); !done || got != nil {
		t.Fatal("esc closes")
	}
}

func TestRender(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	for _, w := range []int{40, 100} {
		lines := m.Render(w, 12)
		if len(lines) != 12 {
			t.Fatalf("%d lines", len(lines))
		}
		for _, l := range lines {
			if n := utf8.RuneCountInString(esc.ReplaceAllString(l, "")); n > w {
				t.Errorf("width %d: %d columns: %q", w, n, l)
			}
		}
	}
	text := esc.ReplaceAllString(strings.Join(m.Render(100, 12), "\n"), "")
	t.Logf("\n%s", text)
	for _, want := range []string{"🔍 Search in acme", "⊟ Collapse all", "❯ foo▏", "3 results in 2 files", "▾ 🐹 a.go", " 2 ", "▌        2  func foo() {}", "9  return foo", "📄 b.txt  sub", "4  FOO upper"} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if !strings.Contains(strings.Join(m.Render(100, 12), "\n"), hit+"foo"+reset) {
		t.Error("the match is highlighted")
	}
}

// More than AutoFold files start folded, one row each; a few start open.
func TestFoldMany(t *testing.T) {
	m := model()
	m.FoldMany()
	if len(m.Folded) != 0 || len(m.Rows()) != 5 {
		t.Fatalf("two files folded: %v", m.Folded)
	}
	m.Results = nil
	for i := 0; i < AutoFold+1; i++ {
		m.Results = append(m.Results, search.Match{Path: fmt.Sprintf("f%d.go", i), Line: 1, Len: 1, Text: "x"})
	}
	m.FoldMany()
	if len(m.Rows()) != AutoFold+1 || m.Sel != 0 {
		t.Fatalf("%d rows, selection %d", len(m.Rows()), m.Sel)
	}
	if text := esc.ReplaceAllString(strings.Join(m.Render(80, 20), "\n"), ""); !strings.Contains(text, "⊞ Expand all") || !strings.Contains(text, "▸ ") {
		t.Errorf("folded:\n%s", text)
	}
}

// Clicks: the fold-all button, a file (fold), a match (open), the bar.
func TestClicks(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.Render(80, 12)
	title := esc.ReplaceAllString(m.Render(80, 12)[0], "")
	x := sidebar.DisplayWidth(title[:strings.Index(title, "⊟")]) + 1
	if mt := m.Click(x, 1); mt != nil || !m.allFolded() {
		t.Fatalf("the fold-all button: folded %v", m.Folded)
	}
	m.Click(10, 1) // the title
	if !m.allFolded() {
		t.Fatal("a click on the title unfolded")
	}
	m.ToggleAll()
	m.Render(80, 12)
	m.Click(10, 4) // a.go's row
	if !m.Folded["a.go"] {
		t.Fatal("a click on a file did not fold it")
	}
	m.Click(10, 4)
	if mt := m.Click(20, 5); mt == nil || mt.Line != 2 {
		t.Fatalf("a click on a match: %+v", mt)
	}
	// Many results: a bar; a click on its last row shows the end.
	m.Results = nil
	for i := 0; i < 50; i++ {
		m.Results = append(m.Results, search.Match{Path: "a.go", Line: i + 1, Len: 1, Text: "x"})
	}
	m.Folded = nil
	lines := m.Render(80, 12)
	if p := esc.ReplaceAllString(lines[3], ""); !strings.HasSuffix(p, "┃") {
		t.Fatalf("no bar: %q", p)
	}
	m.Click(80, 11)
	if m.Top != 51-8 {
		t.Errorf("the bar's last row: top %d", m.Top)
	}
}
