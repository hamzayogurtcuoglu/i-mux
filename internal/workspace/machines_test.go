package workspace

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
)

// TestMachinesMenu (T78.3): each machine with its state opens its menu;
// Add a machine… asks for a destination, which only an option carries.
func TestMachinesMenu(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	if err := machine.Add(machine.Machine{Name: "box", Target: "ali@box"}); err != nil {
		t.Fatal(err)
	}
	_ = machine.WriteState("box", machine.State{Status: machine.StatusAuth, Error: "Permission denied\x1b[2J"})
	m := w.MachinesMenu("%1", "c1")
	if len(m.Items) != 3 || m.Items[0].Name != "💻 box  ✕ needs sign-in (Reconnect)" || m.Items[0].Key != "1" {
		t.Fatalf("items %+v", m.Items)
	}
	if c := m.Items[0].Cmd; !strings.Contains(c, "machine menu box --pane %1 --client c1") {
		t.Errorf("box: %q", c)
	}
	add := cmdOf(m, "a")
	if !strings.Contains(add, "command-prompt -t c1") || !strings.Contains(add, "@imux_machine_answer \"%%%\"") ||
		!strings.Contains(add, "display-popup -E") || !strings.Contains(add, "machine add --from-option") {
		t.Errorf("add: %q", add)
	}
	// The answer is never in a shell command: only in the option.
	if strings.Contains(add, "machine add %") || strings.Contains(add, "add '%") {
		t.Errorf("the answer reaches a command line: %q", add)
	}

	mm := w.MachineMenu("box", "%1", "c1")
	if !strings.HasPrefix(mm.Title, "💻 box · ✕") || !mm.Items[0].Disabled || strings.Contains(mm.Items[0].Name, "\x1b") {
		t.Fatalf("machine menu %q %+v", mm.Title, mm.Items)
	}
	for key, want := range map[string]string{"o": "machine project --from-option", "r": "machine reconnect box", "t": "machine terminal box", "x": "machine confirm-remove box"} {
		if c := cmdOf(mm, key); !strings.Contains(c, want) {
			t.Errorf("%s: %q", key, c)
		}
	}
	if c := cmdOf(w.MachineRemoveMenu("box", "%1", "c1"), "y"); !strings.Contains(c, "machine rm box") {
		t.Errorf("remove: %q", c)
	}
}
