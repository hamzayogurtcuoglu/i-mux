// Package skilldialog is the New skill dialog (D32, §9.3 "Skills"): a popup
// that asks who writes a project skill (an agent in a new pane, the agent of
// the current pane, or the user from a blank template), then what the skill
// should do, or the template's name. It only asks: `imux __skill` acts on
// the answer.
package skilldialog

import (
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/skills"
)

const (
	reset    = "\x1b[0m"
	bold     = "\x1b[1m"
	cyan     = "\x1b[36m"
	yellow   = "\x1b[33m"
	muted    = "\x1b[38;5;247m"
	faint    = "\x1b[38;5;240m"
	selBg    = "\x1b[48;5;24m"
	cursorOn = "\x1b[7m" // the text cursor: reverse video
	cursorNo = "\x1b[27m"
	maxRows  = 8 // the description box's most lines
)

// Kind is how the dialog ended.
type Kind int

// How the dialog ends.
const (
	Cancel   Kind = iota // closed
	NewPane              // Agent writes the skill in a new pane; Text is what it should do ("": the agent asks)
	Here                 // the agent of the current pane writes it; Text likewise
	Template             // a blank SKILL.md; Text is its name (valid, not taken)
)

// Result is the dialog's answer.
type Result struct {
	Kind  Kind
	Agent session.AgentID
	Text  string
}

// Agent is an agent CLI the dialog offers.
type Agent struct {
	ID        session.AgentID
	Name      string // "Claude"
	Key       string // its letter
	Installed bool
	Limited   string // "15:00" while its usage limit lasts
}

// Options fill the dialog.
type Options struct {
	Agents   []Agent         // in config order
	Current  session.AgentID // the agent of the pane the sidebar acts on ("" for none)
	Template bool            // open at the blank template's name
	Exists   func(name string) bool
}

type item struct {
	kind  Kind
	agent Agent
	key   string
}

// ok reports an item that can be chosen (an agent that is installed).
func (it item) ok() bool { return it.kind != NewPane || it.agent.Installed }

// field is an editable text and its cursor (in runes).
type field struct {
	r   []rune
	cur int
	top int // the first line shown
}

// Model is the dialog's state: step 0 picks who writes the skill, step 1
// takes the description or the name.
type Model struct {
	items  []item
	exists func(string) bool
	sel    int
	step   int
	chosen item
	direct bool // opened at the name: Esc closes
	desc   field
	name   field
	err    string
	boxW   int   // the text box's width as last drawn
	rows   []int // step 0: the item on each drawn row (-1 for none)
}

// New makes the dialog for o. The current pane's agent is chosen first.
func New(o Options) *Model {
	m := &Model{exists: o.Exists, sel: -1, boxW: 60}
	var current Agent
	for _, a := range o.Agents {
		m.items = append(m.items, item{kind: NewPane, agent: a, key: a.Key})
		if a.ID == o.Current {
			current = a
			if a.Installed {
				m.sel = len(m.items) - 1
			}
		}
	}
	if o.Current != "" {
		if current.ID == "" {
			current = Agent{ID: o.Current, Name: session.ShortName(o.Current)}
		}
		current.Installed = true // it runs
		m.items = append(m.items, item{kind: Here, agent: current, key: "h"})
	}
	m.items = append(m.items, item{kind: Template, key: "t"})
	if m.sel < 0 {
		m.sel = m.next(-1, 1)
	}
	if o.Template {
		m.direct = true
		m.choose(len(m.items) - 1)
	}
	return m
}

// next is the next item from i in direction d that can be chosen.
func (m *Model) next(i, d int) int {
	n := len(m.items)
	for k := 1; k <= n; k++ {
		if j := ((i+d*k)%n + n) % n; m.items[j].ok() {
			return j
		}
	}
	return i
}

func (m *Model) choose(i int) {
	if i < 0 || i >= len(m.items) || !m.items[i].ok() {
		return
	}
	m.sel, m.chosen, m.step, m.err = i, m.items[i], 1, ""
}

func (m *Model) field() *field {
	if m.chosen.kind == Template {
		return &m.name
	}
	return &m.desc
}

// Key applies a key. more reports typed text after it in the same read: an
// Enter in it is a newline of a paste the terminal did not bracket.
func (m *Model) Key(k string, more bool) (Result, bool) {
	if m.step == 0 {
		switch k {
		case "up", "backtab", "ctrl-p":
			m.sel = m.next(m.sel, -1)
		case "down", "tab", "ctrl-n":
			m.sel = m.next(m.sel, 1)
		case "enter", "right", " ":
			m.choose(m.sel)
		case "esc", "ctrl-c", "q":
			return Result{}, true
		default:
			for i, it := range m.items {
				if it.key == k {
					m.choose(i)
					break
				}
			}
		}
		return Result{}, false
	}
	f := m.field()
	m.err = ""
	switch k {
	case "enter":
		if more {
			m.Paste("\n")
			break
		}
		return m.submit()
	case "esc":
		if m.direct {
			return Result{}, true
		}
		m.step = 0
	case "ctrl-c":
		return Result{}, true
	case "backspace":
		if f.cur > 0 {
			f.r = append(f.r[:f.cur-1], f.r[f.cur:]...)
			f.cur--
		}
	case "delete":
		if f.cur < len(f.r) {
			f.r = append(f.r[:f.cur], f.r[f.cur+1:]...)
		}
	case "left":
		f.cur = max(f.cur-1, 0)
	case "right":
		f.cur = min(f.cur+1, len(f.r))
	case "home", "ctrl-a":
		for f.cur > 0 && f.r[f.cur-1] != '\n' {
			f.cur--
		}
	case "end", "ctrl-e":
		for f.cur < len(f.r) && f.r[f.cur] != '\n' {
			f.cur++
		}
	case "up":
		f.vertical(-1, m.boxW)
	case "down":
		f.vertical(1, m.boxW)
	case "ctrl-u":
		f.r, f.cur, f.top = nil, 0, 0
	case "ctrl-w":
		i := f.cur
		for i > 0 && unicode.IsSpace(f.r[i-1]) {
			i--
		}
		for i > 0 && !unicode.IsSpace(f.r[i-1]) {
			i--
		}
		f.r = append(f.r[:i], f.r[f.cur:]...)
		f.cur = i
	default:
		if utf8.RuneCountInString(k) == 1 {
			m.Paste(k)
		}
	}
	return Result{}, false
}

// Paste inserts text at the cursor: newlines kept in a description, spaces
// in a name; other control characters dropped.
func (m *Model) Paste(s string) {
	if m.step == 0 {
		return
	}
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var ins []rune
	for _, r := range s {
		switch {
		case r == '\n' && m.chosen.kind != Template:
			ins = append(ins, r)
		case r == '\n' || r == '\t':
			ins = append(ins, ' ')
		case unicode.IsPrint(r):
			ins = append(ins, r)
		}
	}
	f := m.field()
	f.r = append(f.r[:f.cur], append(ins, f.r[f.cur:]...)...)
	f.cur += len(ins)
	m.err = ""
}

// submit ends the dialog with the text, or tells what is wrong with a name.
func (m *Model) submit() (Result, bool) {
	text := strings.TrimSpace(string(m.field().r))
	if m.chosen.kind != Template {
		return Result{Kind: m.chosen.kind, Agent: m.chosen.agent.ID, Text: text}, true
	}
	name := skills.Slug(text)
	switch {
	case name == "":
		m.err = i18n.T("skills.dialog.name_empty")
	case m.exists != nil && m.exists(name):
		m.err = i18n.T("skills.exists", name)
	default:
		return Result{Kind: Template, Text: name}, true
	}
	return Result{}, false
}

// Click chooses the item on row y (1-based) of the first step.
func (m *Model) Click(_, y int) (Result, bool) {
	if m.step == 0 && y >= 1 && y <= len(m.rows) {
		m.choose(m.rows[y-1])
	}
	return Result{}, false
}

// span is a drawn line of a text: runes [start, end) (a newline ending it
// belongs to no line).
type span struct{ start, end int }

func width(r rune) int { return sidebar.DisplayWidth(string(r)) }

// layout wraps r to w columns, at the last space when a line has one.
func layout(r []rune, w int) []span {
	w = max(w, 1)
	var out []span
	start := 0
	for {
		col, i, brk := 0, start, -1
		for i < len(r) && r[i] != '\n' {
			if col+width(r[i]) > w {
				break
			}
			if r[i] == ' ' {
				brk = i + 1
			}
			col += width(r[i])
			i++
		}
		switch {
		case i >= len(r):
			return append(out, span{start, i})
		case r[i] == '\n':
			out = append(out, span{start, i})
			start = i + 1
		default:
			if brk > start {
				i = brk
			}
			if i == start {
				i++ // a character wider than the box
			}
			out = append(out, span{start, i})
			start = i
		}
	}
}

// lines wraps f for a box w columns wide; the last column stays free for the
// cursor after a full line.
func (f *field) lines(w int) []span { return layout(f.r, w-1) }

// locate is the line and column of rune index cur.
func locate(lines []span, r []rune, cur int) (int, int) {
	l := 0
	for i, s := range lines {
		if s.start <= cur {
			l = i
		}
	}
	return l, sidebar.DisplayWidth(string(r[lines[l].start:cur]))
}

// vertical moves the cursor d lines in a box w columns wide, keeping its column.
func (f *field) vertical(d, w int) {
	lines := f.lines(w)
	l, col := locate(lines, f.r, f.cur)
	t := l + d
	switch {
	case t < 0:
		f.cur = 0
		return
	case t >= len(lines):
		f.cur = len(f.r)
		return
	}
	s, used, i := lines[t], 0, lines[t].start
	for i < s.end && used+width(f.r[i]) <= col {
		used += width(f.r[i])
		i++
	}
	if i == s.end && i > s.start && t+1 < len(lines) && lines[t+1].start == s.end {
		i-- // the end of a wrapped line is the next line's start
	}
	f.cur = i
}

// box draws f as rows lines of w columns, the cursor shown; an empty field
// shows the placeholder.
func (f *field) box(w, rows int, placeholder string) []string {
	lines := f.lines(w)
	l, _ := locate(lines, f.r, f.cur)
	if l < f.top {
		f.top = l
	}
	if l >= f.top+rows {
		f.top = l - rows + 1
	}
	out := make([]string, rows)
	for i := range out {
		li := f.top + i
		if li >= len(lines) {
			out[i] = strings.Repeat(" ", w)
			continue
		}
		s := lines[li]
		var b strings.Builder
		used := 0
		for j := s.start; j < s.end; j++ {
			if j == f.cur {
				b.WriteString(cursorOn + string(f.r[j]) + cursorNo)
			} else {
				b.WriteRune(f.r[j])
			}
			used += width(f.r[j])
		}
		if li == l && f.cur >= s.end {
			b.WriteString(cursorOn + " " + cursorNo)
			used++
			if len(f.r) == 0 && placeholder != "" {
				p := sidebar.Fit(placeholder, max(w-used, 0))
				b.WriteString(faint + p + reset)
				used += sidebar.DisplayWidth(p)
			}
		}
		out[i] = b.String() + strings.Repeat(" ", max(w-used, 0))
	}
	return out
}

// label is an item's name.
func (it item) label() string {
	switch it.kind {
	case Here:
		return i18n.T("skills.dialog.here", it.agent.Name)
	case Template:
		return i18n.T("skills.dialog.template")
	}
	return it.agent.Name
}

// glyph is an item's colored icon.
func (it item) glyph() string {
	if it.kind == Template {
		return cyan + "✎" + reset
	}
	sym, color := sidebar.AgentIcon(it.agent.ID)
	return color + sym + reset
}

// row draws item i of the first step, w columns wide, its name in nameW.
func (m *Model) row(i, w, nameW int) string {
	it := m.items[i]
	var note string
	switch {
	case it.kind == NewPane && !it.agent.Installed:
		note = muted + i18n.T("menu.not_installed") + reset
	case it.kind == NewPane && it.agent.Limited != "":
		note = yellow + i18n.T("sidebar.limited", it.agent.Limited) + reset
	case it.kind == Here:
		note = muted + i18n.T("skills.dialog.here_note") + reset
	case it.kind == Template:
		note = muted + i18n.T("skills.dialog.template_note") + reset
	}
	name := sidebar.Fit(it.label(), nameW)
	text := "  " + cyan + it.key + reset + "  " + it.glyph() + " " + bold + name + reset + "  " + note
	if !it.ok() {
		text = "  " + muted + it.key + "  " + sidebar.Plain(it.glyph()) + " " + name + reset + "  " + note
	}
	if i != m.sel {
		return " " + text
	}
	text = cyan + "▸" + reset + text
	pad := max(w-sidebar.DisplayWidth(sidebar.Plain(text)), 0)
	return selBg + strings.ReplaceAll(text, reset, reset+selBg) + strings.Repeat(" ", pad) + reset
}

// Render draws the dialog for w columns and h rows.
func (m *Model) Render(w, h int) []string {
	w, h = max(w, 20), max(h, 8)
	title := "✨ " + i18n.T("skills.dialog.title")
	if m.step == 1 {
		title += " · " + sidebar.Plain(m.chosen.glyph()) + " " + m.chosen.label()
		if m.chosen.kind == NewPane {
			title += " · " + i18n.T("skills.dialog.new_pane")
		}
	}
	out := []string{
		" " + bold + sidebar.Fit(title, w-2) + reset,
		faint + strings.Repeat("─", w) + reset,
	}
	var hint string
	m.rows = nil
	switch {
	case m.step == 0:
		out = append(out,
			" "+bold+sidebar.Fit(i18n.T("skills.dialog.who"), w-2)+reset,
			" "+muted+sidebar.Fit(i18n.T("skills.dialog.who_note"), w-2)+reset,
			"")
		nameW := 0
		for _, it := range m.items {
			nameW = max(nameW, sidebar.DisplayWidth(it.label()))
		}
		m.rows = []int{-1, -1, -1, -1, -1}
		for i, it := range m.items {
			if i > 0 && it.kind != NewPane && m.items[i-1].kind == NewPane {
				out = append(out, "   "+faint+strings.Repeat("─", max(min(nameW+8, w-6), 1))+reset)
				m.rows = append(m.rows, -1)
			}
			out = append(out, m.row(i, w, nameW))
			m.rows = append(m.rows, i)
		}
		hint = i18n.T("skills.dialog.choose_hint")
	case m.chosen.kind == Template:
		bw := max(w-5, 4)
		m.boxW = bw
		out = append(out, "", " "+bold+i18n.T("skills.dialog.name")+reset)
		out = append(out, boxed(m.name.box(bw, 1, i18n.T("skills.dialog.name_placeholder")), bw)...)
		name := skills.Slug(string(m.name.r))
		switch {
		case m.err != "":
			out = append(out, " "+yellow+"⚠ "+sidebar.Fit(m.err, w-4)+reset)
		case name != "" && m.exists != nil && m.exists(name):
			out = append(out, " "+yellow+"⚠ "+sidebar.Fit(i18n.T("skills.exists", name), w-4)+reset)
		case name != "":
			out = append(out, " "+muted+"→ "+sidebar.Fit(skills.Home+"/"+name+"/"+skills.File, w-4)+reset)
		default:
			out = append(out, "")
		}
		out = append(out, " "+faint+sidebar.Fit(i18n.T("skills.dialog.name_rule"), w-2)+reset)
		out = append(out, "", " "+muted+sidebar.Fit(i18n.T("skills.dialog.template_info"), w-2)+reset)
		hint = i18n.T("skills.dialog.name_hint")
		if m.direct {
			hint = i18n.T("skills.dialog.name_hint_close")
		}
	default:
		bw := max(w-5, 4)
		m.boxW = bw
		// The title and its rule, the question, the box's borders, the note,
		// a blank line and the hint take 8 rows; the guide shows when the
		// box keeps 3.
		rows := h - 8
		guide := []string{
			" " + muted + sidebar.Fit(i18n.T("skills.dialog.guide", m.chosen.agent.Name), w-2) + reset,
			"   " + faint + "•" + reset + " " + muted + sidebar.Fit(i18n.T("skills.dialog.guide_1"), w-6) + reset,
			"   " + faint + "•" + reset + " " + muted + sidebar.Fit(i18n.T("skills.dialog.guide_2"), w-6) + reset,
			"   " + faint + "•" + reset + " " + muted + sidebar.Fit(i18n.T("skills.dialog.guide_3"), w-6) + reset,
			"",
		}
		if rows-len(guide) >= 3 {
			out = append(out, guide...)
			rows -= len(guide)
		}
		rows = min(max(rows, 1), maxRows)
		out = append(out, " "+bold+i18n.T("skills.dialog.ask")+reset)
		out = append(out, boxed(m.desc.box(bw, rows, i18n.T("skills.dialog.ask_placeholder")), bw)...)
		out = append(out, " "+muted+sidebar.Fit(i18n.T("skills.dialog.ask_empty", m.chosen.agent.Name), w-2)+reset)
		hint = i18n.T("skills.dialog.write_hint")
	}
	for len(out) < h-1 {
		out = append(out, "")
	}
	return append(out[:h-1], " "+faint+sidebar.Fit(hint, w-2)+reset)
}

// boxed frames lines of text bw columns wide.
func boxed(lines []string, bw int) []string {
	out := []string{" " + faint + "╭" + strings.Repeat("─", bw+2) + "╮" + reset}
	for _, l := range lines {
		out = append(out, " "+faint+"│"+reset+" "+l+" "+faint+"│"+reset)
	}
	return append(out, " "+faint+"╰"+strings.Repeat("─", bw+2)+"╯"+reset)
}

// typed reports printable keys among evs.
func typed(evs []sidebar.Event) bool {
	for _, e := range evs {
		if e.Kind == sidebar.EvKey && utf8.RuneCountInString(e.Key) == 1 {
			return true
		}
	}
	return false
}

// Run shows the dialog on the terminal until it ends.
func Run(m *Model, in, out *os.File) (Result, error) {
	fd := int(in.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return Result{}, err
	}
	defer restore()
	// Alternate screen, no cursor (the box draws its own), clicks, and
	// pastes in one piece.
	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h\x1b[?2004h")
	defer func() { _, _ = io.WriteString(out, "\x1b[?2004l\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	draw := func() {
		w, h := sidebar.Size(int(out.Fd()))
		var sc sidebar.Screen
		for _, l := range m.Render(w, h) {
			sc.Lines = append(sc.Lines, sidebar.Line{Text: l})
		}
		_, _ = io.WriteString(out, sidebar.Frame(sc, w, h))
	}
	var pending []byte
	draw()
	for {
		b, err := sidebar.ReadInput(fd, 50*time.Millisecond)
		if err != nil {
			return Result{}, nil // the popup went away
		}
		select {
		case <-winch:
			draw()
		default:
		}
		if b == nil {
			continue
		}
		var evs []sidebar.Event
		evs, pending = sidebar.Parse(append(pending, b...), true)
		for i, ev := range evs {
			var res Result
			done := false
			switch ev.Kind {
			case sidebar.EvKey:
				res, done = m.Key(ev.Key, typed(evs[i+1:]))
			case sidebar.EvPaste:
				m.Paste(ev.Text)
			case sidebar.EvClick:
				if !sidebar.EarlyClick() {
					res, done = m.Click(ev.X, ev.Y)
				}
			}
			if done {
				return res, nil
			}
		}
		draw()
	}
}
