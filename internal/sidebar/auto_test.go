package sidebar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
)

// autoModel is the Scheduler view of a project with every kind of row.
func autoModel() Model {
	m := model()
	m.View = ViewAuto
	now := m.Now
	sched := func(when, cli, model string) *auto.Schedule {
		return &auto.Schedule{When: when, CLI: cli, Model: model, Access: auto.AccessFull, Created: now.Add(-time.Hour)}
	}
	paused := sched("", "copilot", "")
	paused.Paused, paused.Access = true, auto.AccessRead
	m.Auto = []AutoItem{
		{
			Def: auto.Def{ID: "aaaaaaaaaaaa", Kind: auto.KindAgent, Name: "sim-tag", Owner: session.Claude}, Sched: sched("5m", "claude", "haiku"),
			Last: &auto.Run{Start: now.Add(-5 * time.Minute), Status: auto.StatusOK, Summary: "v2.3 is out: pipeline started"}, Next: now.Add(5 * time.Minute),
		},
		{
			Def: auto.Def{ID: "bbbbbbbbbbbb", Kind: auto.KindAgent, Name: "docs-check", Owner: session.Copilot}, Sched: sched("09:00", "copilot", ""),
			Running: true, Since: now.Add(-2 * time.Minute),
		},
		{
			Def: auto.Def{ID: "cccccccccccc", Kind: auto.KindSkill, Name: "nightly"}, Sched: sched("hourly", "antigravity", ""),
			Last: &auto.Run{Start: now.Add(-time.Hour), Status: auto.StatusFailed, Exit: 2, Summary: "go test failed"}, Next: now.Add(time.Hour),
		},
		{
			Def: auto.Def{ID: "dddddddddddd", Kind: auto.KindAgent, Name: "by-hand"}, Sched: paused,
			Last: &auto.Run{Start: now.Add(-time.Minute), Status: auto.StatusQuiet},
		},
		{Def: auto.Def{ID: "eeeeeeeeeeee", Kind: auto.KindAgent, Name: "old"}, Sched: sched("5m", "claude", ""), Gone: true},
		{Def: auto.Def{ID: "ffffffffffff", Kind: auto.KindAgent, Name: "release-helper", Owner: session.Claude, Personal: true, Description: "Prepares a release"}},
		{Def: auto.Def{ID: "000000000000", Kind: auto.KindAgent, Name: "any-cli", Description: "Runs with any CLI"}},
		{Def: auto.Def{ID: "111111111111", Kind: auto.KindSkill, Name: "release-notes", Description: "Writes release notes"}},
	}
	m.AutoRoot = "/home/test/acme-api"
	return m
}

// TestAutoView (T66.3): SCHEDULED with each one's time, CLI, model and last
// run (or its spinner) and the next run, then AGENTS and SKILLS with 🚀 and
// ⏰; every row the panel's width; the buttons' actions.
func TestAutoView(t *testing.T) {
	i18n.SetLocale("en")
	m := autoModel()
	for _, theme := range []string{IconsEmoji, IconsNerd} {
		m.Icons = theme
		for _, w := range []int{41, 60} {
			sc := Render(m, w, 40, NoItem)
			for _, l := range sc.Lines {
				if n := DisplayWidth(Plain(l.Text)); n != w {
					t.Errorf("%s %d: %d columns: %q", theme, w, n, Plain(l.Text))
				}
			}
			if testing.Verbose() {
				t.Logf("%s %d:\n%s", theme, w, screenText(sc))
			}
		}
	}
	m.Icons = IconsEmoji
	sc := Render(m, 60, 40, NoItem)
	text := screenText(sc)
	clock := func(d time.Duration) string { return textfmt.Clock(m.Now, m.Now.Add(d)) }
	for _, want := range []string{
		"SCHEDULER · acme-api", "▾ SCHEDULED  5", "✻  sim-tag", "every 5 min · Claude · haiku", "✓ " + clock(-5*time.Minute) + " v2.3 is out: pipeline started",
		"→ " + clock(5*time.Minute), "docs-check", "daily 09:00 · Copilot", "running · 2m", "✗ " + clock(-time.Hour) + " go test failed",
		"by hand · Copilot · read only · paused", "· " + clock(-time.Minute) + " nothing new", "its definition is gone",
		"▾ AGENTS  2", "release-helper  personal", "Prepares a release", "🤖 any-cli", "▾ SKILLS  1", "🧩 release-notes",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("view lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "run now") {
		t.Error("the hint with schedules there")
	}
	want := map[string]bool{}
	for _, l := range sc.Lines {
		for x := railW; x < 60; x++ {
			a := l.ActionAt(x)
			switch a.Kind {
			case ActAuto:
				want[a.Section+" "+a.Path] = true
			case ActAutoMenu:
				want["menu "+a.Path] = true
			case ActAutoNew:
				want["new"] = true
			}
		}
	}
	for _, k := range []string{
		"run aaaaaaaaaaaa", "log aaaaaaaaaaaa", "menu aaaaaaaaaaaa", "stop bbbbbbbbbbbb", "log bbbbbbbbbbbb",
		"menu eeeeeeeeeeee", "run ffffffffffff", "menu when ffffffffffff", "open ffffffffffff", "run 111111111111", "new",
	} {
		if !want[k] {
			t.Errorf("no %q action; have %v", k, want)
		}
	}
	if want["run eeeeeeeeeeee"] || want["run bbbbbbbbbbbb"] {
		t.Error("🚀 on a running or gone item")
	}
	if !m.autoRunning() || !m.Animated() {
		t.Error("a run going on does not turn the spinner")
	}

	// Folded sections; nothing scheduled: the hint; nothing at all.
	m.Fold = map[string]bool{SecAutoScheduled: true}
	if text := screenText(Render(m, 60, 40, NoItem)); strings.Contains(text, "sim-tag") || !strings.Contains(text, "▸ SCHEDULED  5") {
		t.Errorf("folded:\n%s", text)
	}
	m.Fold = nil
	m.Auto = m.Auto[5:]
	if text := screenText(Render(m, 43, 40, NoItem)); !strings.Contains(text, "⏰ schedule · 🚀 run now") || m.autoRunning() {
		t.Errorf("no schedules:\n%s", text)
	}
	m.Auto = nil
	text = screenText(Render(m, 43, 30, NoItem))
	for _, want := range []string{"No agents or skills yet", ".claude/agents/<name>.md", "Click ✨ to write one"} {
		if !strings.Contains(text, want) {
			t.Errorf("empty view lacks %q:\n%s", want, text)
		}
	}
}

// TestAutoItems (T66.3): the project's definitions with their schedules,
// the last run, the next one; a schedule whose definition went; another
// project's schedules left out; the runs going on counted.
func TestAutoItems(t *testing.T) {
	h := testutil.Isolate(t)
	root := h.Project
	testutil.WriteFile(t, filepath.Join(root, ".claude/agents/sim-tag.md"), []byte("---\nname: sim-tag\ndescription: Watches tags\n---\nGo.\n"))
	testutil.WriteFile(t, filepath.Join(root, ".agents/skills/notes/SKILL.md"), []byte("---\nname: notes\ndescription: Notes\n---\n"))
	now := time.Now()
	defs := auto.List(root, h.Home)
	if len(defs) != 2 {
		t.Fatalf("defs %+v", defs)
	}
	sim := defs[0]
	if _, err := auto.Change(sim.ID, &sim, root, now.Add(-time.Hour), func(s *auto.Schedule) { s.When, s.CLI = "5m", "claude" }); err != nil {
		t.Fatal(err)
	}
	gone := auto.Def{ID: "abcdefabcdef", Kind: auto.KindAgent, Name: "old", File: filepath.Join(root, ".claude/agents/old.md")}
	other := auto.Def{ID: "123456123456", Kind: auto.KindAgent, Name: "elsewhere", File: "/elsewhere/a.md"}
	for _, d := range []struct {
		d    auto.Def
		root string
	}{{gone, root}, {other, "/elsewhere"}} {
		if _, err := auto.Change(d.d.ID, &d.d, d.root, now, func(s *auto.Schedule) { s.When = "hourly" }); err != nil {
			t.Fatal(err)
		}
	}
	ex, err := auto.Begin(sim.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := ex.Finish(auto.Run{Start: now.Add(-2 * time.Minute), End: now.Add(-time.Minute), Status: auto.StatusOK, Summary: "fine"}); err != nil {
		t.Fatal(err)
	}
	running, err := auto.Begin(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Finish(auto.Run{Start: now})
	ss, err := auto.Load()
	if err != nil {
		t.Fatal(err)
	}
	items := AutoItems(root, h.Home, ss, now)
	var got []string
	for _, it := range items {
		got = append(got, it.Def.Name)
	}
	if strings.Join(got, ",") != "sim-tag,notes,old" {
		t.Fatalf("items %q", got)
	}
	s := items[0]
	if s.Sched == nil || s.Last == nil || s.Last.Summary != "fine" || !s.Next.Equal(now.Add(-2*time.Minute).Add(5*time.Minute).In(now.Location())) || s.Running {
		t.Errorf("sim-tag %+v next %v", s, s.Next)
	}
	if items[1].Sched != nil || !items[2].Gone || !items[2].Next.IsZero() {
		t.Errorf("notes %+v, old %+v", items[1], items[2])
	}
	if n := AutoRunning(ss); n != 1 {
		t.Errorf("%d runs going on", n)
	}
	m := Model{View: ViewAuto, Now: now}
	gatherAuto(&m, filepath.Join(root, "sub"), func(string) string { return root })
	if m.AutoRoot != root || len(m.Auto) != 3 || m.AutoRunning != 1 {
		t.Errorf("gathered %q %d %d", m.AutoRoot, len(m.Auto), m.AutoRunning)
	}
	m = Model{View: ViewAgents}
	gatherAuto(&m, root, func(string) string { return root })
	if m.Auto != nil || m.AutoRunning != 1 {
		t.Error("another view reads the definitions")
	}
	_ = os.Remove(auto.Path())
}

// TestRailSchedule (T66.3): the Schedule icon after Skills, its badge the
// runs going on; on a short pane the bottom icons never cover the top ones.
func TestRailSchedule(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.AutoRunning = 2
	sc := Render(m, 41, 30, NoItem)
	schedRow := -1
	for i, l := range sc.Lines {
		if a := l.ActionAt(1); a.Kind == ActView && a.Section == ViewAuto && schedRow < 0 {
			schedRow = i
		}
	}
	if schedRow != 15 {
		t.Fatalf("Schedule icon at row %d", schedRow)
	}
	if p := Plain(sc.Lines[schedRow].Text); !strings.Contains(p, "⏰") || !strings.Contains(p, "2") {
		t.Errorf("icon row %q", p)
	}
	if p := Plain(sc.Lines[schedRow+1].Text); !strings.Contains(p, "Schedule") {
		t.Errorf("label row %q", p)
	}
	for _, h := range []int{12, 15, 16, 20, 25, 26} {
		seen := map[ActionKind]int{}
		views := map[string]bool{}
		for _, l := range Render(m, 41, h, NoItem).Lines {
			if a := l.ActionAt(1); a.Kind != ActNone {
				seen[a.Kind]++
				if a.Kind == ActView {
					views[a.Section] = true
				}
			}
		}
		if len(views) != 5 || !views[ViewAuto] {
			t.Errorf("h=%d: views %v", h, views)
		}
	}
}
