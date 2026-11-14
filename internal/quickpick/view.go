package quickpick

import (
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gotofile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

// Colors from the theme (D68), set again when it changes.
var faint, sepFg, match, selBg string

func init() {
	theme.Watch(func(p theme.Palette) {
		s := theme.StylesOf(p)
		faint = s.Faint
		sepFg = s.Sep
		match = s.Match
		selBg = s.Selected
	})
}

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	xMark = "✕"
	head  = 3 // title, input and line above the list
)

// row is one row of the list: a group's name, or an item.
type row struct {
	group string
	item  int   // index in Pick.Items; -1 on a group row
	pos   []int // the typed letters' places in the label
}

// Model is the Quick Pick's state: what is typed, listed and selected.
type Model struct {
	p     *Pick
	query []rune
	rows  []row
	sel   int // the selected row; -1 for none
	top   int // the first row shown
	w, h  int
}

// New shows p with nothing typed and its first item selected.
func New(p *Pick) *Model {
	m := &Model{p: p}
	m.update()
	return m
}

// Query is what is typed.
func (m *Model) Query() string { return string(m.query) }

// Selected is the selected item's index in the Pick (-1 for none).
func (m *Model) Selected() int {
	if m.sel < 0 || m.sel >= len(m.rows) {
		return -1
	}
	return m.rows[m.sel].item
}

// update lists the items for the query: all of them in their groups, or the
// ones it matches, best first, then the Always ones it does not match (as
// VS Code: Enter takes the best match).
func (m *Model) update() {
	m.rows, m.top = nil, 0
	q := gotofile.Parse(string(m.query))
	if q.Empty() {
		group := ""
		for i, it := range m.p.Items {
			if it.Group != group && it.Group != "" {
				m.rows = append(m.rows, row{group: it.Group, item: -1})
			}
			group = it.Group
			m.rows = append(m.rows, row{item: i})
		}
	} else {
		type hit struct {
			row
			score int
		}
		var hits []hit
		var always []row
		for i, it := range m.p.Items {
			if score, pos, ok := gotofile.Match(it.Label, q); ok {
				hits = append(hits, hit{row{item: i, pos: pos}, score})
			} else if it.Always {
				always = append(always, row{item: i})
			}
		}
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
		for _, h := range hits {
			m.rows = append(m.rows, h.row)
		}
		m.rows = append(m.rows, always...)
	}
	m.sel = -1
	m.move(1)
}

// list is how many rows fit under the input.
func (m *Model) list() int { return max(m.h-head, 1) }

func (m *Model) keepVisible() {
	if m.sel < 0 {
		return
	}
	if m.sel < m.top {
		m.top = m.sel
		if m.top > 0 && m.rows[m.top-1].item < 0 {
			m.top-- // the selected item's group row too
		}
	}
	if m.sel >= m.top+m.list() {
		m.top = m.sel - m.list() + 1
	}
}

// move steps the selection by d items (group rows are not chosen).
func (m *Model) move(d int) {
	i := m.sel
	if i < 0 && d < 0 {
		i = len(m.rows)
	}
	for range len(m.rows) {
		i += d
		if i < 0 || i >= len(m.rows) {
			return
		}
		if m.rows[i].item >= 0 {
			m.sel = i
			m.keepVisible()
			return
		}
	}
}

// Render draws the Quick Pick on a w×h screen.
func (m *Model) Render(w, h int) []string {
	m.w, m.h = w, h
	m.keepVisible()
	lines := []string{bold + " " + sidebar.Fit(m.p.Title, max(w-3, 1)) + reset + faint + xMark + reset + " "}
	input := " › " + string(m.query)
	if len(m.query) == 0 {
		lines = append(lines, input+faint+sidebar.Fit(m.p.Placeholder, max(w-3, 1))+reset)
	} else {
		lines = append(lines, sidebar.Fit(input+"▏", w))
	}
	lines = append(lines, " "+sepFg+strings.Repeat("─", max(w-2, 1))+reset+" ")
	if len(m.rows) == 0 {
		return append(lines, faint+"   "+sidebar.Fit(m.p.Empty, max(w-3, 1))+reset)
	}
	for i := m.top; i < len(m.rows) && len(lines) < h; i++ {
		lines = append(lines, m.draw(m.rows[i], i == m.sel, w))
	}
	return lines
}

// draw is one row of the list w wide.
func (m *Model) draw(r row, selected bool, w int) string {
	if r.item < 0 {
		name := " " + r.group + " "
		return " " + sepFg + "──" + reset + faint + name + reset + sepFg + strings.Repeat("─", max(w-4-sidebar.DisplayWidth(name), 0)) + reset + " "
	}
	it := m.p.Items[r.item]
	lw := min(sidebar.DisplayWidth(it.Label), max(w-4, 1))
	label := sidebar.Fit(it.Label, lw)
	desc := ""
	if it.Description != "" && w-4-lw > 4 {
		desc = "  " + sidebar.Fit(it.Description, w-4-lw-2)
	}
	pad := strings.Repeat(" ", max(w-3-lw-sidebar.DisplayWidth(desc), 0))
	if selected {
		return selBg + "   " + label + desc + pad + reset
	}
	return "   " + paint(label, r.pos) + faint + desc + reset + pad
}

// paint shows the typed letters of label (their rune places) brighter.
func paint(label string, pos []int) string {
	if len(pos) == 0 {
		return label
	}
	at := map[int]bool{}
	for _, p := range pos {
		at[p] = true
	}
	var b strings.Builder
	for i, r := range []rune(label) {
		if at[i] {
			b.WriteString(match + string(r) + reset)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Result is what an event did: Chosen is the item chosen (-1 for none),
// Done that the Quick Pick closes.
type Result struct {
	Chosen int
	Done   bool
}

var none = Result{Chosen: -1}

func (m *Model) choose() Result {
	if i := m.Selected(); i >= 0 {
		return Result{Chosen: i, Done: true}
	}
	return none
}

// Key applies a key (sidebar.Parse's names): letters type, the arrows and
// pages move, Enter chooses, Esc closes.
func (m *Model) Key(k string) Result {
	switch k {
	case "enter":
		return m.choose()
	case "esc", "ctrl-c":
		return Result{Chosen: -1, Done: true}
	case "up", "ctrl-p", "backtab":
		m.move(-1)
	case "down", "ctrl-n", "tab":
		m.move(1)
	case "pgup":
		for range m.list() {
			m.move(-1)
		}
	case "pgdown":
		for range m.list() {
			m.move(1)
		}
	case "backspace":
		if n := len(m.query); n > 0 {
			m.query = m.query[:n-1]
			m.update()
		}
	case "ctrl-u":
		m.query = nil
		m.update()
	case "ctrl-w":
		q := strings.TrimRight(string(m.query), " ")
		m.query = []rune(q[:strings.LastIndex(q, " ")+1])
		m.update()
	default:
		if r := []rune(k); len(r) == 1 && unicode.IsPrint(r[0]) {
			m.Type(k)
		}
	}
	return none
}

// Type adds text to the input (a key, or a paste without its line breaks).
func (m *Model) Type(s string) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
	if s != "" {
		m.query = append(m.query, []rune(s)...)
		m.update()
	}
}

// at is the row on screen row y (1-based), -1 for none.
func (m *Model) at(y int) int {
	i := m.top + y - 1 - head
	if y <= head || i >= len(m.rows) {
		return -1
	}
	return i
}

// Click chooses the item at x, y (1-based) or closes on ✕.
func (m *Model) Click(x, y int) Result {
	if y == 1 && x >= m.w-2 {
		return Result{Chosen: -1, Done: true}
	}
	if i := m.at(y); i >= 0 && m.rows[i].item >= 0 {
		m.sel = i
		return m.choose()
	}
	return none
}

// Move selects the item under the pointer.
func (m *Model) Move(_, y int) {
	if i := m.at(y); i >= 0 && m.rows[i].item >= 0 {
		m.sel = i
	}
}

// Scroll moves the list by three rows.
func (m *Model) Scroll(up bool) {
	if up {
		m.top = max(m.top-3, 0)
	} else {
		m.top = max(min(m.top+3, len(m.rows)-m.list()), 0)
	}
	if m.sel >= 0 && (m.sel < m.top || m.sel >= m.top+m.list()) {
		m.sel = -1
		for i := m.top; i < min(m.top+m.list(), len(m.rows)); i++ {
			if m.rows[i].item >= 0 {
				m.sel = i
				break
			}
		}
	}
}

// Run shows p on the terminal (in, out) until an item is chosen: its index
// (-1 when closed) and what was typed.
func Run(p *Pick, in, out *os.File) (int, string, error) {
	fd := int(in.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return -1, "", err
	}
	defer restore()
	// No cursor; presses, the pointer's motion, the wheel, pastes.
	_, _ = io.WriteString(out, "\x1b[?25l\x1b[?1000h\x1b[?1003h\x1b[?1006h\x1b[?2004h")
	defer func() { _, _ = io.WriteString(out, "\x1b[?2004l\x1b[?1003l\x1b[?1000l\x1b[?1006l\x1b[?25h") }()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	m := New(p)
	last := ""
	draw := func() {
		w, h := sidebar.Size(int(out.Fd()))
		var sc sidebar.Screen
		for _, l := range m.Render(w, h) {
			sc.Lines = append(sc.Lines, sidebar.Line{Text: l})
		}
		if f := sidebar.Frame(sc, w, h); f != last {
			_, _ = io.WriteString(out, f)
			last = f
		}
	}
	draw()
	var pending []byte
	for {
		b, err := sidebar.ReadInput(fd, 50*time.Millisecond)
		if err != nil {
			return -1, "", nil // the popup went away
		}
		select {
		case <-winch:
			last = ""
		default:
		}
		var evs []sidebar.Event
		evs, pending = sidebar.Parse(append(pending, b...), true)
		for _, ev := range evs {
			r := none
			switch ev.Kind {
			case sidebar.EvKey:
				r = m.Key(ev.Key)
			case sidebar.EvPaste:
				m.Type(ev.Text)
			case sidebar.EvClick:
				if !sidebar.EarlyClick() {
					r = m.Click(ev.X, ev.Y)
				}
			case sidebar.EvMove:
				m.Move(ev.X, ev.Y)
			case sidebar.EvScroll:
				m.Scroll(ev.Key == "up")
			}
			if r.Done {
				return r.Chosen, m.Query(), nil
			}
		}
		draw()
	}
}
