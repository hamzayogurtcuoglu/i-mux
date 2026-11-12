package autodialog

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

var now = time.Date(2026, 11, 12, 12, 0, 0, 0, time.Local)

func opts() Options {
	return Options{
		Name: "sim-tag", Description: "Watches the simulator's tags", Project: "scc",
		CLIs: []CLI{
			{ID: session.Claude, Name: "Claude", Key: "c"},
			{ID: session.Antigravity, Name: "Antigravity", Key: "a"},
			{ID: session.Copilot, Name: "Copilot", Key: "p", Limited: "15:00"},
		},
		CLI: session.Claude, AgentModel: "haiku", Owner: session.Claude,
		Used: map[session.AgentID][]string{session.Copilot: {"gpt-6", "auto"}},
		Now:  now,
	}
}

func screen(m *Model, w, h int) string {
	var out []string
	for _, l := range m.Render(w, h) {
		out = append(out, strings.TrimRight(sidebar.Plain(l), " "))
	}
	return strings.Join(out, "\n")
}

func keys(m *Model, ks ...string) (Result, bool) {
	var r Result
	var done bool
	for _, k := range ks {
		r, done = m.Key(k)
	}
	return r, done
}

// TestPanelDefaults (T66.6): everything is chosen already; Enter runs the
// agent now with its own CLI and model and full access.
func TestPanelDefaults(t *testing.T) {
	i18n.SetLocale("en")
	m := New(opts())
	s := screen(m, 80, 16)
	for _, want := range []string{
		"🚀 sim-tag  · agent · scc", "Watches the simulator's tags",
		"▸ Run with   ✻ Claude  ▲ Antigravity  ◉ Copilot", "Model      Default  ✎ other…", "When       Now  every 5 min  every 15 min  hourly  daily 09:00  ✎ other…",
		"Access     Full  Read only", "Runs now with Claude · haiku, full access, in scc.", "🚀 Run now    Cancel",
		"↑↓ row · ←→ choose",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("panel lacks %q:\n%s", want, s)
		}
	}
	if r, done := m.Key("enter"); !done || r != (Result{Run: true, CLI: session.Claude}) {
		t.Errorf("enter: %+v %v", r, done)
	}
	if r, done := New(opts()).Key("esc"); !done || r.Run {
		t.Error("esc runs")
	}
	if Height(opts()) < len(strings.Split(strings.TrimRight(screen(New(opts()), 80, 0), "\n"), "\n"))-3 {
		t.Error("Height")
	}
}

// TestPanelChoices (T66.6): the CLI by arrow or letter (its models come
// in the background), a model chosen or typed, a time chosen or typed (a
// wrong one says so), read only.
func TestPanelChoices(t *testing.T) {
	i18n.SetLocale("en")
	m := New(opts())
	if p := m.Pending(); len(p) != 1 || p[0] != session.Claude || m.Pending() != nil {
		t.Fatalf("pending %v", p)
	}
	if s := screen(m, 80, 16); !strings.Contains(s, "Default  ✎ other…  …") {
		t.Errorf("no loading mark:\n%s", s)
	}
	m.SetModels(session.Claude, []string{"fable", "opus", "sonnet"})
	keys(m, "down", "right", "right")
	if r, _, _ := m.chosen(); r.Model != "opus" {
		t.Errorf("model %q", r.Model)
	}
	// Typed: a space is no part of a model's name.
	keys(m, "m", "y", " ", "-", "x")
	if r, _, _ := m.chosen(); r.Model != "my-x" || !strings.Contains(screen(m, 80, 16), "✎ my-x▏") {
		t.Errorf("typed model %q", r.Model)
	}
	keys(m, "backspace", "backspace", "left") // back to sonnet
	if r, _, _ := m.chosen(); r.Model != "sonnet" {
		t.Errorf("after the typed one %q", r.Model)
	}
	// A time: every 15 min.
	keys(m, "down", "right", "right")
	if s := screen(m, 100, 16); !strings.Contains(s, "Runs every 15 min with Claude · sonnet, full access, in scc. The first run starts now.") || !strings.Contains(s, "⏰ Schedule") {
		t.Errorf("scheduled:\n%s", s)
	}
	// One typed in Turkish units, then a wrong one.
	keys(m, "2", " ", "s", "a", "a", "t")
	if r, _, _ := m.chosen(); r.When != "2h" {
		t.Errorf("typed time %q", r.When)
	}
	keys(m, "ctrl-u", "3", "0", "s", "n")
	if _, done := m.Key("enter"); done || !strings.Contains(screen(m, 80, 16), "⚠ At most once a minute") {
		t.Errorf("too often accepted:\n%s", screen(m, 80, 16))
	}
	keys(m, "ctrl-u", "s", "o", "o", "n")
	if _, done := m.Key("enter"); done || !strings.Contains(screen(m, 80, 16), "Not a time: soon") {
		t.Error("a wrong time accepted")
	}
	keys(m, "ctrl-u", "0", "9", ":", "3", "0")
	keys(m, "down", "right")
	r, done := m.Key("enter")
	if !done || r != (Result{Run: true, CLI: session.Claude, Model: "sonnet", When: "09:30", ReadOnly: true}) {
		t.Errorf("result %+v %v", r, done)
	}

	// The letter of a CLI on its row; Copilot's models: the ones used
	// before; it is limited: a warning instead of the summary.
	m = New(opts())
	keys(m, "p")
	if p := m.Pending(); len(p) != 2-1 || p[0] != session.Copilot {
		t.Errorf("pending %v", p)
	}
	s := screen(m, 80, 16)
	if !strings.Contains(s, "Default  gpt-6  auto  ✎ other…") || !strings.Contains(s, "⚠ Copilot is limited until 15:00") {
		t.Errorf("copilot:\n%s", s)
	}
	// The buttons: Cancel ends it without running.
	keys(m, "up", "right")
	if r, done := m.Key("enter"); !done || r.Run {
		t.Error("Cancel ran it")
	}
}

// TestPanelMouse (T66.6): a click on a chip chooses it, on Run runs, on
// Cancel closes.
func TestPanelMouse(t *testing.T) {
	i18n.SetLocale("en")
	m := New(opts())
	lines := m.Render(80, 16)
	at := func(text string) (int, int) {
		for y, l := range lines {
			p := sidebar.Plain(l)
			if i := strings.Index(p, text); i >= 0 {
				return sidebar.DisplayWidth(p[:i]) + 1, y + 1
			}
		}
		t.Fatalf("no %q", text)
		return 0, 0
	}
	x, y := at("Antigravity")
	if _, done := m.Click(x, y); done || m.cliID() != session.Antigravity {
		t.Errorf("clicked %s", m.cliID())
	}
	lines = m.Render(80, 16)
	x, y = at("hourly")
	m.Click(x+2, y)
	if r, _, _ := m.chosen(); r.When != "hourly" || m.row != rowWhen {
		t.Errorf("when %q row %d", r.When, m.row)
	}
	lines = m.Render(80, 16)
	x, y = at("Schedule")
	if r, done := m.Click(x, y); !done || r != (Result{Run: true, CLI: session.Antigravity, When: "hourly"}) {
		t.Errorf("schedule button %+v %v", r, done)
	}
	m = New(opts())
	lines = m.Render(80, 16)
	x, y = at("Cancel")
	if r, done := m.Click(x, y); !done || r.Run {
		t.Error("Cancel by click")
	}
	if _, done := m.Click(1, 1); done {
		t.Error("a click on nothing")
	}
}

// TestPanelLayout (T66.6): every row fits the popup; a row of many models
// shows a window around the one chosen; opened at its time, the next run.
func TestPanelLayout(t *testing.T) {
	i18n.SetLocale("en")
	o := opts()
	o.CLI = session.Antigravity
	m := New(o)
	m.SetModels(session.Antigravity, []string{"gemini-4-argon-high", "gemini-4-argon-medium", "gemini-4-argon-low", "gemini-4-carbon", "claude-opus-5-5", "claude-sonnet-5-5", "gpt-oss-120b"})
	keys(m, "down", "end", "left", "left")
	for _, w := range []int{46, 60, 80} {
		for _, l := range m.Render(w, 16) {
			if n := sidebar.DisplayWidth(sidebar.Plain(l)); n > w {
				t.Errorf("w %d: %d columns: %q", w, n, sidebar.Plain(l))
			}
		}
	}
	s := screen(m, 60, 16)
	if !strings.Contains(s, "‹ ") || !strings.Contains(s, "claude-sonnet-5-5") {
		t.Errorf("window:\n%s", s)
	}
	keys(m, "home")
	if s := screen(m, 60, 16); strings.Contains(s, "‹ ") || !strings.Contains(s, " ›") {
		t.Errorf("window at the start:\n%s", s)
	}

	o = opts()
	o.When, o.AtWhen = "2h", true
	o.Created, o.LastStart = now.Add(-3*time.Hour), now.Add(-30*time.Minute)
	m = New(o)
	s = screen(m, 80, 16)
	if !strings.Contains(s, "▸ When       Now  every 5 min  every 15 min  hourly  daily 09:00  every 2 h  ›") || !strings.Contains(s, "Next run: 13:30.") {
		t.Errorf("at its time:\n%s", s)
	}
	if r, done := m.Key("enter"); !done || r.When != "2h" {
		t.Errorf("kept time %+v", r)
	}
	if _, done := New(Options{Name: "x", Now: now}).Key("enter"); done {
		t.Error("ran without a CLI")
	}
}
