package sidebar

import (
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// machineStatus is a machine's state on its row: its glyph, words and
// color (D73).
func machineStatus(st machine.State) (string, string) {
	switch st.Status {
	case machine.StatusOK:
		return "● " + i18n.T("machine.connected"), green
	case machine.StatusAuth:
		return "✕ " + i18n.T("machine.auth"), red
	case machine.StatusNoImux:
		return "✕ " + i18n.T("machine.no_imux"), red
	case machine.StatusOffline:
		return "✕ " + i18n.T("machine.offline"), red
	}
	return "○ " + i18n.T("machine.connecting"), muted
}

// machineRow is a machine's row: 💻 and its name, its state on the right;
// a click opens its menu.
func machineRow(mv MachineView, w int) Line {
	st, color := machineStatus(mv.State)
	sw := runes(st)
	if sw > w/2 {
		st, sw = string([]rune(st)[:1]), 1 // the glyph alone in a narrow sidebar
	}
	nw := max(w-sw-6, 1)
	left := " 💻 " + bold + fit(mv.Name, nw) + reset
	return Line{Text: spread(left, 4+nw, color+st+reset+" ", sw+1, w), Action: Action{Kind: ActMachineMenu, Path: mv.Name}}
}

// remotePanes are a machine's project's agents as the sidebar's rows
// (the machine sent them: checked and cleaned, machine.Parse).
func remotePanes(p machine.Project) []PaneRow {
	var out []PaneRow
	for _, x := range p.Panes {
		r := PaneRow{ID: x.ID, Agent: session.AgentID(x.Agent), Name: x.Name, State: x.State}
		if r.State == "" {
			r.State = StateIdle
		}
		if x.Since > 0 {
			r.Since = time.Unix(x.Since, 0)
		}
		out = append(out, r)
	}
	return out
}

// dimmed is a row of a machine that is not connected: its text in the
// muted color, as it was last.
func dimmed(l Line, connected bool) Line {
	if connected {
		return l
	}
	l.Text = muted + Plain(l.Text) + reset
	return l
}

// machineProjectRows are a machine's row and its projects' rows (in
// PROJECTS), indented; a click opens one here.
func machineProjectRows(mv MachineView, w, tick int) []Line {
	out := []Line{machineRow(mv, w)}
	ok := mv.State.Status == machine.StatusOK
	for _, p := range mv.State.Projects {
		l := projectRow(ProjectRow{Session: p.Session, Name: p.Name, Branch: p.Branch, Panes: remotePanes(p)}, max(w-2, 8), tick)
		l.Text = "  " + l.Text
		l.Action = Action{Kind: ActMachineOpen, Path: mv.Name, Session: p.Session}
		out = append(out, dimmed(l, ok))
	}
	return out
}

// machineAgentRows are a machine's agents (in AGENTS): each project's name
// with the machine's, then its agents; a click opens one here.
func machineAgentRows(mv MachineView, w int, now time.Time, tick int) []Line {
	var out []Line
	ok := mv.State.Status == machine.StatusOK
	for _, p := range mv.State.Projects {
		panes := remotePanes(p)
		if len(panes) == 0 {
			continue
		}
		head := Line{Text: muted + fit(" 💻 "+mv.Name+" · "+p.Name, w) + reset, Action: Action{Kind: ActMachineOpen, Path: mv.Name, Session: p.Session}}
		out = append(out, dimmed(head, ok))
		for _, r := range panes {
			l := agentRow(r, w, now, tick)
			l.Action = Action{Kind: ActMachineOpen, Path: mv.Name, Session: p.Session, Pane: r.ID}
			out = append(out, dimmed(l, ok))
		}
	}
	return out
}
