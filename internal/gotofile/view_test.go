package gotofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

func TestModel(t *testing.T) {
	i18n.SetLocale("en")
	dir := t.TempDir()
	now := time.Now()
	for i, f := range []string{"old.go", "src/new.go", "mid.md"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		at := now.Add(time.Duration([]int{-3, 0, -1}[i]) * time.Hour)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	all := []string{"mid.md", "old.go", "src/new.go"}
	m := &Model{Title: "Go to File · p", Files: all, Recent: RecentFirst(dir, all)}
	m.Update()
	if got := paths(m.Results); strings.Join(got, " ") != "src/new.go mid.md old.go" {
		t.Fatalf("empty query: the files changed last first: %v", got)
	}
	// Typing narrows; ↓ and Enter choose; a line after ":".
	for _, k := range []string{"g", "o", ":", "7"} {
		m.Key(k)
	}
	if got := paths(m.Results); len(got) != 2 {
		t.Fatalf("go:7 %v", got)
	}
	m.Key("down")
	if done, ok := m.Key("enter"); !done || !ok {
		t.Fatal("enter did not choose")
	}
	if p, line, _ := m.Chosen(); p != m.Results[1].Path || line != 7 {
		t.Fatalf("chosen %q %d", p, line)
	}
	if args := LineArgs("a.go", 7); strings.Join(args, " ") != "--line 7 a.go" {
		t.Fatal(args)
	}
	m.Key("ctrl-u")
	if m.Query != "" || len(m.Results) != 3 {
		t.Fatal("ctrl-u")
	}
	if done, ok := m.Key("esc"); !done || ok {
		t.Fatal("esc")
	}

	// Every row is exactly w columns, the matched letters bold yellow, a
	// long folder cut from the left.
	m.Files = append(m.Files, "very/long/folder/names/that/go/on/and/on/render.go")
	m.Query = "render"
	m.Update()
	for _, w := range []int{30, 60} {
		lines := m.Render(w, 12)
		for i, l := range lines[3:4] {
			if n := sidebar.DisplayWidth(sidebar.Plain(l)); n != w {
				t.Errorf("w %d row %d: %d columns: %q", w, i, n, sidebar.Plain(l))
			}
		}
		if !strings.Contains(lines[3], hit+"r"+reset) {
			t.Errorf("no highlight: %q", lines[3])
		}
		if w == 30 && !strings.Contains(sidebar.Plain(lines[3]), "…") {
			t.Errorf("long folder not cut: %q", sidebar.Plain(lines[3]))
		}
	}
}
