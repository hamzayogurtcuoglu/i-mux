package sidebar

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// ActionKind is what activating a sidebar row does.
type ActionKind int

// Actions.
const (
	ActNone     ActionKind = iota
	ActFocus               // focus pane Pane (switching project if needed)
	ActSwitch              // continue the target pane in Agent (with context)
	ActSplitH              // split the target pane side by side (agent chooser)
	ActSplitV              // split it one above the other
	ActSessions            // session picker
	ActLimits              // limits popup
	ActKeys                // keys help
	ActProject             // switch the client to project Session
	ActOpen                // ask for a folder and open it as a project
	ActRename              // ask for a name for the target pane
	ActDiff                // show the diff of Path
	ActNewAgent            // split the target pane (direction by its shape) with an agent chooser
)

// Action is a row's action.
type Action struct {
	Kind    ActionKind
	Pane    string
	Agent   session.AgentID
	Session string
	Path    string
	Status  string
}

// ANSI styles. Only foreground colors and attributes, so any terminal theme
// keeps its background.
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	rev    = "\x1b[7m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	cyan   = "\x1b[36m"
)

// Line is one rendered row: its text with escapes, and its action. A row of
// side-by-side buttons (the toolbar) has Cells instead of one Action.
type Line struct {
	Text   string
	Action Action
	Cells  []Cell
}

// Cell is one button of a row: columns [X, X+W) of the line.
type Cell struct {
	X, W   int
	Text   string // exactly W columns, with escapes
	Action Action
}

// ActionAt is the action of column x (0-based) of the line.
func (l Line) ActionAt(x int) Action {
	if len(l.Cells) == 0 {
		return l.Action
	}
	for _, c := range l.Cells {
		if x >= c.X && x < c.X+c.W {
			return c.Action
		}
	}
	return Action{}
}

// Item is one selectable thing: a row (Cell -1) or a button of a row.
type Item struct{ Row, Cell int }

// NoItem selects nothing.
var NoItem = Item{Row: -1, Cell: -1}

// Screen is a rendered sidebar.
type Screen struct {
	Lines []Line
}

// Items returns what the keyboard can select, top to bottom.
func (s Screen) Items() []Item {
	var out []Item
	for i, l := range s.Lines {
		if len(l.Cells) > 0 {
			for j, c := range l.Cells {
				if c.Action.Kind != ActNone {
					out = append(out, Item{i, j})
				}
			}
			continue
		}
		if l.Action.Kind != ActNone {
			out = append(out, Item{i, -1})
		}
	}
	return out
}

// ActionOf is the action of a selected item.
func (s Screen) ActionOf(it Item) Action {
	if it.Row < 0 || it.Row >= len(s.Lines) {
		return Action{}
	}
	l := s.Lines[it.Row]
	if it.Cell >= 0 && it.Cell < len(l.Cells) {
		return l.Cells[it.Cell].Action
	}
	return l.Action
}

// runes counts display columns. The sidebar uses only narrow characters
// (no emoji such as ⛔, which many terminals draw two columns wide).
func runes(s string) int { return utf8.RuneCountInString(s) }

// fit cuts s to w columns (with "…") or pads it with spaces.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	n := runes(s)
	if n > w {
		r := []rune(s)
		if w == 1 {
			return "…"
		}
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-n)
}

// fitLeft is fit that keeps the end (file names): "…/auth/redirect.ts".
func fitLeft(s string, w int) string {
	n := runes(s)
	if n <= w || w <= 1 {
		return fit(s, w)
	}
	r := []rune(s)
	return "…" + string(r[n-w+1:])
}

// cols lays out a left part and a right-aligned right part in w columns.
func cols(left, right string, w int) (string, string) {
	if right == "" {
		return fit(left, w), ""
	}
	rw := runes(right)
	if rw+2 > w {
		return fit(left, w), ""
	}
	return fit(left, w-rw-1) + " ", right
}

// stateText is the label of a pane state ("" for a shell).
func stateText(st string) string {
	switch st {
	case StateWorking:
		return i18n.T("sidebar.state.working")
	case StateExited:
		return i18n.T("sidebar.state.exited")
	case StateShell:
		return ""
	}
	return i18n.T("sidebar.state.idle")
}

// dot is a pane's state symbol and color.
func dot(p PaneRow) (string, string) {
	switch {
	case p.Limited != "":
		return "◆", red
	case p.State == StateWorking:
		return "●", green
	case p.State == StateExited:
		return "✕", red
	case p.State == StateShell:
		return "·", dim
	}
	return "○", dim
}

// layout says how much of the sidebar is drawn; Render shrinks it until it
// fits the height.
type layout struct {
	changes int  // change rows shown (-1 = all)
	detail  bool // second line of agent cards
	others  int  // other projects shown (-1 = all)
	blanks  bool // blank lines between sections
}

// Render draws the sidebar for w columns and h rows (0 = no limit) with sel
// selected (keyboard; NoItem for none). Sections (§9.3 "Sidebar v3"):
// PROJECTS, AGENTS, CHANGES, MOVE THIS CHAT TO, then the toolbar anchored to
// the bottom. When the rows do not fit, changes shrink first, then agent
// cards lose their second line, then other projects, then blank lines go.
func Render(m Model, w, h int, sel Item) Screen {
	lay := layout{changes: -1, detail: true, others: -1, blanks: true}
	lines := build(m, w, lay)
	if h > 0 {
		steps := []func(){
			func() { lay.changes = min(2, len(m.Changes)) },
			func() { lay.detail = false },
			func() { lay.others = 0 },
			func() { lay.blanks = false },
			func() { lay.changes = 0 },
		}
		for _, step := range steps {
			if len(lines) <= h {
				break
			}
			step()
			lines = build(m, w, lay)
		}
		if len(lines) > h {
			// Still too long: keep the toolbar, cut the content.
			tb := toolbar(w)
			keep := max(h-len(tb), 0)
			lines = append(lines[:min(keep, len(lines)-len(tb))], tb...)
			if len(lines) > h {
				lines = lines[len(lines)-h:]
			}
		} else {
			// Anchor the toolbar to the bottom.
			tb := len(toolbar(w))
			fill := make([]Line, h-len(lines))
			for i := range fill {
				fill[i] = Line{Text: strings.Repeat(" ", w)}
			}
			body := append(append([]Line{}, lines[:len(lines)-tb]...), fill...)
			lines = append(body, lines[len(lines)-tb:]...)
		}
	}
	sc := Screen{Lines: lines}
	highlight(sc, sel)
	return sc
}

// highlight draws the keyboard selection in reverse video.
func highlight(sc Screen, sel Item) {
	if sel.Row < 0 || sel.Row >= len(sc.Lines) {
		return
	}
	l := &sc.Lines[sel.Row]
	revd := func(t string) string { return rev + strings.ReplaceAll(t, reset, reset+rev) + reset }
	if len(l.Cells) == 0 {
		if l.Action.Kind != ActNone {
			l.Text = revd(l.Text)
		}
		return
	}
	if sel.Cell < 0 || sel.Cell >= len(l.Cells) {
		return
	}
	cells := append([]Cell{}, l.Cells...)
	cells[sel.Cell].Text = revd(cells[sel.Cell].Text)
	l.Text = compose(cells, runes(Plain(l.Text)))
}

// compose lays out cells on a w-column row.
func compose(cells []Cell, w int) string {
	var b strings.Builder
	at := 0
	for _, c := range cells {
		if c.X > at {
			b.WriteString(strings.Repeat(" ", c.X-at))
		}
		b.WriteString(c.Text)
		at = c.X + c.W
	}
	if w > at {
		b.WriteString(strings.Repeat(" ", w-at))
	}
	return b.String()
}

// toolbar is the bottom block: a rule, two rows of two buttons, the hint.
func toolbar(w int) []Line {
	cw := (w - 2) / 2
	btn := func(x int, icon, label string, a Action) Cell {
		return Cell{X: x, W: cw, Text: " " + dim + icon + reset + " " + fit(label, cw-3), Action: a}
	}
	row := func(a, b Cell) Line {
		cells := []Cell{a, b}
		return Line{Text: compose(cells, w), Cells: cells}
	}
	return []Line{
		{Text: dim + strings.Repeat("─", w) + reset},
		row(btn(1, "✎", i18n.T("sidebar.rename"), Action{Kind: ActRename}),
			btn(1+cw, "⧉", i18n.T("sidebar.history"), Action{Kind: ActSessions})),
		row(btn(1, "◔", i18n.T("sidebar.limits"), Action{Kind: ActLimits}),
			btn(1+cw, "?", i18n.T("sidebar.help"), Action{Kind: ActKeys})),
		{Text: dim + fit(" "+i18n.T("sidebar.footer"), w) + reset},
	}
}

// build draws every section with layout lay; the toolbar comes last.
func build(m Model, w int, lay layout) []Line {
	plain := func(style, s string) string { return style + fit(s, w) + reset }
	heading := func(left, right string) Line {
		l, r := cols(" "+left, right, w-1)
		return Line{Text: bold + dim + l + reset + dim + r + reset + " "}
	}
	add := func(label string, a Action) Line {
		return Line{Text: "   " + cyan + "+" + reset + " " + fit(label, w-5), Action: a}
	}
	var out []Line
	blank := func() {
		if lay.blanks && len(out) > 0 {
			out = append(out, Line{Text: strings.Repeat(" ", w)})
		}
	}

	// PROJECTS: one row each, with the state dots of their agents.
	out = append(out, heading(i18n.T("sidebar.projects"), ""))
	var cur *ProjectRow
	shown, limit := 0, lay.others
	if limit >= 0 && len(m.Projects)-1-limit == 1 {
		limit++ // "+1 more" would take the row the project takes
	}
	for i := range m.Projects {
		pr := &m.Projects[i]
		if pr.Current {
			cur = pr
		} else {
			if limit >= 0 && shown >= limit {
				continue
			}
			shown++
		}
		var dots strings.Builder
		n := min(len(pr.Panes), 4)
		for _, p := range pr.Panes[:n] {
			sym, color := dot(p)
			dots.WriteString(color + sym + reset)
		}
		extra := ""
		if len(pr.Panes) > n {
			extra = dim + "+" + strconv.Itoa(len(pr.Panes)-n) + reset
		}
		dw := n + runes(Plain(extra))
		mark, style, a := " ", "", Action{Kind: ActProject, Session: pr.Session}
		if pr.Current {
			mark, style, a = cyan+"▌"+reset, bold, Action{}
		}
		out = append(out, Line{
			Text:   " " + mark + style + fit(pr.Name, w-3-dw-1) + reset + " " + dots.String() + extra + " ",
			Action: a,
		})
	}
	if hidden := len(m.Projects) - 1 - shown; cur != nil && hidden > 0 {
		out = append(out, Line{Text: plain(dim, "   "+i18n.T("sidebar.more_projects", hidden))})
	}
	out = append(out, add(i18n.T("sidebar.open_project"), Action{Kind: ActOpen}))

	// AGENTS: the current project's panes as cards.
	blank()
	out = append(out, heading(i18n.T("sidebar.agents"), ""))
	if cur == nil || len(cur.Panes) == 0 {
		out = append(out, Line{Text: plain(dim, "   "+i18n.T("sidebar.no_panes"))})
	} else {
		for _, p := range cur.Panes {
			out = append(out, card(p, w, lay.detail)...)
		}
	}
	out = append(out, add(i18n.T("sidebar.new_agent"), Action{Kind: ActNewAgent}))

	// CHANGES (git only).
	if m.Git && lay.changes != 0 {
		blank()
		title := i18n.T("sidebar.changes")
		if m.Branch != "" {
			title += " · " + m.Branch
		}
		count := ""
		if m.Changed > 0 {
			count = strconv.Itoa(m.Changed)
		}
		out = append(out, heading(title, count))
		if len(m.Changes) == 0 {
			out = append(out, Line{Text: plain(dim, "   "+i18n.T("sidebar.no_changes"))})
		}
		changes := m.Changes
		if lay.changes > 0 && len(changes) > lay.changes {
			changes = changes[:lay.changes]
		}
		for _, c := range changes {
			color := yellow
			switch c.Status {
			case "A":
				color = green
			case "D":
				color = red
			case "?":
				color = dim
			case "R":
				color = cyan
			}
			out = append(out, Line{
				Text:   "   " + color + fit(c.Status, 1) + reset + " " + fitLeft(c.Path, w-6) + " ",
				Action: Action{Kind: ActDiff, Path: c.Path, Status: c.Status},
			})
		}
		if hidden := m.Changed - len(changes); hidden > 0 {
			out = append(out, Line{Text: plain(dim, "   "+i18n.T("sidebar.more", hidden))})
		}
	}

	// MOVE THIS CHAT TO: the other agents, with their maker.
	var moves []Line
	for _, a := range m.Agents {
		if a.Current {
			continue
		}
		right, style, rstyle := a.Vendor, "", dim
		switch {
		case !a.Installed:
			right, style, rstyle = i18n.T("sidebar.not_installed"), dim, dim
		case a.Limited != "":
			right, rstyle = i18n.T("sidebar.limited", a.Limited), red
		}
		l, r := cols(a.Name, right, w-4)
		moves = append(moves, Line{
			Text:   "   " + style + l + reset + rstyle + r + reset + " ",
			Action: Action{Kind: ActSwitch, Agent: a.ID},
		})
	}
	if len(moves) > 0 && m.Target != "" {
		blank()
		out = append(out, heading(i18n.T("sidebar.move_to"), ""))
		out = append(out, moves...)
	}
	if lay.blanks {
		out = append(out, Line{Text: strings.Repeat(" ", w)})
	}
	return append(out, toolbar(w)...)
}

// card draws one agent pane of the current project: its name (or agent) and
// state, and with detail a dim second line with the agent (and model) when
// the name does not say it already.
func card(p PaneRow, w int, detail bool) []Line {
	mark, style := " ", ""
	if p.Target {
		mark, style = cyan+"▌"+reset, bold
	}
	sym, color := dot(p)
	short := session.ShortName(p.Agent)
	name, sub := short, ""
	switch {
	case p.Agent == "":
		name = i18n.T("sidebar.shell")
	case p.Label != "" && p.Label != short:
		sub = strings.TrimPrefix(p.Label, short+" · ")
	}
	if p.Name != "" {
		name, sub = p.Name, p.Label
		if p.Agent == "" {
			sub = i18n.T("sidebar.shell")
		}
		if sub == "" {
			sub = short
		}
	}
	right, rcolor := stateText(p.State), color
	if p.State == StateIdle {
		rcolor = dim
	}
	if p.Limited != "" {
		right, rcolor = i18n.T("sidebar.limited", p.Limited), red
	}
	a := Action{Kind: ActFocus, Pane: p.ID}
	l, r := cols(name, right, w-5)
	out := []Line{{
		Text:   " " + mark + color + sym + reset + " " + style + l + reset + rcolor + r + reset + " ",
		Action: a,
	}}
	if detail && sub != "" {
		out = append(out, Line{Text: "     " + dim + fit(sub, w-6) + reset + " ", Action: a})
	}
	return out
}

// Frame turns a screen into the bytes that draw it on an h-row terminal:
// cursor home, each line erased to its end, the rest of the screen cleared.
func Frame(sc Screen, h int) string {
	var b strings.Builder
	b.WriteString("\x1b[H")
	for i, l := range sc.Lines {
		if h > 0 && i >= h {
			break
		}
		if i > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(l.Text)
		b.WriteString("\x1b[K")
	}
	b.WriteString("\x1b[J")
	return b.String()
}

// Plain strips escapes from rendered text (tests, hit-testing by text).
func Plain(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
