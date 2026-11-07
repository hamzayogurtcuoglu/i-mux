package picker

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

var esc = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func model(t *testing.T) (*Model, string) {
	home := t.TempDir()
	for _, d := range []string{"code/acme-api/src", "code/blog", "work/acme-mobile", "Masaüstü/notlar"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p := func(s string) string { return filepath.Join(home, s) }
	m := &Model{Home: home, Cands: folders.Merge(nil, []string{p("code/blog")}, []string{p("code/acme-api"), p("work/acme-mobile"), p("Masaüstü/notlar")})}
	m.Shorten = func(s string) string {
		if s == home {
			return "~"
		}
		return "~" + strings.TrimPrefix(s, home)
	}
	return m, home
}

func typeIn(m *Model, s string) {
	for _, r := range s {
		m.Key(string(r))
	}
}

func TestPickByName(t *testing.T) {
	i18n.SetLocale("en")
	m, home := model(t)
	typeIn(m, "acm")
	if l := m.List(); len(l) != 2 || filepath.Base(l[0].Path) != "acme-api" {
		t.Fatalf("list %+v", l)
	}
	m.Key("down")
	if got, done := m.Key("enter"); !done || got != filepath.Join(home, "work/acme-mobile") {
		t.Fatalf("enter %q %v", got, done)
	}
	// Turkish letters, backspace, Ctrl+U.
	m.Key("ctrl-u")
	typeIn(m, "masaü")
	if l := m.List(); len(l) != 1 || filepath.Base(l[0].Path) != "notlar" {
		t.Fatalf("utf-8 %+v", l)
	}
	m.Key("backspace")
	if m.Query != "masa" {
		t.Fatalf("backspace %q", m.Query)
	}
	if _, done := m.Key("esc"); !done {
		t.Fatal("esc cancels")
	}
}

func TestBrowseAndTab(t *testing.T) {
	i18n.SetLocale("en")
	m, home := model(t)
	typeIn(m, "~/co")
	m.Key("tab") // ~/code/
	if m.Query != "~/code/" {
		t.Fatalf("tab %q", m.Query)
	}
	// The folder itself first, then its subfolders.
	l := m.List()
	if len(l) != 3 || l[0].Path != filepath.Join(home, "code") || l[0].Kind != kindThis || filepath.Base(l[1].Path) != "acme-api" {
		t.Fatalf("browse %+v", l)
	}
	if got, _ := m.Key("enter"); got != filepath.Join(home, "code") {
		t.Fatalf("enter on this folder %q", got)
	}
	// Tab on a name match goes into it.
	m.Key("ctrl-u")
	typeIn(m, "blog")
	m.Key("tab")
	if m.Query != "~/code/blog/" {
		t.Fatalf("tab from a match %q", m.Query)
	}
	// A typed folder that lists nothing still opens.
	m.Key("ctrl-u")
	typeIn(m, "~/code/acme-api/src/")
	m.Sel = 5
	if got, _ := m.Key("enter"); got != filepath.Join(home, "code/acme-api/src") {
		t.Fatalf("typed path %q", got)
	}
}

func TestRender(t *testing.T) {
	i18n.SetLocale("en")
	m, _ := model(t)
	for _, w := range []int{30, 80} {
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
	text := esc.ReplaceAllString(strings.Join(m.Render(80, 12), "\n"), "")
	t.Logf("\n%s", text)
	for _, want := range []string{"Open a project", "> ▏", "▸ ~/code/blog", "recent", "~/code/acme-api", "git", "Esc cancel"} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q", want)
		}
	}
	m.Query = "zzz"
	m.Scanning = true
	if text := esc.ReplaceAllString(strings.Join(m.Render(80, 12), "\n"), ""); !strings.Contains(text, "Searching") {
		t.Errorf("scanning:\n%s", text)
	}
	// The selection scrolls into view.
	m.Query, m.Scanning = "", false
	m.Sel = 3
	if text := esc.ReplaceAllString(strings.Join(m.Render(80, 6), "\n"), ""); !strings.Contains(text, "▸ ~/Masaüstü/notlar") {
		t.Errorf("scroll:\n%s", text)
	}
}
