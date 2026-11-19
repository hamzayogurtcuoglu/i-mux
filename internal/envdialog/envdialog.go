// Package envdialog is the New environment dialog (D84, T89.7): a popup in
// the middle of the client that asks a new environment's name, shows the
// name what is typed makes (envfile.Slug) and warns when one has it. It
// only asks: `imux env new` writes the file and opens it in the editor.
package envdialog

import (
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/envfile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
)

// Colors from the theme (D68), set again when it changes.
var yellow, muted, faint string

func init() {
	theme.Watch(func(p theme.Palette) {
		s := theme.StylesOf(p)
		yellow, muted, faint = s.Yellow, s.Muted, s.Faint
	})
}

const (
	reset    = "\x1b[0m"
	bold     = "\x1b[1m"
	cursorOn = "\x1b[7m" // the text cursor: reverse video
	cursorNo = "\x1b[27m"
)

// Model is the dialog's state: what is typed and the cursor (in runes).
type Model struct {
	r      []rune
	cur    int
	exists func(name string) bool
	err    string
}

// New makes the dialog; exists reports a name an environment has.
func New(exists func(name string) bool) *Model { return &Model{exists: exists} }

// Name is the name what is typed makes.
func (m *Model) Name() string { return envfile.Slug(string(m.r)) }

func (m *Model) taken(name string) bool { return m.exists != nil && m.exists(name) }

// Key applies a key (sidebar.Parse's names). done ends the dialog, with
// the name chosen, or "" when it was closed.
func (m *Model) Key(k string) (name string, done bool) {
	m.err = ""
	switch k {
	case "enter":
		return m.submit()
	case "esc", "ctrl-c":
		return "", true
	case "backspace":
		if m.cur > 0 {
			m.r = append(m.r[:m.cur-1], m.r[m.cur:]...)
			m.cur--
		}
	case "delete":
		if m.cur < len(m.r) {
			m.r = append(m.r[:m.cur], m.r[m.cur+1:]...)
		}
	case "left":
		m.cur = max(m.cur-1, 0)
	case "right":
		m.cur = min(m.cur+1, len(m.r))
	case "home", "ctrl-a", "up":
		m.cur = 0
	case "end", "ctrl-e", "down":
		m.cur = len(m.r)
	case "ctrl-u":
		m.r, m.cur = nil, 0
	case "ctrl-w":
		i := m.cur
		for i > 0 && unicode.IsSpace(m.r[i-1]) {
			i--
		}
		for i > 0 && !unicode.IsSpace(m.r[i-1]) {
			i--
		}
		m.r = append(m.r[:i], m.r[m.cur:]...)
		m.cur = i
	default:
		if utf8.RuneCountInString(k) == 1 {
			m.Paste(k)
		}
	}
	return "", false
}

// Paste inserts text at the cursor: line breaks and tabs as spaces, other
// control characters dropped.
func (m *Model) Paste(s string) {
	var ins []rune
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			ins = append(ins, ' ')
		case unicode.IsPrint(r):
			ins = append(ins, r)
		}
	}
	m.r = append(m.r[:m.cur], append(ins, m.r[m.cur:]...)...)
	m.cur += len(ins)
	m.err = ""
}

// submit ends the dialog with the name, or says what is wrong with it.
func (m *Model) submit() (string, bool) {
	name := m.Name()
	switch {
	case name == "":
		m.err = i18n.T("env.new.empty")
	case m.taken(name):
		m.err = i18n.T("env.new.exists", name)
	default:
		return name, true
	}
	return "", false
}

// Render draws the dialog for w columns and h rows.
func (m *Model) Render(w, h int) []string {
	w, h = max(w, 20), max(h, 8)
	bw := w - 5
	out := []string{
		" " + bold + sidebar.Fit("🔑 "+i18n.T("env.new.title"), w-2) + reset,
		faint + strings.Repeat("─", w) + reset,
		"",
		" " + bold + sidebar.Fit(i18n.T("env.new.name"), w-2) + reset,
	}
	out = append(out, boxed(m.field(bw), bw)...)
	name := m.Name()
	switch {
	case m.err != "":
		out = append(out, " "+yellow+"⚠ "+sidebar.Fit(m.err, w-4)+reset)
	case name != "" && m.taken(name):
		out = append(out, " "+yellow+"⚠ "+sidebar.Fit(i18n.T("env.new.exists", name), w-4)+reset)
	case name != "":
		out = append(out, " "+muted+"→ "+sidebar.Fit(paths.Shorten(envfile.Path(name)), w-4)+reset)
	default:
		out = append(out, "")
	}
	out = append(out, wrapped(i18n.T("env.new.rule"), faint, w)...)
	out = append(out, "")
	out = append(out, wrapped(i18n.T("env.new.info"), muted, w)...)
	// What an environment can hold, when there is room: any program's
	// variables, Claude Code's endpoints being one use.
	uses := []string{"", " " + sidebar.Fit(i18n.T("env.new.uses"), w-2)}
	for _, k := range []string{"env.new.use_1", "env.new.use_2", "env.new.use_3"} {
		uses = append(uses, "   "+faint+"•"+reset+" "+muted+sidebar.Fit(i18n.T(k), w-6)+reset)
	}
	if len(out)+len(uses) <= h-1 {
		out = append(out, uses...)
	}
	for len(out) < h-1 {
		out = append(out, "")
	}
	return append(out[:h-1], " "+faint+sidebar.Fit(i18n.T("env.new.hint"), w-2)+reset)
}

// wrapped is text broken into lines between words, w columns, in color.
func wrapped(text, color string, w int) []string {
	var out []string
	for _, l := range sidebar.WrapWords(text, w-2) {
		out = append(out, " "+color+sidebar.Fit(l, w-2)+reset)
	}
	return out
}

// field is the name box's text, bw columns: the part around the cursor,
// the cursor shown; empty, the placeholder after the cursor.
func (m *Model) field(bw int) string {
	if len(m.r) == 0 {
		return cursorOn + " " + cursorNo + faint + sidebar.Fit(i18n.T("env.new.placeholder"), bw-1) + reset
	}
	start := 0 // the first rune shown: the cursor stays in the box
	for start < m.cur && sidebar.DisplayWidth(string(m.r[start:m.cur]))+1 > bw {
		start++
	}
	var b strings.Builder
	used := 0
	for i := start; i < len(m.r); i++ {
		cw := sidebar.DisplayWidth(string(m.r[i]))
		if used+cw > bw {
			break
		}
		if i == m.cur {
			b.WriteString(cursorOn + string(m.r[i]) + cursorNo)
		} else {
			b.WriteRune(m.r[i])
		}
		used += cw
	}
	if m.cur == len(m.r) && used < bw {
		b.WriteString(cursorOn + " " + cursorNo)
		used++
	}
	return b.String() + strings.Repeat(" ", max(bw-used, 0))
}

// boxed frames a line of text bw columns wide.
func boxed(line string, bw int) []string {
	return []string{
		" " + faint + "╭" + strings.Repeat("─", bw+2) + "╮" + reset,
		" " + faint + "│" + reset + " " + line + " " + faint + "│" + reset,
		" " + faint + "╰" + strings.Repeat("─", bw+2) + "╯" + reset,
	}
}

// Run shows the dialog on the terminal (in, out) until it ends: the name
// chosen, or "" when it was closed.
func Run(m *Model, in, out *os.File) (string, error) {
	fd := int(in.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return "", err
	}
	defer restore()
	// Alternate screen, no cursor (the box draws its own), pastes in one
	// piece.
	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[?25l\x1b[?2004h")
	defer func() { _, _ = io.WriteString(out, "\x1b[?2004l\x1b[?25h\x1b[?1049l") }()
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
			return "", nil // the popup went away
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
		for _, ev := range evs {
			switch ev.Kind {
			case sidebar.EvKey:
				if name, done := m.Key(ev.Key); done {
					return name, nil
				}
			case sidebar.EvPaste:
				m.Paste(ev.Text)
			}
		}
		draw()
	}
}
