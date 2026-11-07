// Package searchview is the project search popup (ARCHITECTURE D26): a query
// line, results grouped by file as you type, Enter opens a result.
package searchview

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/search"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	muted = "\x1b[38;5;247m"
	faint = "\x1b[38;5;243m"
	cyan  = "\x1b[36m"
	selBg = "\x1b[48;5;236m"
	hit   = "\x1b[48;5;58m\x1b[38;5;229m"
)

// Row is one line of the result list: a file header or a match.
type Row struct {
	File  string
	Count int           // matches in the file (header rows)
	Match *search.Match // nil for a header
}

// Model is the search popup's state.
type Model struct {
	Title   string
	Query   string
	Results []search.Match
	Cut     bool // the search stopped at its limit
	Busy    bool // a search is running
	Sel     int  // index into Rows (a match row)
	Top     int
}

// Rows groups the results by file.
func (m *Model) Rows() []Row {
	var out []Row
	for i := 0; i < len(m.Results); {
		f := m.Results[i].Path
		j := i
		for j < len(m.Results) && m.Results[j].Path == f {
			j++
		}
		out = append(out, Row{File: f, Count: j - i})
		for k := i; k < j; k++ {
			out = append(out, Row{File: f, Match: &m.Results[k]})
		}
		i = j
	}
	return out
}

// fix moves the selection onto a match row (searching in direction d).
func (m *Model) fix(rows []Row, d int) {
	if len(rows) == 0 {
		m.Sel = 0
		return
	}
	m.Sel = min(max(m.Sel, 0), len(rows)-1)
	for range rows {
		if rows[m.Sel].Match != nil {
			return
		}
		next := m.Sel + d
		if next < 0 || next >= len(rows) {
			d = -d
			next = m.Sel + d
			if next < 0 || next >= len(rows) {
				return
			}
		}
		m.Sel = next
	}
}

// Key applies a key. changed reports a new query; done with a match opens
// it, done without one closes the popup.
func (m *Model) Key(k string) (choice *search.Match, done, changed bool) {
	rows := m.Rows()
	m.fix(rows, 1)
	switch k {
	case "up":
		if m.Sel > 0 {
			m.Sel--
			m.fix(rows, -1)
		}
	case "down":
		if m.Sel < len(rows)-1 {
			m.Sel++
			m.fix(rows, 1)
		}
	case "esc", "ctrl-c":
		return nil, true, false
	case "enter":
		if m.Sel < len(rows) && rows[m.Sel].Match != nil {
			mt := *rows[m.Sel].Match
			return &mt, true, false
		}
	case "backspace":
		if r := []rune(m.Query); len(r) > 0 {
			m.Query = string(r[:len(r)-1])
			return nil, false, true
		}
	case "ctrl-u":
		if m.Query != "" {
			m.Query = ""
			return nil, false, true
		}
	default:
		if utf8.RuneCountInString(k) == 1 {
			if c := folders.Clean(k); c != "" {
				m.Query += c
				return nil, false, true
			}
		}
	}
	return nil, false, false
}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

// matchText draws a match's line with the match highlighted, trimmed to w
// columns around it.
func matchText(mt *search.Match, w int) string {
	line := []rune(strings.ReplaceAll(mt.Text, "\t", " "))
	start := 0
	for start < len(line) && line[start] == ' ' {
		start++
	}
	col, n := mt.Col, mt.Len
	if col < start {
		start = col
	}
	// Keep the match in view.
	if col+n-start > w-1 {
		start = max(col-w/3, start)
	}
	line = line[start:]
	col -= start
	if col+n > len(line) {
		n = max(len(line)-col, 0)
	}
	if len(line) > w {
		line = line[:w]
		if col+n > w {
			n = max(w-col, 0)
		}
	}
	before, match, after := string(line[:col]), string(line[col:col+n]), string(line[col+n:])
	pad := w - len(line)
	return before + hit + match + reset + after + strings.Repeat(" ", max(pad, 0))
}

// Render draws the popup on a w×h terminal.
func (m *Model) Render(w, h int) []string {
	rows := m.Rows()
	m.fix(rows, 1)
	list := max(h-4, 1)
	if m.Sel < m.Top {
		m.Top = m.Sel
		if m.Top > 0 && rows[m.Top-1].Match == nil {
			m.Top--
		}
	}
	if m.Sel >= m.Top+list {
		m.Top = m.Sel - list + 1
	}
	files := 0
	for _, r := range rows {
		if r.Match == nil {
			files++
		}
	}
	status := ""
	switch {
	case m.Busy:
		status = i18n.T("search.busy")
	case m.Query != "" && len(m.Results) == 0:
		status = i18n.T("search.none")
	case m.Query != "":
		status = i18n.T("search.count", len(m.Results), files)
		if m.Cut {
			status = i18n.T("search.cut", len(m.Results))
		}
	}
	q := m.Query + "▏"
	out := []string{
		" " + bold + fit(m.Title, w-1) + reset,
		" " + cyan + ">" + reset + " " + fit(q, max(w-4-utf8.RuneCountInString(status), 1)) + muted + status + reset + " ",
		faint + strings.Repeat("─", w) + reset,
	}
	for i := m.Top; i < len(rows) && i < m.Top+list; i++ {
		r := rows[i]
		if r.Match == nil {
			c := strconv.Itoa(r.Count)
			out = append(out, " "+bold+fit(r.File, w-3-len(c))+reset+" "+muted+c+reset+" ")
			continue
		}
		no := strconv.Itoa(r.Match.Line)
		mark := "  "
		if i == m.Sel {
			mark = cyan + "▸ " + reset
		}
		line := " " + mark + muted + fit(no, 5) + reset + matchText(r.Match, max(w-9, 1)) + " "
		if i == m.Sel {
			line = selBg + strings.ReplaceAll(line, reset, reset+selBg) + reset
		}
		out = append(out, line)
	}
	if m.Query == "" {
		out = append(out, "   "+muted+fit(i18n.T("search.type"), w-3)+reset)
	}
	for len(out) < h-1 {
		out = append(out, "")
	}
	return append(out[:h-1], " "+faint+fit(i18n.T("search.hint"), w-1)+reset)
}

// Options run the popup on a terminal in project folder Dir.
type Options struct {
	In  *os.File
	Out *os.File
	Dir string
}

type result struct {
	gen     int
	matches []search.Match
	cut     bool
}

// Run shows the popup until a match is chosen (nil: closed).
func Run(o Options) (*search.Match, error) {
	restore, err := sidebar.RawMode(int(o.In.Fd()))
	if err != nil {
		return nil, err
	}
	defer restore()
	_, _ = io.WriteString(o.Out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(o.Out, "\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()

	name := o.Dir
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	m := &Model{Title: i18n.T("search.title", name)}
	files := search.Files(context.Background(), o.Dir)
	results := make(chan result, 4)
	gen := 0
	var cancel context.CancelFunc = func() {}
	start := func() {
		cancel()
		gen++
		m.Busy = m.Query != ""
		if m.Query == "" {
			m.Results, m.Cut = nil, false
			return
		}
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		g, q := gen, m.Query
		go func() {
			select { // a short wait: typing goes on
			case <-time.After(150 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			ms, cut := search.Search(ctx, o.Dir, files, q)
			if ctx.Err() == nil {
				results <- result{g, ms, cut}
			}
		}()
	}
	defer func() { cancel() }()

	draw := func() {
		w, h := sidebar.Size(int(o.Out.Fd()))
		var b strings.Builder
		b.WriteString("\x1b[H")
		for i, l := range m.Render(w, h) {
			if i > 0 {
				b.WriteString("\r\n")
			}
			b.WriteString(l + "\x1b[K")
		}
		b.WriteString("\x1b[J")
		_, _ = io.WriteString(o.Out, b.String())
	}
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	input := make(chan []byte, 8)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := o.In.Read(buf)
			if n > 0 {
				b := make([]byte, n)
				copy(b, buf[:n])
				input <- b
			}
			if err != nil {
				close(input)
				return
			}
		}
	}()
	var pending []byte
	draw()
	for {
		select {
		case <-winch:
		case r := <-results:
			if r.gen == gen {
				m.Results, m.Cut, m.Busy, m.Sel, m.Top = r.matches, r.cut, false, 0, 0
			}
		case b, ok := <-input:
			if !ok {
				return nil, nil
			}
			var evs []sidebar.Event
			evs, pending = sidebar.Parse(append(pending, b...), true)
			for _, ev := range evs {
				if ev.Kind == sidebar.EvClick {
					rows := m.Rows()
					if i := m.Top + ev.Y - 4; ev.Y >= 4 && i < len(rows) && rows[i].Match != nil {
						mt := *rows[i].Match
						return &mt, nil
					}
					continue
				}
				if ev.Kind != sidebar.EvKey {
					continue
				}
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
