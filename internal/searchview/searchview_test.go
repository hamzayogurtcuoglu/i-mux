package searchview

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/search"
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
	for _, want := range []string{"Search in acme", "> foo▏", "3 results in 2 files", "▾ a.go", "▸ 2    func foo() {}", "9    return foo", "sub/b.txt", "4    FOO upper"} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if !strings.Contains(strings.Join(m.Render(100, 12), "\n"), hit+"foo"+reset) {
		t.Error("the match is highlighted")
	}
}
