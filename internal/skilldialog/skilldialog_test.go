package skilldialog

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

func agents() []Agent {
	return []Agent{
		{ID: session.Claude, Name: "Claude", Key: "c", Installed: true},
		{ID: session.Antigravity, Name: "Antigravity", Key: "a", Installed: true},
		{ID: session.Copilot, Name: "Copilot", Key: "p", Installed: true},
		{ID: session.Codex, Name: "Codex", Key: "x", Installed: true, Limited: "15:00"},
		{ID: session.OpenCode, Name: "OpenCode", Key: "e"},
	}
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Key(string(r), false)
	}
}

func screen(m *Model, w, h int) string {
	var b strings.Builder
	for _, l := range m.Render(w, h) {
		b.WriteString(sidebar.Plain(l) + "\n")
	}
	return b.String()
}

// TestChooseWhoWrites: the current pane's agent is chosen first, an agent
// that is not installed cannot be chosen, and this pane's agent is offered
// only when there is one.
func TestChooseWhoWrites(t *testing.T) {
	i18n.SetLocale("en")
	m := New(Options{Agents: agents(), Current: session.Copilot})
	if it := m.items[m.sel]; it.kind != NewPane || it.agent.ID != session.Copilot {
		t.Fatalf("first choice: %+v", it)
	}
	s := screen(m, 70, 20)
	for _, want := range []string{"✨ New skill", "Who should write it?", "An agent opens in a new pane", "▸  p  ◉ Copilot", "x  ◎ Codex", "limit 15:00", "e  ▣ OpenCode", "(not installed)", "h  ◉ Copilot in this pane", "knows this conversation", "t  ✎ Blank SKILL.md", "write it yourself", "Esc close"} {
		if !strings.Contains(s, want) {
			t.Errorf("first step lacks %q:\n%s", want, s)
		}
	}
	// Down from Codex skips OpenCode (not installed); its letter does nothing.
	m.Key("down", false)
	m.Key("down", false)
	if it := m.items[m.sel]; it.kind != Here {
		t.Fatalf("down from Codex: %+v", it)
	}
	m.Key("e", false)
	if m.step != 0 {
		t.Fatal("OpenCode (not installed) was chosen")
	}
	// A letter chooses at once.
	m.Key("c", false)
	if m.step != 1 || m.chosen.agent.ID != session.Claude || m.chosen.kind != NewPane {
		t.Fatalf("c: step %d %+v", m.step, m.chosen)
	}

	m = New(Options{Agents: agents()})
	for _, it := range m.items {
		if it.kind == Here {
			t.Fatal("this pane offered without an agent in it")
		}
	}
	if it := m.items[m.sel]; it.agent.ID != session.Claude {
		t.Fatalf("no current agent: the first installed one is chosen, got %+v", it)
	}
	if res, done := m.Key("esc", false); !done || res.Kind != Cancel {
		t.Fatalf("esc: %+v %v", res, done)
	}
}

// TestDescribe: the description is edited like a line editor; Enter sends
// it; Esc goes back and keeps it; an Enter that came with more text (a
// paste without brackets) is a newline.
func TestDescribe(t *testing.T) {
	i18n.SetLocale("en")
	m := New(Options{Agents: agents(), Current: session.Claude})
	m.Key("enter", false)
	s := screen(m, 70, 20)
	for _, want := range []string{"✨ New skill · ✻ Claude · new pane", "Claude gets i-mux's skill-writing guide first:", ".agents/skills/<name>/SKILL.md", "What should the skill do?", "e.g. commit in English", "Leave it empty and Claude asks you first.", "Enter write it · Esc back"} {
		if !strings.Contains(s, want) {
			t.Errorf("describe step lacks %q:\n%s", want, s)
		}
	}
	typeText(m, "sürüm notu yaz")
	m.Key("ctrl-w", false) // "yaz"
	m.Key("home", false)
	m.Key("delete", false) // "s"
	typeText(m, "S")
	m.Key("end", false)
	m.Key("backspace", false) // the space
	m.Key("left", false)
	typeText(m, "!")
	if got := string(m.desc.r); got != "Sürüm not!u" {
		t.Fatalf("edited: %q", got)
	}
	m.Key("ctrl-u", false)
	typeText(m, "write release notes")
	m.Key("enter", true) // more typed text came with it: a pasted newline
	typeText(m, "from git log")
	m.Paste("\r\nand tag\r")
	if got := string(m.desc.r); got != "write release notes\nfrom git log\nand tag\n" {
		t.Fatalf("pasted: %q", got)
	}
	m.Key("esc", false)
	if m.step != 0 {
		t.Fatal("esc did not go back")
	}
	m.Key("enter", false)
	res, done := m.Key("enter", false)
	want := Result{Kind: NewPane, Agent: session.Claude, Text: "write release notes\nfrom git log\nand tag"}
	if !done || !reflect.DeepEqual(res, want) {
		t.Fatalf("enter: %+v %v", res, done)
	}

	// This pane's agent; an empty description is fine (the agent asks).
	m = New(Options{Agents: agents(), Current: session.Claude})
	m.Key("h", false)
	if s := screen(m, 70, 20); !strings.Contains(s, "✨ New skill · ✻ Claude in this pane") || strings.Contains(s, "in this pane · ") {
		t.Errorf("here title:\n%s", s)
	}
	if res, done := m.Key("enter", false); !done || res != (Result{Kind: Here, Agent: session.Claude}) {
		t.Fatalf("here: %+v %v", res, done)
	}
}

// TestTemplateName: the template's name becomes a slug, shown with its path;
// a taken or empty name is refused; opened at the name, Esc closes.
func TestTemplateName(t *testing.T) {
	i18n.SetLocale("en")
	taken := func(n string) bool { return n == "release-notes" }
	m := New(Options{Agents: agents(), Template: true, Exists: taken})
	if res, done := m.Key("enter", false); done {
		t.Fatalf("empty name accepted: %+v", res)
	}
	if s := screen(m, 70, 20); !strings.Contains(s, "Type a name for the skill") || !strings.Contains(s, "Esc close") {
		t.Errorf("empty name:\n%s", s)
	}
	typeText(m, "Release Notes")
	if s := screen(m, 70, 20); !strings.Contains(s, `A skill named "release-notes" already exists`) {
		t.Errorf("taken name:\n%s", s)
	}
	if _, done := m.Key("enter", false); done {
		t.Fatal("taken name accepted")
	}
	m.Paste("\n2")
	if s := screen(m, 70, 20); !strings.Contains(s, "→ .agents/skills/release-notes-2/SKILL.md") {
		t.Errorf("free name:\n%s", s)
	}
	if res, done := m.Key("enter", false); !done || res != (Result{Kind: Template, Text: "release-notes-2"}) {
		t.Fatalf("enter: %+v %v", res, done)
	}

	m = New(Options{Agents: agents(), Template: true})
	if res, done := m.Key("esc", false); !done || res.Kind != Cancel {
		t.Fatalf("esc at a directly opened name: %+v %v", res, done)
	}
	// Chosen in the first step, Esc goes back to it.
	m = New(Options{Agents: agents()})
	m.Key("t", false)
	if _, done := m.Key("esc", false); done || m.step != 0 {
		t.Fatal("esc from the name did not go back")
	}
}

// TestClick: a click on a row of the first step chooses it.
func TestClick(t *testing.T) {
	i18n.SetLocale("en")
	m := New(Options{Agents: agents()})
	lines := m.Render(70, 20)
	for y, l := range lines {
		if strings.Contains(sidebar.Plain(l), "Blank SKILL.md") {
			m.Click(3, y+1)
		}
	}
	if m.step != 1 || m.chosen.kind != Template {
		t.Fatalf("click: step %d %+v", m.step, m.chosen)
	}
}

// TestLayout: the box wraps at spaces, keeps newlines, cuts a word longer
// than a line, and ↑↓ keep the cursor's column.
func TestLayout(t *testing.T) {
	text := []rune("aaa bbb ccccccccc\nd")
	got := layout(text, 6)
	want := []span{{0, 4}, {4, 8}, {8, 14}, {14, 17}, {18, 19}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("layout = %v, want %v", got, want)
	}
	f := &field{r: text, cur: 5} // b|bb
	f.vertical(1, 7)             // wraps at 6
	if f.cur != 9 {
		t.Fatalf("down: %d", f.cur)
	}
	f.vertical(-2, 7)
	if f.cur != 1 {
		t.Fatalf("up twice: %d", f.cur)
	}
	f.vertical(-1, 7)
	if f.cur != 0 {
		t.Fatalf("up from the first line: %d", f.cur)
	}

	// A small dialog drops the guide but keeps the box.
	i18n.SetLocale("en")
	m := New(Options{Agents: agents()})
	m.Key("enter", false)
	s := screen(m, 40, 12)
	if strings.Contains(s, "guide") || !strings.Contains(s, "What should the skill do?") || len(m.Render(40, 12)) != 12 {
		t.Errorf("small dialog:\n%s", s)
	}
}
