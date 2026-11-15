package sidebar

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
)

func remoteModel() Model {
	return Model{View: ViewRemote, Icons: IconsEmoji, Now: time.Unix(1_800_000_000, 0), Machines: []MachineView{{Name: "box", State: machine.State{Status: machine.StatusOK, Projects: []machine.Project{
		{Session: "web", Name: "web", Branch: "main", Panes: []machine.Pane{{ID: "%3", Agent: "claude", State: "blocked"}, {ID: "%4", Agent: "codex", State: "working"}}},
		{Session: "api", Name: "api"},
	}}}}}
}

// TestRemoteView (T79.1, T79.2): the rail has Remote after Git; the view is
// a tree: the machine with its state, its projects, their agents; every
// row as wide as the sidebar; clicks fold the machine and open a project
// or an agent; a machine not connected is dimmed; machines are not in the
// Agents view.
func TestRemoteView(t *testing.T) {
	i18n.SetLocale("en")
	top, _ := railItems(IconsEmoji)
	var views []string
	for _, it := range top {
		views = append(views, it.label)
	}
	if strings.Join(views, " ") != "rail.agents rail.files rail.search rail.git rail.remote rail.schedule" {
		t.Fatalf("rail %v", views)
	}
	m := remoteModel()
	for _, w := range []int{32, 40, 60} {
		sc := Render(m, w, 0, NoItem)
		for i, l := range sc.Lines {
			if d := DisplayWidth(Plain(l.Text)); d != w {
				t.Errorf("w=%d row %d %q is %d wide", w, i, Plain(l.Text), d)
			}
		}
	}
	text := screenText(Render(m, 50, 0, NoItem))
	for _, want := range []string{"REMOTE", "▾ 💻 box", "● connected", "📁 web", "main", "📁 api", "Claude", "Codex"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
	rows, _ := renderRemote(m, 40, 0)
	acts := map[ActionKind]int{}
	for _, l := range rows {
		acts[l.ActionAt(30).Kind]++
	}
	if acts[ActFold] != 1 || acts[ActMachineOpen] != 4 || rows[0].ActionAt(38).Kind != ActMachineAdd {
		t.Errorf("actions %v", acts)
	}
	if a := rows[1].Action; a.Kind != ActFold || a.Section != "machine:box" {
		t.Errorf("machine row %+v", a)
	}
	if a := rows[4].Action; a.Kind != ActMachineOpen || a.Session != "web" || a.Pane != "%4" {
		t.Errorf("agent row %+v", a)
	}

	// Folded: the machine's row alone.
	m.Fold = map[string]bool{"machine:box": true}
	if text := screenText(Render(m, 50, 0, NoItem)); strings.Contains(text, "web") || !strings.Contains(text, "▸ 💻 box") {
		t.Errorf("folded:\n%s", text)
	}
	m.Fold = nil

	// The rail: one agent works there, one needs you.
	if n, blocked := remoteCounts(m); n != 1 || !blocked {
		t.Errorf("counts %d %v", n, blocked)
	}

	// Not in the Agents view.
	m.View = ViewAgents
	if text := screenText(Render(m, 50, 0, NoItem)); strings.Contains(text, "💻 box") {
		t.Errorf("a machine in the Agents view:\n%s", text)
	}

	// Offline: dimmed as it was last.
	m.View = ViewRemote
	m.Machines[0].State.Status = machine.StatusOffline
	rows, _ = renderRemote(m, 40, 0)
	if !strings.Contains(Plain(rows[1].Text), "✕ offline") || !strings.HasPrefix(rows[2].Text, muted) {
		t.Errorf("offline %q", rows[2].Text)
	}
	if n, blocked := remoteCounts(m); n != 0 || blocked {
		t.Error("an offline machine counted")
	}

	// None: what the view is for, and Add a machine….
	m.Machines = nil
	text = screenText(Render(m, 50, 0, NoItem))
	if !strings.Contains(text, "other computers") || !strings.Contains(text, "Add a machine…") {
		t.Errorf("empty:\n%s", text)
	}
}
