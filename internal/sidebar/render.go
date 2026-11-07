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

// Line is one rendered row: its text with escapes, and its action.
type Line struct {
	Text   string
	Action Action
}

// Screen is a rendered sidebar.
type Screen struct {
	Lines []Line
}

// Selectable returns the indexes of rows with an action.
func (s Screen) Selectable() []int {
	var out []int
	for i, l := range s.Lines {
		if l.Action.Kind != ActNone {
			out = append(out, i)
		}
	}
	return out
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

// stateText is the label of a pane state.
func stateText(st string) string {
	switch st {
	case StateWorking:
		return i18n.T("sidebar.state.working")
	case StateExited:
		return i18n.T("sidebar.state.exited")
	case StateShell:
		return i18n.T("sidebar.state.shell")
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

// Render draws the sidebar for w columns and h rows (0 = no limit). sel is
// the selected row (keyboard), or -1. When the rows do not fit, changes and
// other projects shrink to a "+N more" row first; the buttons always stay.
func Render(m Model, w, h, sel int) Screen {
	plain := func(style, s string) string { return style + fit(s, w) + reset }
	sepLine := Line{Text: dim + strings.Repeat("─", w) + reset}
	heading := func(text string) Line { return Line{Text: plain(dim+bold, " "+text)} }
	button := func(key, label string, a Action) Line {
		return Line{Text: " " + dim + fit(key, 1) + reset + " " + fit(label, w-4) + " ", Action: a}
	}

	// PROJECTS: the current project expanded, the others one row each.
	var cur, others []Line
	for _, pr := range m.Projects {
		if pr.Current {
			l, r := cols("▾ "+pr.Name, i18n.T("sidebar.current_project"), w-1)
			cur = append(cur, Line{Text: " " + bold + l + reset + dim + r + reset})
			for _, p := range pr.Panes {
				cur = append(cur, paneLine(p, w))
			}
			continue
		}
		var dots strings.Builder
		shown := min(len(pr.Panes), 4)
		for _, p := range pr.Panes[:shown] {
			sym, color := dot(p)
			dots.WriteString(color + sym + reset)
		}
		count := strconv.Itoa(len(pr.Panes))
		name := fit("▸ "+pr.Name, w-1-shown-1-runes(count)-1)
		others = append(others, Line{
			Text:   " " + name + " " + dots.String() + " " + dim + count + reset,
			Action: Action{Kind: ActProject, Session: pr.Session},
		})
	}
	if len(m.Projects) == 0 {
		cur = append(cur, Line{Text: plain(dim, "  "+i18n.T("sidebar.no_panes"))})
	}
	open := button("o", "+ "+i18n.T("sidebar.open_project"), Action{Kind: ActOpen})

	// CHANGES (git only).
	var changes []Line
	if m.Git {
		title := i18n.T("sidebar.changes")
		if m.Branch != "" {
			title += " · " + m.Branch
		}
		changes = append(changes, heading(title))
		if len(m.Changes) == 0 {
			changes = append(changes, Line{Text: plain(dim, "  "+i18n.T("sidebar.no_changes"))})
		}
		for _, c := range m.Changes {
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
			changes = append(changes, Line{
				Text:   "  " + color + fit(c.Status, 1) + reset + " " + fitLeft(c.Path, w-5) + " ",
				Action: Action{Kind: ActDiff, Path: c.Path, Status: c.Status},
			})
		}
	}

	// SWITCH TO: the agents, with their maker when nothing else is shown.
	agents := []Line{heading(i18n.T("sidebar.continue"))}
	for _, a := range m.Agents {
		right, style, rstyle := a.Vendor, "", dim
		switch {
		case a.Current:
			right, style, rstyle = i18n.T("sidebar.current"), bold, bold
		case !a.Installed:
			right, style, rstyle = i18n.T("sidebar.not_installed"), dim, dim
		case a.Limited != "":
			right, style, rstyle = i18n.T("sidebar.limited", a.Limited), red, red
		}
		l, r := cols(a.Name, right, w-4)
		key := dim + a.Key + reset
		if !a.Installed {
			key = " "
		}
		agents = append(agents, Line{
			Text:   " " + key + " " + style + l + reset + rstyle + r + reset + " ",
			Action: Action{Kind: ActSwitch, Agent: a.ID},
		})
	}

	buttons := []Line{
		button("n", "✎ "+i18n.T("sidebar.rename"), Action{Kind: ActRename}),
		button("v", "→ "+i18n.T("sidebar.split_right"), Action{Kind: ActSplitH}),
		button("-", "↓ "+i18n.T("sidebar.split_down"), Action{Kind: ActSplitV}),
		button("s", "⧉ "+i18n.T("sidebar.sessions"), Action{Kind: ActSessions}),
		button("l", "◔ "+i18n.T("sidebar.limits"), Action{Kind: ActLimits}),
		button("?", "≡ "+i18n.T("sidebar.keys"), Action{Kind: ActKeys}),
	}
	footer := Line{Text: plain(dim, " "+i18n.T("sidebar.footer"))}

	assemble := func(others, changes []Line) []Line {
		out := []Line{heading(i18n.T("sidebar.projects"))}
		out = append(out, cur...)
		out = append(out, others...)
		out = append(out, open, sepLine)
		if len(changes) > 0 {
			out = append(out, changes...)
			out = append(out, sepLine)
		}
		out = append(out, agents...)
		out = append(out, sepLine)
		out = append(out, buttons...)
		return append(out, sepLine, footer)
	}
	more := func(n int, key string) Line { return Line{Text: plain(dim, "  "+i18n.T(key, n))} }

	// git status was capped; say how many more files changed.
	hidden := m.Changed - len(m.Changes)
	lines := assemble(others, changes)
	if h > 0 && len(lines) > h && len(changes) > 2 {
		keep := max(len(changes)-(len(lines)-h)-1, 2)
		hidden += len(changes) - keep
		changes = changes[:keep]
	}
	if hidden > 0 && len(changes) > 0 {
		changes = append(changes, more(hidden, "sidebar.more"))
	}
	lines = assemble(others, changes)
	if h > 0 && len(lines) > h && len(others) > 0 {
		keep := max(len(others)-(len(lines)-h)-1, 0)
		trimmed := append(append([]Line{}, others[:keep]...), more(len(others)-keep, "sidebar.more_projects"))
		lines = assemble(trimmed, changes)
	}
	if h > 0 && len(lines) > h {
		lines = lines[:len(lines)-2] // separator and footer
	}

	sc := Screen{Lines: lines}
	if sel >= 0 && sel < len(sc.Lines) && sc.Lines[sel].Action.Kind != ActNone {
		// Reverse video for the keyboard selection, without inner resets.
		t := strings.ReplaceAll(sc.Lines[sel].Text, reset, reset+rev)
		sc.Lines[sel].Text = rev + t + reset
	}
	return sc
}

// paneLine draws one pane of the current project: its name (or agent), and
// the agent when it is named, else its state.
func paneLine(p PaneRow, w int) Line {
	mark := " "
	if p.Target {
		mark = cyan + "▌" + reset
	}
	sym, color := dot(p)
	name, right, rcolor := p.Label, stateText(p.State), color
	if name == "" {
		name = i18n.T("sidebar.shell")
	}
	if p.Name != "" {
		name, right, rcolor = p.Name, session.ShortName(p.Agent), dim
		if p.Agent == "" {
			right = i18n.T("sidebar.shell")
		}
	}
	if p.Limited != "" {
		right, rcolor = i18n.T("sidebar.limited", p.Limited), red
	}
	l, r := cols(name, right, w-5)
	style := ""
	if p.Target {
		style = bold
	}
	return Line{
		Text:   " " + mark + color + sym + reset + " " + style + l + reset + rcolor + r + reset + " ",
		Action: Action{Kind: ActFocus, Pane: p.ID},
	}
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
