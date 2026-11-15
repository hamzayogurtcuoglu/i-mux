package workspace

import (
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Options that carry a machine's prompts (D73): the answer, and the
// machine, pane and client it is for (a prompt's template turns "%1" into
// the answer, so they cannot be in it).
const (
	OptMachineAnswer = "@imux_machine_answer"
	OptMachineName   = "@imux_machine_name"
	OptMachinePane   = "@imux_machine_pane"
	OptMachineClient = "@imux_machine_client"
)

// MachineStatus is a machine's state as a menu or the sidebar says it.
func MachineStatus(st machine.State) string {
	switch st.Status {
	case machine.StatusOK:
		return "● " + i18n.T("machine.connected")
	case machine.StatusAuth:
		return "✕ " + i18n.T("machine.auth")
	case machine.StatusNoImux:
		return "✕ " + i18n.T("machine.no_imux")
	case machine.StatusOffline:
		return "✕ " + i18n.T("machine.offline")
	}
	return "○ " + i18n.T("machine.connecting")
}

// MachineMenu is one machine's menu: open a project there, reconnect, a
// terminal there, remove.
func (w *WS) MachineMenu(name, pane, client string) *Menu {
	st := machine.ReadState(name)
	tgt := uiTarget(pane, client)
	m := &Menu{Title: "💻 " + name + " · " + MachineStatus(st)}
	if st.Error != "" && st.Status != machine.StatusOK {
		m.Items = append(m.Items, MenuItem{Name: machine.Clean(st.Error), Disabled: true}, Separator())
	}
	m.Items = append(m.Items,
		MenuItem{Name: i18n.T("machine.open_project"), Key: "o", Cmd: w.machinePrompt("project", name, pane, client, i18n.T("machine.project_prompt"))},
		MenuItem{Name: i18n.T("machine.reconnect"), Key: "r", Cmd: w.popupCmd("80%", "60%", append([]string{"machine", "reconnect", name}, tgt...)...)},
		MenuItem{Name: i18n.T("machine.terminal"), Key: "t", Cmd: w.ui(append([]string{"machine", "terminal", name}, tgt...)...)},
		Separator(),
		MenuItem{Name: i18n.T("machine.remove"), Key: "x", Cmd: w.ui(append([]string{"machine", "confirm-remove", name}, tgt...)...)},
	)
	return m
}

// MachineRemoveMenu asks before removing a machine.
func (w *WS) MachineRemoveMenu(name, pane, client string) *Menu {
	return &Menu{Title: i18n.T("machine.remove_title", name), Items: []MenuItem{
		{Name: i18n.T("machine.remove_yes"), Key: "y", Cmd: w.ui(append([]string{"machine", "rm", name}, uiTarget(pane, client)...)...)},
		{Name: i18n.T("click.cancel"), Key: "n"},
	}}
}

// machinePrompt asks a machine's question: a folder to open there.
func (w *WS) machinePrompt(what, name, pane, client, label string) string {
	c := "set-option -g " + OptMachineName + " " + tmux.Quote(name) +
		" ; set-option -g " + OptMachinePane + " " + tmux.Quote(pane) +
		" ; set-option -g " + OptMachineClient + " " + tmux.Quote(client) + " ; command-prompt"
	if client != "" {
		c += " -t " + tmux.Quote(client)
	}
	run := tmux.RunShell(w.Self, "machine", what, "--from-option")
	tmpl := "set-option -g " + OptMachineAnswer + " \"%%%\" ; " + run
	return c + " -p " + tmux.Quote(label) + " " + tmux.Quote(tmpl)
}
