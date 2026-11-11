// Package historyview is History's search popup (§9.3 "Search in History"):
// the project's conversations, and as you type the ones whose text has it,
// with the messages that do.
package historyview

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/histsearch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
)

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	muted   = "\x1b[38;5;247m"
	faint   = "\x1b[38;5;243m"
	text    = "\x1b[38;5;250m"
	pointer = "\x1b[36m"
	selBg   = "\x1b[48;5;236m"
	hit     = "\x1b[48;5;94m\x1b[38;5;230m"
	pill    = "\x1b[48;5;238m\x1b[38;5;252m"
)

// Conv is one conversation of the list.
type Conv struct {
	Ref    session.Ref
	Title  string // its name, else its first request
	Pinned bool
}

// Row is a line of the list: a conversation, or one of its messages.
type Row struct {
	Conv int              // index into Convs
	Snip *histsearch.Snip // nil for the conversation's row
}

// Model is the popup's state.
type Model struct {
	Title  string
	Convs  []Conv // pinned first, then the newest first
	Query  string
	Hits   map[string]histsearch.Hit // by conversation key, for Query
	Busy   bool                      // a search runs
	Looked int                       // conversations looked at so far
	Sel    int
	Top    int
	Folded map[string]bool
	Now    time.Time

	w, list int
}

// matches reports whether conversation c is in the results: its text, or
// its title, has the query.
func (m *Model) matches(c Conv) (histsearch.Hit, bool) {
	h, ok := m.Hits[c.Ref.Key()]
	if ok {
		return h, true
	}
	q := strings.ToLower(m.Query)
	return histsearch.Hit{Ref: c.Ref}, strings.Contains(strings.ToLower(c.Title), q)
}

// Rows lists the conversations (with a query: those that have it, each
// with its messages unless folded).
func (m *Model) Rows() []Row {
	var out []Row
	for i, c := range m.Convs {
		if m.Query == "" {
			out = append(out, Row{Conv: i})
			continue
		}
		h, ok := m.matches(c)
		if !ok {
			continue
		}
		out = append(out, Row{Conv: i})
		if !m.Folded[c.Ref.Key()] {
			for k := range h.Snips {
				out = append(out, Row{Conv: i, Snip: &h.Snips[k]})
			}
		}
	}
	return out
}

func (m *Model) fix(rows []Row) {
	m.Sel = min(max(m.Sel, 0), max(len(rows)-1, 0))
}

// Key applies a key: done with a conversation's key opens it, done with ""
// closes; changed reports a new query.
func (m *Model) Key(k string) (choice string, done, changed bool) {
	rows := m.Rows()
	m.fix(rows)
	switch k {
	case "up":
		m.Sel--
	case "down":
		m.Sel++
	case "pgup":
		m.Sel -= max(m.list-1, 1)
	case "pgdown":
		m.Sel += max(m.list-1, 1)
	case "esc", "ctrl-c":
		return "", true, false
	case "enter":
		if m.Sel < len(rows) {
			return m.Convs[rows[m.Sel].Conv].Ref.Key(), true, false
		}
	case "left", "right":
		if m.Sel < len(rows) && m.Query != "" {
			m.fold(rows[m.Sel].Conv, k == "left")
		}
	case "tab":
		m.foldAll()
	case "backspace":
		if r := []rune(m.Query); len(r) > 0 {
			m.Query = string(r[:len(r)-1])
			return "", false, true
		}
	case "ctrl-u":
		if m.Query != "" {
			m.Query = ""
			return "", false, true
		}
	default:
		if utf8.RuneCountInString(k) == 1 && k >= " " {
			m.Query += k
			return "", false, true
		}
	}
	m.fix(rows)
	return "", false, false
}

// fold folds or unfolds conversation i's messages, keeping the selection on
// its row.
func (m *Model) fold(i int, fold bool) {
	if m.Folded == nil {
		m.Folded = map[string]bool{}
	}
	m.Folded[m.Convs[i].Ref.Key()] = fold
	for k, r := range m.Rows() {
		if r.Conv == i && r.Snip == nil {
			m.Sel = k
		}
	}
}

// foldAll folds every conversation, or unfolds them all when all are.
func (m *Model) foldAll() {
	all := true
	for _, r := range m.Rows() {
		if r.Snip == nil && !m.Folded[m.Convs[r.Conv].Ref.Key()] {
			all = false
		}
	}
	m.Folded = map[string]bool{}
	if !all {
		for _, c := range m.Convs {
			m.Folded[c.Ref.Key()] = true
		}
	}
	m.Sel, m.Top = 0, 0
}

// Click handles a click at (x, y) (1-based): a conversation or one of its
// messages opens it, its arrow folds it; the scroll bar scrolls.
func (m *Model) Click(x, y int) string {
	rows := m.Rows()
	switch {
	case y >= 4 && x == m.w && len(rows) > m.list:
		m.Top = sidebar.ScrollTo(len(rows), m.list, y-4)
		m.Sel = m.Top
	case y >= 4 && m.Top+y-4 < len(rows):
		r := rows[m.Top+y-4]
		if r.Snip == nil && x <= 3 && m.Query != "" {
			m.fold(r.Conv, !m.Folded[m.Convs[r.Conv].Ref.Key()])
			return ""
		}
		return m.Convs[r.Conv].Ref.Key()
	}
	return ""
}

// convRow draws conversation c: fold arrow, agent, title, age and count.
func (m *Model) convRow(c Conv, h histsearch.Hit, w int) string {
	arrow := " "
	if m.Query != "" {
		arrow = "▾"
		if m.Folded[c.Ref.Key()] || len(h.Snips) == 0 {
			arrow = "▸"
		}
	}
	sym, color := sidebar.AgentIcon(c.Ref.Agent)
	agent := session.ShortName(c.Ref.Agent)
	age := textfmt.Ago(m.Now, c.Ref.UpdatedAt)
	right, rw := muted+age+reset, sidebar.DisplayWidth(age)
	if m.Query != "" && h.Count > 0 {
		n := " " + strconv.Itoa(h.Count) + " "
		right, rw = right+" "+pill+n+reset, rw+1+len(n)
	}
	title := c.Title
	if c.Pinned {
		title = "📌 " + title
	}
	room := w - 5 - sidebar.DisplayWidth(agent) - 2 - rw - 2
	tw := min(sidebar.DisplayWidth(title), max(room, 1))
	left := " " + muted + arrow + reset + " " + color + sym + reset + " " + muted + agent + reset + "  " + bold + sidebar.Fit(title, tw) + reset
	used := 5 + sidebar.DisplayWidth(agent) + 2 + tw
	return left + strings.Repeat(" ", max(w-used-rw-1, 1)) + right + " "
}

// snipRow draws a message with the match highlighted.
func snipRow(s *histsearch.Snip, agent string, w int) string {
	who := i18n.T("history.you")
	if !s.User {
		who = agent
	}
	lw := sidebar.DisplayWidth(who)
	tw := max(w-6-lw-2, 1)
	r := []rune(s.Text)
	col, n := s.Col, s.Len
	// Keep the match in view.
	start := 0
	if col+n > tw {
		start = min(col-tw/3, len(r))
		start = max(start, 0)
	}
	r = r[start:]
	col -= start
	if len(r) > tw {
		r = r[:tw]
	}
	col = min(max(col, 0), len(r))
	n = min(n, len(r)-col)
	line := text + string(r[:col]) + hit + string(r[col:col+n]) + reset + text + string(r[col+n:]) + reset
	pad := tw - sidebar.DisplayWidth(string(r))
	return "      " + faint + who + reset + "  " + line + strings.Repeat(" ", max(pad, 0))
}

// Render draws the popup on a w×h terminal.
func (m *Model) Render(w, h int) []string {
	rows := m.Rows()
	m.fix(rows)
	list := max(h-4, 1)
	m.w, m.list = w, list
	if m.Sel < m.Top {
		m.Top = m.Sel
	}
	if m.Sel >= m.Top+list {
		m.Top = m.Sel - list + 1
	}
	m.Top = min(max(m.Top, 0), max(len(rows)-list, 0))
	count := i18n.T("history.count", len(m.Convs))
	status := ""
	if m.Query != "" {
		found := 0
		for _, r := range rows {
			if r.Snip == nil {
				found++
			}
		}
		status = i18n.T("history.found", found)
		if m.Busy {
			status = i18n.T("history.searching", m.Looked, len(m.Convs)) + " · " + status
		}
	}
	icon := "📜"
	title := " " + icon + " " + bold + sidebar.Fit(m.Title, max(w-6-sidebar.DisplayWidth(count), 1)) + reset + " " + muted + count + reset + " "
	sw := sidebar.DisplayWidth(status)
	out := []string{
		title,
		" " + pointer + "❯" + reset + " " + sidebar.Fit(m.Query+"▏", max(w-4-sw, 1)) + muted + status + reset + " ",
		faint + strings.Repeat("─", w) + reset,
	}
	bar := sidebar.ScrollBar(len(rows), m.Top, list)
	lw := w
	if bar != nil {
		lw = w - 1
	}
	for i := 0; i < list && m.Top+i < len(rows); i++ {
		n := m.Top + i
		r := rows[n]
		c := m.Convs[r.Conv]
		var line string
		if r.Snip == nil {
			hh, _ := m.matches(c)
			line = m.convRow(c, hh, lw)
		} else {
			line = snipRow(r.Snip, session.ShortName(c.Ref.Agent), lw)
		}
		line = sidebar.Cut(line, 0, lw)
		if n == m.Sel {
			line = selBg + pointer + "▌" + reset + selBg + strings.ReplaceAll(sidebar.Cut(line, 1, lw-1), reset, reset+selBg) + reset
		}
		if bar != nil {
			line += bar[i]
		}
		out = append(out, line)
	}
	switch {
	case len(m.Convs) == 0:
		out = append(out, "   "+muted+sidebar.Fit(i18n.T("history.none"), w-3)+reset)
	case m.Query == "":
	case len(rows) == 0 && !m.Busy:
		out = append(out, "   "+muted+sidebar.Fit(i18n.T("history.no_match", m.Query), w-3)+reset)
	}
	for len(out) < h-1 {
		out = append(out, "")
	}
	return append(out[:h-1], " "+faint+sidebar.Fit(i18n.T("history.hint"), w-1)+reset)
}

// Options run the popup on a terminal.
type Options struct {
	In, Out *os.File
	Load    histsearch.Loader
}

type found struct {
	gen int
	hit histsearch.Hit
	end bool
}

// Run shows the popup until a conversation is chosen (its key) or it is
// closed ("").
func Run(m *Model, o Options) (string, error) {
	fd := int(o.In.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return "", err
	}
	defer restore()
	_, _ = io.WriteString(o.Out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h\x1b[?2004h")
	defer func() { _, _ = io.WriteString(o.Out, "\x1b[?2004l\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()
	winch := make(chan os.Signal, 1)
	sysx.Notify(winch, sysx.SIGWINCH)
	defer sysx.Stop(winch)

	refs := make([]session.Ref, len(m.Convs))
	for i, c := range m.Convs {
		refs[i] = c.Ref
	}
	results := make(chan found, 64)
	gen := 0
	cancel := context.CancelFunc(func() {})
	defer func() { cancel() }()
	start := func() {
		cancel()
		gen++
		m.Hits, m.Looked, m.Sel, m.Top = map[string]histsearch.Hit{}, 0, 0, 0
		m.Busy = m.Query != ""
		if !m.Busy {
			return
		}
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		g, q := gen, m.Query
		go func() {
			select { // a short wait: typing goes on
			case <-time.After(250 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			hits := make(chan histsearch.Hit, 8)
			go histsearch.Search(ctx, refs, q, o.Load, hits, func() {
				select {
				case results <- found{gen: g}:
				case <-ctx.Done():
				}
			})
			for h := range hits {
				select {
				case results <- found{gen: g, hit: h}:
				case <-ctx.Done():
					return
				}
			}
			select {
			case results <- found{gen: g, end: true}:
			case <-ctx.Done():
			}
		}()
	}
	draw := func() {
		w, h := sidebar.Size(int(o.Out.Fd()))
		var sc sidebar.Screen
		for _, l := range m.Render(w, h) {
			sc.Lines = append(sc.Lines, sidebar.Line{Text: l})
		}
		_, _ = io.WriteString(o.Out, sidebar.Frame(sc, w, h))
	}
	var pending []byte
	draw()
	for {
		b, err := sidebar.ReadInput(fd, 50*time.Millisecond)
		if err != nil {
			return "", nil // the popup went away
		}
		redraw := false
		select {
		case <-winch:
			redraw = true
		default:
		}
		for more := true; more; {
			select {
			case r := <-results:
				if r.gen != gen {
					continue
				}
				switch {
				case r.end:
					m.Busy = false
				case r.hit.Ref.ID != "":
					m.Hits[r.hit.Ref.Key()] = r.hit
				default:
					m.Looked++
				}
				redraw = true
			default:
				more = false
			}
		}
		if b == nil {
			if redraw {
				draw()
			}
			continue
		}
		var evs []sidebar.Event
		evs, pending = sidebar.Parse(append(pending, b...), true)
		for _, ev := range evs {
			switch ev.Kind {
			case sidebar.EvScroll:
				if ev.Key == "up" {
					m.Sel -= 3
				} else {
					m.Sel += 3
				}
			case sidebar.EvClick:
				if !sidebar.EarlyClick() {
					if key := m.Click(ev.X, ev.Y); key != "" {
						return key, nil
					}
				}
			case sidebar.EvPaste:
				m.Query += strings.Join(strings.Fields(ev.Text), " ")
				start()
			case sidebar.EvKey:
				choice, done, changed := m.Key(ev.Key)
				if done {
					return choice, nil
				}
				if changed {
					start()
				}
			}
		}
		draw()
	}
}
