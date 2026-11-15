package sidebar

import (
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// foldMachine is a machine's name in @imux_fold (D74).
const foldMachine = "machine:"

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

// renderRemote is the Remote view (D74), a tree as VS Code's Remote
// Explorer: REMOTE and + (add a machine); each machine with its state, its
// projects under it and their agents under them.
func renderRemote(m Model, w, h int) ([]Line, ListView) {
	ic := m.icons()
	add := button(ic.Add, muted, Action{Kind: ActMachineAdd})
	add.Tip = i18n.T("machine.add_tip")
	head, style := " "+i18n.T("sidebar.remote"), title
	if m.Tip != "" {
		head, style = " "+m.Tip, tipStyle
	}
	out := []Line{buttons(w, Cell{Text: style + fit(head, w-add.W) + reset}, add)}
	var rows []Line
	if len(m.Machines) == 0 {
		rows = []Line{{Text: strings.Repeat(" ", w)}}
		for _, l := range wrapWords(i18n.T("machine.empty"), w-3) {
			rows = append(rows, Line{Text: muted + fit("  "+l, w) + reset})
		}
		rows = append(rows, Line{Text: strings.Repeat(" ", w)},
			Line{Text: cyan + fit("  "+ic.Add+" "+i18n.T("machine.add_row"), w) + reset, Action: Action{Kind: ActMachineAdd}})
	}
	for _, mv := range m.Machines {
		rows = append(rows, machineTree(mv, m.Fold[foldMachine+mv.Name], w, m.Now, m.Tick, ic)...)
	}
	n := 0
	if h > 0 {
		n = h - len(out)
	}
	return withList(out, rows, m.Scroll, n, w)
}

// machineTree is a machine's row and, unless it is folded, its projects
// and their agents; a machine that is not connected is dimmed as it was
// last.
func machineTree(mv MachineView, folded bool, w int, now time.Time, tick int, ic Icons) []Line {
	out := []Line{machineRow(mv, folded, w, ic)}
	if folded {
		return out
	}
	ok := mv.State.Status == machine.StatusOK
	if ok && len(mv.State.Projects) == 0 {
		out = append(out, Line{Text: muted + fit("     "+i18n.T("machine.no_projects"), w) + reset, Action: Action{Kind: ActMachineMenu, Path: mv.Name}})
	}
	for _, p := range mv.State.Projects {
		out = append(out, dimmed(remoteProjectRow(mv.Name, p, w), ok))
		for _, r := range remotePanes(p) {
			l := agentRow(r, w-4, now, tick)
			l.Text = "    " + l.Text
			l.Cells = nil
			l.Action = Action{Kind: ActMachineOpen, Path: mv.Name, Session: p.Session, Pane: r.ID}
			out = append(out, dimmed(l, ok))
		}
	}
	return out
}

// machineRow is a machine's row: its arrow, 💻 and name, its state on the
// right. A click folds it; a right-click opens its menu.
func machineRow(mv MachineView, folded bool, w int, ic Icons) Line {
	st, color := machineStatus(mv.State)
	sw := runes(st)
	if sw > w/2 {
		st, sw = string([]rune(st)[:1]), 1 // the glyph alone in a narrow sidebar
	}
	arrow := ic.fold(folded)
	lead := 1 + DisplayWidth(arrow) + 1 + 2 + 1 // " ▾ 💻 "
	nw := max(w-sw-1-lead, 1)
	left := " " + muted + arrow + reset + " 💻 " + bold + fit(mv.Name, nw) + reset
	return Line{Text: spread(left, lead+nw, color+st+reset+" ", sw+1, w), Action: Action{Kind: ActFold, Section: foldMachine + mv.Name}}
}

// remoteProjectRow is one of a machine's projects: 📁 and its name, its
// branch on the right; a click opens it here.
func remoteProjectRow(name string, p machine.Project, w int) Line {
	br, bw := "", 0
	if p.Branch != "" && w >= 24 {
		b := fit(p.Branch, min(runes(p.Branch), w/3))
		br, bw = muted+b+reset+" ", runes(b)+1
	}
	const lead = 3 + 2 + 1 // "   📁 "
	nw := max(w-lead-bw, 1)
	left := "   📁 " + fit(p.Name, min(runes(p.Name), nw))
	return Line{Text: spread(left, lead+min(runes(p.Name), nw), br, bw, w), Action: Action{Kind: ActMachineOpen, Path: name, Session: p.Session}}
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

// remoteCounts are the machines' agents that work, and whether one needs
// the user (the rail's Remote icon).
func remoteCounts(m Model) (working int, blocked bool) {
	for _, mv := range m.Machines {
		if mv.State.Status != machine.StatusOK {
			continue
		}
		for _, p := range mv.State.Projects {
			for _, x := range p.Panes {
				switch x.State {
				case StateWorking:
					working++
				case StateBlocked:
					blocked = true
				}
			}
		}
	}
	return working, blocked
}

// wrapWords breaks text into lines of at most w columns, between words.
func wrapWords(text string, w int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case DisplayWidth(line+" "+word) <= w:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}
