package sidebar

import (
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
	ActFocus               // focus pane Pane
	ActSwitch              // continue the target pane in Agent (with context)
	ActSplitH              // split the target pane side by side (agent chooser)
	ActSplitV              // split it one above the other
	ActSessions            // session picker
	ActLimits              // limits popup
	ActKeys                // keys help
)

// Action is a row's action.
type Action struct {
	Kind  ActionKind
	Pane  string
	Agent session.AgentID
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

// Render draws the sidebar for a width of w columns. sel is the selected
// row (keyboard), or -1.
func Render(m Model, w int, sel int) Screen {
	var sc Screen
	add := func(text string, a Action) { sc.Lines = append(sc.Lines, Line{Text: text, Action: a}) }
	plain := func(style, s string) string { return style + fit(s, w) + reset }
	sep := func() { add(dim+strings.Repeat("─", w)+reset, Action{}) }
	heading := func(key string) { add(plain(dim+bold, " "+i18n.T(key)), Action{}) }
	blank := func() { add(strings.Repeat(" ", w), Action{}) }

	// The pane border already says "i-mux": the first line is the project.
	title := " i-mux"
	if m.Project != "" {
		title = " " + m.Project
	}
	add(plain(bold, title), Action{})
	switch {
	case m.Git && m.Branch != "" && m.Changed > 0:
		add(plain(dim, "  "+i18n.T("sidebar.branch_changed", m.Branch, m.Changed)), Action{})
	case m.Git && m.Branch != "":
		add(plain(dim, "  "+i18n.T("sidebar.branch", m.Branch)), Action{})
	default:
		blank()
	}
	sep()

	heading("sidebar.panes")
	for _, p := range m.Panes {
		mark := " "
		if p.Target {
			mark = cyan + "▌" + reset
		}
		sym, color, state := "○", dim, stateText(p.State)
		switch {
		case p.Limited != "":
			sym, color, state = "◆", red, i18n.T("sidebar.limited", p.Limited)
		case p.State == StateWorking:
			sym, color = "●", green
		case p.State == StateExited:
			sym, color = "✕", red
		case p.State == StateShell:
			sym = "·"
		}
		name := p.Label
		if name == "" {
			name = i18n.T("sidebar.shell")
		}
		l, r := cols(name, state, w-4)
		text := mark + color + sym + reset + " " + l + color + r + reset + " "
		if p.Target {
			text = mark + color + sym + reset + " " + bold + l + reset + color + r + reset + " "
		}
		add(text, Action{Kind: ActFocus, Pane: p.ID})
	}
	if len(m.Panes) == 0 {
		add(plain(dim, "  "+i18n.T("sidebar.no_panes")), Action{})
	}
	sep()

	heading("sidebar.continue")
	for _, a := range m.Agents {
		var right, style string
		switch {
		case a.Current:
			right, style = i18n.T("sidebar.current"), bold
		case !a.Installed:
			right, style = i18n.T("sidebar.not_installed"), dim
		case a.Limited != "":
			right, style = i18n.T("sidebar.limited", a.Limited), red
		}
		l, r := cols(a.Name, right, w-4)
		key := dim + a.Key + reset
		if !a.Installed {
			key = " "
		}
		add(" "+key+" "+style+l+r+reset+" ", Action{Kind: ActSwitch, Agent: a.ID})
	}
	sep()

	for _, b := range []struct {
		key, label string
		kind       ActionKind
	}{
		{"v", "sidebar.split_right", ActSplitH},
		{"-", "sidebar.split_down", ActSplitV},
		{"s", "sidebar.sessions", ActSessions},
		{"l", "sidebar.limits", ActLimits},
		{"?", "sidebar.keys", ActKeys},
	} {
		add(" "+dim+b.key+reset+" "+fit(i18n.T(b.label), w-4)+" ", Action{Kind: b.kind})
	}
	sep()
	add(plain(dim, " "+i18n.T("sidebar.footer")), Action{})

	if sel >= 0 && sel < len(sc.Lines) && sc.Lines[sel].Action.Kind != ActNone {
		// Reverse video for the keyboard selection, without inner resets.
		t := sc.Lines[sel].Text
		t = strings.ReplaceAll(t, reset, reset+rev)
		sc.Lines[sel].Text = rev + t + reset
	}
	return sc
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
