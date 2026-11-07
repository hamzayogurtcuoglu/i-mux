package sidebar

import (
	"strconv"
	"strings"
	"time"
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
	ActNewAgent            // split the target pane (direction by its shape) with an agent chooser
	ActChanges             // menu of the changed files, each opening its diff
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

// 256-color styles of the v4 look. Backgrounds are used only for the
// highlighted rows, the project avatars and the state pills.
const (
	muted     = "\x1b[38;5;245m"
	faint     = "\x1b[38;5;240m"
	selBg     = "\x1b[48;5;236m"
	spinFg    = "\x1b[38;5;75m"
	doneFg    = "\x1b[38;5;71m"
	dirtyFg   = "\x1b[38;5;179m"
	needPill  = "\x1b[48;5;58m\x1b[38;5;214m"
	limitPill = "\x1b[48;5;52m\x1b[38;5;210m"
	white     = "\x1b[38;5;255m"
)

// avatarBg are the avatar colors; a project's comes from its name.
var avatarBg = []int{30, 166, 68, 71, 98, 168, 136, 37}

// spinner frames for working agents (narrow braille glyphs).
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// icon is an agent's glyph and color.
func icon(a session.AgentID) (string, string) {
	switch a {
	case session.Claude:
		return "✻", "\x1b[38;5;173m"
	case session.Antigravity:
		return "▲", "\x1b[38;5;75m"
	case session.Codex:
		return "◎", "\x1b[38;5;252m"
	case session.Copilot:
		return "◉", "\x1b[38;5;141m"
	}
	return "›", faint
}

// avatar is a project's letter on its color (3 columns).
func avatar(name string) string {
	r := []rune(strings.TrimLeft(name, ".-_ "))
	letter := "?"
	if len(r) > 0 {
		letter = strings.ToUpper(string(r[0]))
	}
	var h uint32
	for _, c := range name {
		h = h*31 + uint32(c)
	}
	return "\x1b[48;5;" + strconv.Itoa(avatarBg[h%uint32(len(avatarBg))]) + "m" + white + bold + " " + letter + " " + reset
}

// age is the time since since: "<1m", "9m", "2h", "3d" ("" if unknown).
func age(now, since time.Time) string {
	if since.IsZero() || now.IsZero() {
		return ""
	}
	d := now.Sub(since)
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	}
	return strconv.Itoa(int(d/(24*time.Hour))) + "d"
}

// badge is a pane's state indicator and its width. compact draws the pills
// as one "!" (narrow rows).
func badge(p PaneRow, tick int, compact bool) (string, int) {
	pill := func(style, text string) (string, int) {
		if compact {
			return style + "!" + reset, 1
		}
		return style + " " + text + " " + reset, runes(text) + 2
	}
	switch {
	case p.Limited != "":
		return pill(limitPill, p.Limited)
	case p.State == StateBlocked:
		return pill(needPill, i18n.T("sidebar.needs_you"))
	case p.State == StateWorking:
		return spinFg + spinner[tick%len(spinner)] + reset, 1
	case p.State == StateDone:
		return doneFg + "●" + reset, 1
	case p.State == StateExited:
		return red + "✕" + reset, 1
	case p.State == StateIdle:
		return faint + "○" + reset, 1
	}
	return "", 0
}

// urgency orders states for a project's roll-up.
func urgency(p PaneRow) int {
	switch {
	case p.State == StateBlocked:
		return 5
	case p.Limited != "":
		return 4
	case p.State == StateDone:
		return 3
	case p.State == StateWorking:
		return 2
	}
	return 0
}

// rollup is the most urgent pane of a project (ok false when all are idle).
func rollup(panes []PaneRow) (PaneRow, bool) {
	var best PaneRow
	for _, p := range panes {
		if urgency(p) > urgency(best) {
			best = p
		}
	}
	return best, urgency(best) > 0
}

// spread lays out left (lw columns) and right (rw columns) on a w-column row.
func spread(left string, lw int, right string, rw, w int) string {
	return left + strings.Repeat(" ", max(w-lw-rw, 0)) + right
}

// withBg draws a whole row on the highlight background.
func withBg(t string) string {
	return selBg + strings.ReplaceAll(t, reset, reset+selBg) + reset
}

// header is a section title with a "+" button on the right.
func header(title string, w int, plus Action) Line {
	cells := []Cell{{X: 0, W: w - 3, Text: muted + fit(" "+title, w-3) + reset}}
	if plus.Kind != ActNone {
		cells = append(cells, Cell{X: w - 3, W: 3, Text: muted + " + " + reset, Action: plus})
	}
	return Line{Text: compose(cells, w), Cells: cells}
}

// projectRow: avatar, name, branch, dirty dot and the roll-up state.
func projectRow(pr ProjectRow, w, tick int) Line {
	var right []string
	rw := 0
	add := func(t string, n int) {
		if n == 0 {
			return
		}
		if rw > 0 {
			right, rw = append(right, " "), rw+1
		}
		right, rw = append(right, t), rw+n
	}
	st, ok := rollup(pr.Panes)
	room := func() int { return w - 5 - rw - 2 }
	branch := pr.Branch
	build := func(compact bool) {
		right, rw = nil, 0
		if branch != "" {
			add(muted+branch+reset, runes(branch))
		}
		if pr.Dirty {
			add(dirtyFg+"●"+reset, 1)
		}
		if ok {
			add(badge(st, tick, compact))
		}
	}
	build(false)
	if room() < 8 {
		build(true)
	}
	if room() < 6 && branch != "" {
		branch = ""
		build(true)
	}
	if room() < 6 {
		right, rw = nil, 0
	}
	name := pr.Name
	style := ""
	if pr.Current {
		style = bold
	}
	nw := min(runes(name), max(room(), 1))
	left := " " + avatar(pr.Name) + " " + style + fit(name, nw) + reset
	text := spread(left, 5+nw, strings.Join(right, "")+" ", rw+1, w)
	a := Action{Kind: ActProject, Session: pr.Session}
	if pr.Current {
		return Line{Text: withBg(text)}
	}
	return Line{Text: text, Action: a}
}

// agentRow: icon, name, time since the state changed and the state.
func agentRow(p PaneRow, w int, now time.Time, tick int) Line {
	sym, color := icon(p.Agent)
	name := p.Display(i18n.T("sidebar.shell"))
	nameStyle := ""
	if p.State == StateIdle || p.State == StateShell || p.State == StateExited {
		nameStyle = muted
	}
	if p.Target {
		nameStyle = bold
	}
	b, bw := badge(p, tick, false)
	t := age(now, p.Since)
	if p.State == StateShell {
		t = ""
	}
	right, rw := "", 0
	layout := func() {
		right, rw = "", 0
		if t != "" {
			right, rw = muted+t+reset, runes(t)
		}
		if bw > 0 {
			if rw > 0 {
				right, rw = right+" ", rw+1
			}
			right, rw = right+b, rw+bw
		}
	}
	layout()
	room := func() int { return w - 4 - rw - 2 }
	if room() < 8 && bw > 1 {
		b, bw = badge(p, tick, true)
		layout()
	}
	if room() < 6 {
		t = ""
		layout()
	}
	nw := min(runes(name), max(room(), 1))
	left := " " + color + sym + reset + "  " + nameStyle + fit(name, nw) + reset
	text := spread(left, 4+nw, right+" ", rw+1, w)
	if p.Target {
		text = withBg(text)
	}
	return Line{Text: text, Action: Action{Kind: ActFocus, Pane: p.ID}}
}

// groupRow is a project's name above its agents; collapsed, it also shows
// how many agents it has and their roll-up state.
func groupRow(pr ProjectRow, w, tick int, collapsed bool) Line {
	a := Action{}
	if !pr.Current {
		a = Action{Kind: ActProject, Session: pr.Session}
	}
	if !collapsed {
		return Line{Text: faint + fit(" "+pr.Name, w) + reset, Action: a}
	}
	right, rw := muted+strconv.Itoa(len(pr.Panes))+reset, runes(strconv.Itoa(len(pr.Panes)))
	if st, ok := rollup(pr.Panes); ok {
		b, bw := badge(st, tick, true)
		right, rw = right+" "+b, rw+1+bw
	}
	nw := max(w-rw-3, 1)
	return Line{Text: spread(faint+" "+fit(pr.Name, nw)+reset, nw+1, right+" ", rw+1, w), Action: a}
}

// continueRows: the agents the chat can move to, two buttons per row.
func continueRows(m Model, w int) []Line {
	cw := w / 2
	var cells []Cell
	for _, a := range m.Agents {
		if a.Current {
			continue
		}
		sym, color := icon(a.ID)
		style := ""
		switch {
		case !a.Installed:
			style, color = faint, faint
		case a.Limited != "":
			style = red
		}
		cells = append(cells, Cell{W: cw, Text: " " + color + sym + reset + " " + style + fit(a.Name, cw-3) + reset, Action: Action{Kind: ActSwitch, Agent: a.ID}})
	}
	var out []Line
	for i := 0; i < len(cells); i += 2 {
		row := []Cell{cells[i]}
		row[0].X = 0
		if i+1 < len(cells) {
			c := cells[i+1]
			c.X = cw
			row = append(row, c)
		}
		out = append(out, Line{Text: compose(row, w), Cells: row})
	}
	return out
}

// toolbar is the bottom block: Rename and Changes, then History, Limits and
// Help.
func toolbar(m Model, w int) []Line {
	btn := func(x, cw int, sym, label string, a Action) Cell {
		return Cell{X: x, W: cw, Text: muted + sym + reset + " " + fit(label, cw-2), Action: a}
	}
	row := func(cells ...Cell) Line { return Line{Text: compose(cells, w), Cells: cells} }
	half := (w - 1) / 2
	first := []Cell{btn(1, half, "✎", i18n.T("sidebar.rename"), Action{Kind: ActRename})}
	if m.Git && m.Changed > 0 {
		first = append(first, btn(1+half, half, "±", i18n.T("sidebar.changes_n", m.Changed), Action{Kind: ActChanges}))
	}
	third := (w - 1) / 3
	return []Line{
		row(first...),
		row(btn(1, third, "⧉", i18n.T("sidebar.history"), Action{Kind: ActSessions}),
			btn(1+third, third, "◔", i18n.T("sidebar.limits"), Action{Kind: ActLimits}),
			btn(1+2*third, w-1-2*third, "?", i18n.T("sidebar.help"), Action{Kind: ActKeys})),
	}
}

// top is the Projects and Agents blocks. Without blanks there are no empty
// rows; collapse shows the other projects' agents as one row each.
func top(m Model, w int, blanks, collapse bool) []Line {
	rule := Line{Text: faint + strings.Repeat("─", w) + reset}
	out := []Line{header(i18n.T("sidebar.projects"), w, Action{Kind: ActOpen})}
	for _, pr := range m.Projects {
		out = append(out, projectRow(pr, w, m.Tick))
	}
	if blanks {
		out = append(out, Line{Text: strings.Repeat(" ", w)})
	}
	out = append(out, rule, header(i18n.T("sidebar.agents"), w, Action{Kind: ActNewAgent}))
	for _, pr := range m.Projects {
		if len(pr.Panes) == 0 {
			continue
		}
		folded := collapse && !pr.Current
		out = append(out, groupRow(pr, w, m.Tick, folded))
		if folded {
			continue
		}
		for _, p := range pr.Panes {
			out = append(out, agentRow(p, w, m.Now, m.Tick))
		}
	}
	return out
}

// bottom is the "Continue this chat in" block and the toolbar.
func bottom(m Model, w int) []Line {
	rule := Line{Text: faint + strings.Repeat("─", w) + reset}
	var out []Line
	if m.Target != "" {
		if rows := continueRows(m, w); len(rows) > 0 {
			out = append(out, rule, Line{Text: muted + fit(" "+i18n.T("sidebar.continue_in"), w) + reset})
			out = append(out, rows...)
		}
	}
	out = append(out, rule)
	return append(out, toolbar(m, w)...)
}

// Render draws the sidebar for w columns and h rows (0 = no limit) with sel
// selected (keyboard; NoItem for none), §9.3 "Sidebar v4": Projects and
// Agents at the top, "Continue this chat in" and the toolbar at the bottom.
// When the rows do not fit, blank rows go first, then the other projects'
// agents fold into one row each, then the top is cut.
func Render(m Model, w, h int, sel Item) Screen {
	up, down := top(m, w, true, false), bottom(m, w)
	if h > 0 && len(up)+len(down) > h {
		up = top(m, w, false, false)
		if len(up)+len(down) > h {
			up = top(m, w, false, true)
		}
		if len(up)+len(down) > h {
			up = up[:max(h-len(down), 0)]
		}
		if len(down) > h {
			down = down[len(down)-h:]
		}
	}
	lines := append([]Line{}, up...)
	if h > 0 {
		for len(lines)+len(down) < h {
			lines = append(lines, Line{Text: strings.Repeat(" ", w)})
		}
	}
	sc := Screen{Lines: append(lines, down...)}
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

// Animated reports whether the screen shows a spinner (a working agent).
func (m Model) Animated() bool {
	for _, pr := range m.Projects {
		for _, p := range pr.Panes {
			if p.State == StateWorking && p.Limited == "" {
				return true
			}
		}
	}
	return false
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
