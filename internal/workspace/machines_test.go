package workspace

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
)

// TestMachineMenu (T79.2): a machine's menu (a right-click on its row in
// the Remote view) shows why it is not connected, cleaned, and its actions.
func TestMachineMenu(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	if err := machine.Add(machine.Machine{Name: "box", Target: "ali@box"}); err != nil {
		t.Fatal(err)
	}
	_ = machine.WriteState("box", machine.State{Status: machine.StatusAuth, Error: "Permission denied\x1b[2J"})
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
