// Package installview is the popup shown for an agent that is not installed
// (§9.3 "Not installed"): what to run, and a button that runs it.
package installview

import (
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	cyan  = "\x1b[36m"
	muted = "\x1b[38;5;247m"
	faint = "\x1b[38;5;240m"
	selBg = "\x1b[48;5;24m"
)

// Result is what the user chose.
type Result int

// Results.
const (
	Close Result = iota
	Install
)

// Choice is a button of a popup with its own: its label, the letter that
// presses it ("" for none) and what it means.
type Choice struct {
	Label, Key string
	R          Result
}

// Model is the popup's state.
type Model struct {
	Icon    string // the agent's icon, drawn
	Name    string // "Codex"
	Cmd     string // the command that installs it
	Missing string // a program the command needs that is not here ("npm"), or ""

	// Title, Text, Choices and Hint make another popup (the one that
	// offers micro, D45): its title, its paragraphs (one starting with "\t"
	// is dimmed), its buttons (the first is the default) and the keys line.
	Title   string
	Text    []string
	Choices []Choice
	Hint    string

	focus int     // the button Enter presses: 0 the first
	btnY  int     // the buttons' row (1-based)
	btnX  [][]int // each button's columns: [from, to) (1-based)
}

// buttons are the popup's choices, the default first.
func (m *Model) buttons() []Result {
	if len(m.Choices) > 0 {
		rs := make([]Result, len(m.Choices))
		for i, c := range m.Choices {
			rs[i] = c.R
		}
		return rs
	}
	if m.Missing != "" {
		return []Result{Close}
	}
	return []Result{Install, Close}
}

func (m *Model) label(r Result) string {
	for _, c := range m.Choices {
		if c.R == r {
			return c.Label
		}
	}
	if r == Install {
		return i18n.T("install.now")
	}
	return i18n.T("install.close")
}

// Render draws the popup for w columns and h rows.
func (m *Model) Render(w, h int) []string {
	w, h = max(w, 24), max(h, 8)
	title := i18n.T("install.title", m.Name)
	if m.Title != "" {
		title = m.Title
	}
	out := []string{
		" " + m.Icon + " " + bold + sidebar.Fit(title, w-4) + reset,
		faint + strings.Repeat("─", w) + reset,
	}
	text := func(s, style string) {
		for _, l := range wrap(s, w-2) {
			out = append(out, " "+style+l+reset)
		}
	}
	if m.Title != "" {
		for _, p := range m.Text {
			if t, ok := strings.CutPrefix(p, "\t"); ok {
				text(t, muted)
			} else {
				text(p, "")
			}
			out = append(out, "")
		}
	} else if m.Missing != "" {
		text(i18n.T("install.needs", m.Name, m.Missing), "")
		text(howTo(m.Missing), muted)
		out = append(out, "", " "+muted+i18n.T("install.then")+reset)
	} else {
		text(i18n.T("install.why", m.Name), "")
		out = append(out, "")
	}
	if m.Cmd != "" {
		out = append(out, "   "+cyan+"$ "+sidebar.Fit(m.Cmd, w-6)+reset, "")
	}
	row, x := "   ", 4
	m.btnX = nil
	for i, b := range m.buttons() {
		t := " " + m.label(b) + " "
		if i == m.focus {
			t = selBg + bold + t + reset
		} else {
			t = faint + "[" + reset + t + faint + "]" + reset
		}
		bw := sidebar.DisplayWidth(sidebar.Plain(t))
		m.btnX = append(m.btnX, []int{x, x + bw})
		row += t + "   "
		x += bw + 3
	}
	out = append(out, row)
	m.btnY = len(out)
	for len(out) < h-1 {
		out = append(out, "")
	}
	hint := i18n.T("install.hint")
	switch {
	case m.Hint != "":
		hint = m.Hint
	case m.Missing != "":
		hint = i18n.T("install.hint_close")
	}
	return append(out[:h-1], " "+faint+sidebar.Fit(hint, w-2)+reset)
}

// howTo says how to get a program an install command needs.
func howTo(tool string) string {
	switch tool {
	case "npm":
		return i18n.T("install.needs_npm")
	case "curl":
		return i18n.T("install.needs_curl")
	case "brew":
		return i18n.T("install.needs_brew")
	}
	return ""
}

// wrap breaks s into lines of at most w columns, at spaces.
func wrap(s string, w int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case sidebar.DisplayWidth(line+" "+word) <= w:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	return append(out, line)
}

// Key handles a key; done reports a choice.
func (m *Model) Key(k string) (Result, bool) {
	bs := m.buttons()
	for _, c := range m.Choices {
		if c.Key != "" && c.Key == k {
			return c.R, true
		}
	}
	switch k {
	case "enter":
		return bs[m.focus], true
	case "esc", "q", "ctrl-c":
		return Close, true
	case "i":
		if m.Missing == "" && len(m.Choices) == 0 {
			return Install, true
		}
	case "tab", "right", "l":
		m.focus = (m.focus + 1) % len(bs)
	case "backtab", "left", "h":
		m.focus = (m.focus + len(bs) - 1) % len(bs)
	}
	return Close, false
}

// Click handles a click at (x, y) (1-based).
func (m *Model) Click(x, y int) (Result, bool) {
	if y != m.btnY {
		return Close, false
	}
	for i, r := range m.btnX {
		if x >= r[0] && x < r[1] {
			return m.buttons()[i], true
		}
	}
	return Close, false
}

// Run shows the popup on the terminal until the user chooses.
func Run(m *Model, in, out *os.File) (Result, error) {
	fd := int(in.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return Close, err
	}
	defer restore()
	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(out, "\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()
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
			return Close, nil // the popup went away
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
			var r Result
			done := false
			switch ev.Kind {
			case sidebar.EvKey:
				r, done = m.Key(ev.Key)
			case sidebar.EvClick:
				if !sidebar.EarlyClick() {
					r, done = m.Click(ev.X, ev.Y)
				}
			}
			if done {
				return r, nil
			}
		}
		draw()
	}
}
