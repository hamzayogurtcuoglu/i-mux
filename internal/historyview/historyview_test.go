package historyview

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/histsearch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func model() *Model {
	now := testutil.FrozenTime
	ref := func(a session.AgentID, id string, ago time.Duration) session.Ref {
		return session.Ref{Agent: a, ID: id, UpdatedAt: now.Add(-ago)}
	}
	return &Model{Title: "History · ~/acme", Now: now, Convs: []Conv{
		{Ref: ref("claude", "c1", time.Hour), Title: "fix the login bug", Pinned: true},
		{Ref: ref("codex", "x1", 2*time.Hour), Title: "write the README"},
		{Ref: ref("copilot", "p1", 24*time.Hour), Title: "release notes"},
	}}
}

func plain(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(sidebar.Plain(l) + "\n")
	}
	return b.String()
}

func TestList(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	s := plain(m.Render(80, 12))
	for _, want := range []string{"History · ~/acme", "3 conversations", "Claude  📌 fix the login bug", "1 h ago", "Codex  write the README", "Copilot  release notes"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	m.Key("down")
	if key, done, _ := m.Key("enter"); !done || key != "codex:x1" {
		t.Fatalf("enter: %q %v", key, done)
	}
}

func TestSearchResults(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	for _, k := range []string{"l", "o", "g", "i", "n"} {
		if _, _, changed := m.Key(k); !changed {
			t.Fatal("typing did not change the query")
		}
	}
	// Claude's title has it; Copilot's text has it twice.
	m.Hits = map[string]histsearch.Hit{"copilot:p1": {Count: 2, Snips: []histsearch.Snip{
		{User: true, Text: "the login page", Col: 4, Len: 5},
		{Text: "…so login works", Col: 4, Len: 5},
	}}}
	rows := m.Rows()
	if len(rows) != 4 || rows[0].Conv != 0 || rows[1].Conv != 2 || rows[2].Snip == nil {
		t.Fatalf("rows %+v", rows)
	}
	s := plain(m.Render(80, 12))
	for _, want := range []string{"2 found", "▸ ✻ Claude", "▾ ◉ Copilot  release notes", " 2 ", "you  the login page", "Copilot  …so login works"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if !strings.Contains(strings.Join(m.Render(80, 12), "\n"), hit+"login"+reset) {
		t.Error("the match is not highlighted")
	}
	// ← folds Copilot's messages; Tab folds all, again opens all.
	m.Sel = 2
	m.Key("left")
	if len(m.Rows()) != 2 || m.Sel != 1 {
		t.Fatalf("fold: %d rows, sel %d", len(m.Rows()), m.Sel)
	}
	m.Key("tab")
	m.Key("tab")
	if len(m.Rows()) != 4 {
		t.Fatalf("unfold all: %d rows", len(m.Rows()))
	}
	// A click on a message opens its conversation; on the arrow, folds it.
	m.Render(80, 12)
	if key := m.Click(20, 6); key != "copilot:p1" {
		t.Errorf("click on a message: %q", key)
	}
	if key := m.Click(2, 5); key != "" || !m.Folded["copilot:p1"] {
		t.Errorf("click on the arrow: %q %v", key, m.Folded)
	}
}

// A long list scrolls, with a bar; every row keeps the width.
func TestLongList(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	for i := 0; i < 40; i++ {
		m.Convs = append(m.Convs, Conv{Ref: session.Ref{Agent: "claude", ID: strings.Repeat("x", i+1)}, Title: "a conversation"})
	}
	lines := m.Render(70, 12)
	for _, l := range lines[3:11] {
		p := sidebar.Plain(l)
		if sidebar.DisplayWidth(p) != 70 || !strings.HasSuffix(p, "┃") && !strings.HasSuffix(p, "│") {
			t.Fatalf("row %q (%d columns)", p, sidebar.DisplayWidth(p))
		}
	}
}
