package gotofile

import (
	"context"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
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

// Limit is the most rows the list shows.
const Limit = 200

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	cyan   = "\x1b[36m"
	hit    = "\x1b[1m\x1b[33m" // a matched letter
	muted  = "\x1b[38;5;247m"
	faint  = "\x1b[38;5;240m"
	selBg  = "\x1b[48;5;24m"
	header = 3 // title, query, rule
)

// Model is the popup's state.
type Model struct {
	Title   string
	Query   string
	Files   []string // every file, relative to the project folder
	Recent  []string // the files changed last first (the empty query's list)
	Results []Result
	Sel     int
	Top     int

	w, list int // the width and list rows of the last Render
}

// Update ranks the files for the query.
func (m *Model) Update() {
	q := Parse(m.Query)
	m.Sel, m.Top = 0, 0
	if q.Empty() {
		m.Results = m.Results[:0]
		for _, f := range m.Recent {
			if len(m.Results) == Limit {
				break
			}
			m.Results = append(m.Results, Result{Path: f})
		}
		return
	}
	m.Results = Rank(m.Files, q, Limit)
}

// Chosen is the selected file and the query's line (0 for none).
func (m *Model) Chosen() (string, int, bool) {
	if m.Sel < 0 || m.Sel >= len(m.Results) {
		return "", 0, false
	}
	return m.Results[m.Sel].Path, Parse(m.Query).Line, true
}

// Key applies a key: done with ok opens the chosen file, done without it
// closes the popup.
func (m *Model) Key(k string) (done, ok bool) {
	switch k {
	case "up", "ctrl-p", "backtab":
		if m.Sel > 0 {
			m.Sel--
		}
	case "down", "ctrl-n", "tab":
		if m.Sel < len(m.Results)-1 {
			m.Sel++
		}
	case "esc", "ctrl-c":
		return true, false
	case "enter":
		_, _, ok := m.Chosen()
		return ok, ok
	case "backspace":
		if r := []rune(m.Query); len(r) > 0 {
			m.Query = string(r[:len(r)-1])
			m.Update()
		}
	case "ctrl-u":
		if m.Query != "" {
			m.Query = ""
			m.Update()
		}
	default:
		if utf8.RuneCountInString(k) == 1 {
			if c := folders.Clean(k); c != "" {
				m.Query += c
				m.Update()
			}
		}
	}
	return false, false
}

// Scroll moves the selection by d rows.
func (m *Model) Scroll(d int) {
	m.Sel = min(max(m.Sel+d, 0), max(len(m.Results)-1, 0))
}

// paint writes runes rs (those at index offset+i in pos highlighted) in
// style st, at most w columns; it returns the text and its width.
func paint(rs []rune, offset int, pos map[int]bool, st string, w int) (string, int) {
	var b strings.Builder
	b.WriteString(st)
	used := 0
	for i, r := range rs {
		rw := sidebar.DisplayWidth(string(r))
		if used+rw > w {
			break
		}
		if pos[offset+i] {
			b.WriteString(hit + string(r) + reset + st)
		} else {
			b.WriteRune(r)
		}
		used += rw
	}
	b.WriteString(reset)
	return b.String(), used
}

// row draws a result: the file's name, then its folder, dimmer, like the
// editor's list.
func row(r Result, w int) string {
	rs := []rune(r.Path)
	cut := strings.LastIndex(r.Path, "/") + 1
	base := utf8.RuneCountInString(r.Path[:cut])
	pos := map[int]bool{}
	for _, p := range r.Pos {
		pos[p] = true
	}
	name, nw := paint(rs[base:], base, pos, bold, w)
	if base == 0 || nw+2 >= w {
		return name + strings.Repeat(" ", max(w-nw, 0))
	}
	dirRunes := rs[:base-1]
	room := w - nw - 2
	start := 0
	if dw := sidebar.DisplayWidth(string(dirRunes)); dw > room {
		// Keep the end of a long folder: "…ar/render".
		for start < len(dirRunes) && sidebar.DisplayWidth(string(dirRunes[start:]))+1 > room {
			start++
		}
		d, used := paint(dirRunes[start:], start, pos, muted, room-1)
		return name + "  " + muted + "…" + reset + d + strings.Repeat(" ", max(room-1-used, 0))
	}
	d, used := paint(dirRunes, 0, pos, muted, room)
	return name + "  " + d + strings.Repeat(" ", max(room-used, 0))
}

// Render draws the popup for w columns and h rows.
func (m *Model) Render(w, h int) []string {
	list := max(h-header-1, 1)
	m.w, m.list = w, list
	if m.Sel < m.Top {
		m.Top = m.Sel
	}
	if m.Sel >= m.Top+list {
		m.Top = m.Sel - list + 1
	}
	status := ""
	switch {
	case m.Query == "":
		status = i18n.T("goto.recent")
	case len(m.Results) == 0:
		status = i18n.T("goto.none")
	default:
		status = i18n.T("goto.count", len(m.Results))
		if len(m.Results) == Limit {
			status = i18n.T("goto.more", Limit)
		}
	}
	q := m.Query + "▏"
	qw := max(w-4-sidebar.DisplayWidth(status), 1)
	out := []string{
		" " + bold + sidebar.Fit(m.Title, w-1) + reset,
		" " + cyan + ">" + reset + " " + sidebar.Fit(q, qw) + muted + status + reset + " ",
		faint + strings.Repeat("─", w) + reset,
	}
	for i := m.Top; i < len(m.Results) && i < m.Top+list; i++ {
		mark := "  "
		if i == m.Sel {
			mark = cyan + "▸ " + reset
		}
		line := " " + mark + row(m.Results[i], max(w-4, 1)) + " "
		if i == m.Sel {
			line = selBg + strings.ReplaceAll(line, reset, reset+selBg) + reset
		}
		out = append(out, line)
	}
	copy(out[header:], sidebar.WithBar(out[header:], sidebar.ScrollBar(len(m.Results), m.Top, list), w))
	for len(out) < h-1 {
		out = append(out, "")
	}
	return append(out[:h-1], " "+faint+sidebar.Fit(i18n.T("goto.hint"), w-1)+reset)
}

// RecentFirst orders files by change time, newest first.
func RecentFirst(dir string, files []string) []string {
	type f struct {
		p string
		t int64
	}
	all := make([]f, 0, len(files))
	for _, p := range files {
		var t int64
		if fi, err := os.Stat(filepath.Join(dir, p)); err == nil {
			t = fi.ModTime().UnixNano()
		}
		all = append(all, f{p, t})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].t > all[j].t })
	out := make([]string, len(all))
	for i, x := range all {
		out[i] = x.p
	}
	return out
}

// Options run the popup on a terminal in project folder Dir.
type Options struct {
	In  *os.File
	Out *os.File
	Dir string
}

// Run shows the popup until a file is chosen: its path (relative to Dir)
// and the query's line; "" when closed.
func Run(o Options) (string, int, error) {
	restore, err := sidebar.RawMode(int(o.In.Fd()))
	if err != nil {
		return "", 0, err
	}
	defer restore()
	_, _ = io.WriteString(o.Out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(o.Out, "\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()

	files := search.Files(context.Background(), o.Dir)
	m := &Model{Title: i18n.T("goto.title", filepath.Base(o.Dir)), Files: files, Recent: RecentFirst(o.Dir, files)}
	m.Update()
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
	// No reader goroutine: one left waiting after the popup hands the
	// terminal to the editor would swallow the editor's first key.
	fd := int(o.In.Fd())
	var pending []byte
	draw()
	for {
		b, err := sidebar.ReadInput(fd, 50*time.Millisecond)
		if err != nil {
			return "", 0, nil
		}
		select {
		case <-winch:
			draw()
		default:
		}
		if b == nil {
			continue
		}
		{
			var evs []sidebar.Event
			evs, pending = sidebar.Parse(append(pending, b...), true)
			for _, ev := range evs {
				switch ev.Kind {
				case sidebar.EvScroll:
					if ev.Key == "up" {
						m.Scroll(-3)
					} else {
						m.Scroll(3)
					}
				case sidebar.EvClick:
					if sidebar.EarlyClick() {
						continue
					}
					if ev.X == m.w && len(m.Results) > m.list && ev.Y > header {
						m.Top = sidebar.ScrollTo(len(m.Results), m.list, ev.Y-header-1)
						m.Sel = m.Top
						continue
					}
					if i := m.Top + ev.Y - header - 1; ev.Y > header && i < len(m.Results) {
						m.Sel = i
						p, line, _ := m.Chosen()
						return p, line, nil
					}
				case sidebar.EvKey:
					if done, ok := m.Key(ev.Key); done {
						if !ok {
							return "", 0, nil
						}
						p, line, _ := m.Chosen()
						return p, line, nil
					}
				}
			}
		}
		draw()
	}
}

// LineArgs is the editor's arguments for path at line (0: none).
func LineArgs(path string, line int) []string {
	if line > 0 {
		return []string{"--line", strconv.Itoa(line), path}
	}
	return []string{path}
}
