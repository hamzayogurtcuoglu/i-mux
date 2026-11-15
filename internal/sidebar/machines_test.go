package sidebar

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
)

// TestMachineRows (T78.4): a machine's row with its state, its projects
// indented under it, its agents in AGENTS; every row as wide as the
// sidebar; a machine that is not connected dimmed.
func TestMachineRows(t *testing.T) {
	i18n.SetLocale("en")
	mv := MachineView{Name: "box", State: machine.State{Status: machine.StatusOK, Projects: []machine.Project{
		{Session: "web", Name: "web", Branch: "main", Panes: []machine.Pane{{ID: "%3", Agent: "claude", State: "blocked"}, {ID: "%4", Agent: "codex", State: "working"}}},
		{Session: "api", Name: "api"},
	}}}
	now := time.Unix(1_800_000_000, 0)
	for _, w := range []int{14, 24, 40} {
		rows := machineProjectRows(mv, w, 0)
		if len(rows) != 3 {
			t.Fatalf("%d rows", len(rows))
		}
		for i, l := range append(rows, machineAgentRows(mv, w, now, 0)...) {
			if d := DisplayWidth(Plain(l.Text)); d != w {
				t.Errorf("w=%d row %d %q is %d wide", w, i, Plain(l.Text), d)
			}
		}
		if rows[0].Action.Kind != ActMachineMenu || rows[0].Action.Path != "box" {
			t.Errorf("machine row action %+v", rows[0].Action)
		}
		if a := rows[1].Action; a.Kind != ActMachineOpen || a.Path != "box" || a.Session != "web" {
			t.Errorf("project row action %+v", a)
		}
	}
	if p := Plain(machineRow(mv, 40).Text); !strings.HasPrefix(p, " 💻 box") || !strings.HasSuffix(strings.TrimSpace(p), "● connected") {
		t.Errorf("machine row %q", p)
	}
	agents := machineAgentRows(mv, 40, now, 0)
	if len(agents) != 3 || !strings.Contains(Plain(agents[0].Text), "box · web") || agents[2].Action.Pane != "%4" {
		t.Fatalf("agents %+v", agents)
	}
	// Offline: as it was last, in the muted color.
	mv.State.Status, mv.State.Error = machine.StatusOffline, "refused"
	rows := machineProjectRows(mv, 40, 0)
	if !strings.Contains(Plain(rows[0].Text), "✕ offline") || !strings.HasPrefix(rows[1].Text, muted) {
		t.Errorf("offline %q", rows[1].Text)
	}
}
