// Package searchview is the project search popup (ARCHITECTURE D26): a query
// line, results grouped by file as you type, Enter opens a result.
package searchview

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
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
	hit   = "\x1b[48;5;94m\x1b[38;5;230m"
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
	Sel     int  // index into Rows
	Top     int
	// Folded holds the files whose matches are hidden.
	Folded map[string]bool
	Theme  string // file icons: sidebar.IconsNerd or IconsEmoji

	w, list int   // the width and list rows of the last Render
	btn     []int // the fold-all button's columns, [from, to) (1-based)
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
		if !m.Folded[f] {
			for k := i; k < j; k++ {
				out = append(out, Row{File: f, Match: &m.Results[k]})
			}
		}
		i = j
	}
	return out
}

// fix keeps the selection on a row.
func (m *Model) fix(rows []Row, _ int) {
	if len(rows) == 0 {
		m.Sel = 0
		return
	}
	m.Sel = min(max(m.Sel, 0), len(rows)-1)
}

// toggle folds or unfolds a file's matches, keeping the selection on its
// header.
func (m *Model) toggle(file string, fold bool) {
	if m.Folded == nil {
		m.Folded = map[string]bool{}
	}
	m.Folded[file] = fold
	for i, r := range m.Rows() {
		if r.Match == nil && r.File == file {
			m.Sel = i
			return
		}
	}
}

// ToggleAll folds every file, or unfolds them all when all are folded.
func (m *Model) ToggleAll() {
	all := true
	for _, r := range m.Rows() {
		if r.Match == nil && !m.Folded[r.File] {
			all = false
		}
	}
	m.Folded = map[string]bool{}
	m.Sel, m.Top = 1, 0 // unfolded: the first match
	if !all {
		for _, r := range m.Results {
			m.Folded[r.Path] = true
		}
		m.Sel = 0
	}
}

// Scroll moves the view by d rows (the mouse wheel).
func (m *Model) Scroll(d int) {
	rows := m.Rows()
	m.Sel = min(max(m.Sel+d, 0), max(len(rows)-1, 0))
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
		if m.Sel < len(rows) {
			m.toggle(rows[m.Sel].File, !m.Folded[rows[m.Sel].File])
		}
	case "left":
		if m.Sel < len(rows) {
			m.toggle(rows[m.Sel].File, true)
		}
	case "right":
		if m.Sel < len(rows) {
			m.toggle(rows[m.Sel].File, false)
		}
	case "tab":
		m.ToggleAll()
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

func fit(s string, w int) string { return sidebar.Fit(s, w) }

// Colors of the result list.
const (
	text    = "\x1b[38;5;250m"
	lineNo  = "\x1b[38;5;240m"
	pill    = "\x1b[48;5;238m\x1b[38;5;252m"
	pointer = "\x1b[36m"
)

// matchText draws a match's line in w columns: its text with the match
// highlighted, scrolled to keep the match in view.
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
	used := sidebar.DisplayWidth(string(line))
	return text + before + hit + match + reset + text + after + reset + strings.Repeat(" ", max(w-used, 0))
}

// AutoFold is how many files a search may find before its files start
// folded, each a row with its count.
const AutoFold = 5

// FoldMany folds every file when there are more than AutoFold.
func (m *Model) FoldMany() {
	m.Folded = nil
	files := map[string]bool{}
	for _, r := range m.Results {
		files[r.Path] = true
	}
	if len(files) <= AutoFold {
		return
	}
	m.Folded = files
	m.Sel, m.Top = 0, 0
}

// allFolded reports whether every file is folded.
func (m *Model) allFolded() bool {
	for _, r := range m.Results {
		if !m.Folded[r.Path] {
			return false
		}
	}
	return len(m.Results) > 0
}

// fileRow draws a file's row: fold arrow, icon, name, folder and count.
func (m *Model) fileRow(r Row, w int) string {
	arrow := "▾"
	if m.Folded[r.File] {
		arrow = "▸"
	}
	icon, iw := sidebar.FileIcon(filepath.Base(r.File), m.Theme)
	name, dir := filepath.Base(r.File), filepath.Dir(r.File)
	if dir == "." {
		dir = ""
	}
	c := " " + strconv.Itoa(r.Count) + " "
	room := w - 5 - iw - len(c) - 1 // " ▾ " icon " " … " " count " "
	nw := min(sidebar.DisplayWidth(name), max(room, 1))
	dw := max(room-nw-2, 0)
	left := " " + muted + arrow + reset + " " + icon + " " + bold + fit(name, nw) + reset
	if dw > 0 && dir != "" {
		left += "  " + faint + fit(dir, dw) + reset
		nw += 2 + dw
	}
	pad := max(w-5-iw-nw-len(c)-1, 0)
	return left + strings.Repeat(" ", pad) + pill + c + reset + " "
}

// Render draws the popup on a w×h terminal.
func (m *Model) Render(w, h int) []string {
	rows := m.Rows()
	m.fix(rows, 1)
	list := max(h-4, 1)
	m.w, m.list = w, list
	if m.Sel < m.Top {
		m.Top = m.Sel
		if m.Top > 0 && rows[m.Top-1].Match == nil {
			m.Top--
		}
	}
	if m.Sel >= m.Top+list {
		m.Top = m.Sel - list + 1
	}
	m.Top = min(max(m.Top, 0), max(len(rows)-list, 0))
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
	// The title, with the button that folds or unfolds every file.
	m.btn = nil
	btn := ""
	if files > 0 {
		btn = i18n.T("search.collapse_all")
		if m.allFolded() {
			btn = i18n.T("search.expand_all")
		}
	}
	bw := sidebar.DisplayWidth(btn)
	ic := sidebar.ButtonIcons(m.Theme).Search
	iw := sidebar.DisplayWidth(ic)
	title := " " + pointer + ic + reset + " " + bold + fit(m.Title, max(w-bw-4-iw, 1)) + reset
	if btn != "" {
		title += " " + muted + btn + reset + " "
		m.btn = []int{w - bw, w} // 1-based columns [from, to)
	}
	sw := sidebar.DisplayWidth(status)
	q := m.Query + "▏"
	out := []string{
		title,
		" " + pointer + "❯" + reset + " " + fit(q, max(w-4-sw, 1)) + muted + status + reset + " ",
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
		var line string
		if r.Match == nil {
			line = m.fileRow(r, lw)
		} else {
			no := strconv.Itoa(r.Match.Line)
			line = "     " + lineNo + fmt.Sprintf("%5s", no) + reset + "  " + matchText(r.Match, max(lw-13, 1)) + " "
		}
		if n == m.Sel {
			line = selBg + pointer + "▌" + reset + selBg + strings.ReplaceAll(sidebar.Cut(line, 1, lw-1), reset, reset+selBg) + reset
		}
		if bar != nil {
			line += bar[i]
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

// Click handles a click at (x, y) (1-based): the fold-all button, the
// scroll bar, a file (fold) or a match (open it).
func (m *Model) Click(x, y int) *search.Match {
	rows := m.Rows()
	switch {
	case y == 1 && m.btn != nil && x >= m.btn[0] && x < m.btn[1]:
		m.ToggleAll()
	case y >= 4 && x == m.w && len(rows) > m.list:
		m.Top = sidebar.ScrollTo(len(rows), m.list, y-4)
		m.Sel = m.Top
	case y >= 4 && m.Top+y-4 < len(rows):
		r := rows[m.Top+y-4]
		if r.Match != nil {
			mt := *r.Match
			return &mt
		}
		m.toggle(r.File, !m.Folded[r.File])
	}
	return nil
}

// Options run the popup on a terminal in project folder Dir.
type Options struct {
	In    *os.File
	Out   *os.File
	Dir   string
	Theme string // the file icons' (sidebar.IconsNerd, IconsEmoji)
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
	m := &Model{Title: i18n.T("search.title", name), Theme: o.Theme}
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
	// No reader goroutine: one left waiting after the popup hands the
	// terminal to the editor would swallow the editor's first key.
	fd := int(o.In.Fd())
	var pending []byte
	draw()
	for {
		b, err := sidebar.ReadInput(fd, 50*time.Millisecond)
		if err != nil {
			return nil, nil
		}
		changed := false
		select {
		case <-winch:
			changed = true
		default:
		}
		select {
		case r := <-results:
			if r.gen == gen {
				m.Results, m.Cut, m.Busy, m.Sel, m.Top = r.matches, r.cut, false, 1, 0 // the first match
				m.FoldMany()
				changed = true
			}
		default:
		}
		if b == nil {
			if changed {
				draw()
			}
			continue
		}
		{
			var evs []sidebar.Event
			evs, pending = sidebar.Parse(append(pending, b...), true)
			for _, ev := range evs {
				if ev.Kind == sidebar.EvScroll {
					if ev.Key == "up" {
						m.Scroll(-3)
					} else {
						m.Scroll(3)
					}
					continue
				}
				if ev.Kind == sidebar.EvClick && !sidebar.EarlyClick() {
					if mt := m.Click(ev.X, ev.Y); mt != nil {
						return mt, nil
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
