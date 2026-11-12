// Package picker is the open-a-project folder picker (ARCHITECTURE D22,
// §9.3 "Open and close projects"): a filter line and a list that narrows as
// the user types; paths starting with / or ~ are browsed, Tab completes.
package picker

import (
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	muted = "\x1b[38;5;245m"
	faint = "\x1b[38;5;240m"
	selBg = "\x1b[48;5;236m"
	cyan  = "\x1b[36m"
)

// Model is the picker's state.
type Model struct {
	Home     string
	Cands    []folders.Cand // open, recent and git folders
	Restore  *folders.Cand  // "Restore last session", when there is one
	Here     string         // the folder imux started in, offered after Restore ("" = none)
	Query    string
	Sel      int
	Top      int  // first list row shown
	Scanning bool // git folders are still being searched
	Shorten  func(string) string

	w, rows int // the width and list rows of the last Render
}

// List is what the query shows. With nothing typed: the restore row, the
// RECENT section (open and recent folders) and ON THIS MAC (git folders),
// with headers; typed text ranks every folder; a typed path is browsed:
// its folder (when the query ends with "/") and the matching subfolders.
func (m *Model) List() []folders.Cand {
	if folders.IsPath(m.Query) {
		var out []folders.Cand
		if m.Query == "~" || strings.HasSuffix(m.Query, "/") {
			dir := filepath.Clean(folders.Expand(m.Query, m.Home))
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
				out = append(out, folders.Cand{Path: dir, Kind: kindThis})
			}
		}
		return append(out, folders.Browse(m.Query, m.Home)...)
	}
	if strings.TrimSpace(m.Query) != "" {
		return folders.Rank(m.Query, m.Cands, m.Shorten)
	}
	var out []folders.Cand
	if m.Restore != nil {
		out = append(out, *m.Restore)
	}
	if m.Here != "" {
		out = append(out, folders.Cand{Path: m.Here, Kind: kindThis})
	}
	var recent, git []folders.Cand
	for _, c := range m.Cands {
		if c.Path == m.Here && m.Here != "" {
			continue
		}
		if c.Kind == folders.KindGit {
			git = append(git, c)
		} else {
			recent = append(recent, c)
		}
	}
	if len(recent) > 0 {
		out = append(out, folders.Cand{Kind: folders.KindHeader, Label: i18n.T("folders.recent")})
		out = append(out, recent...)
	}
	if len(git) > 0 {
		out = append(out, folders.Cand{Kind: folders.KindHeader, Label: i18n.T("folders.on_this_mac")})
		out = append(out, git...)
	}
	return out
}

// kindThis marks the browsed folder itself.
const kindThis folders.Kind = 100

// fix moves the selection off a header (searching in direction d).
func (m *Model) fix(list []folders.Cand, d int) {
	if len(list) == 0 {
		m.Sel = 0
		return
	}
	m.Sel = min(max(m.Sel, 0), len(list)-1)
	for i := 0; i < len(list); i++ {
		if list[m.Sel].Selectable() {
			return
		}
		next := m.Sel + d
		if next < 0 || next >= len(list) {
			d = -d
			next = m.Sel + d
		}
		if next < 0 || next >= len(list) {
			return
		}
		m.Sel = next
	}
}

// Key applies one key. It returns the chosen row with done, or done with
// a zero row when the user cancelled.
func (m *Model) Key(k string) (choice folders.Cand, done bool) {
	list := m.List()
	m.fix(list, 1)
	switch k {
	case "up":
		if m.Sel > 0 {
			m.Sel--
			m.fix(list, -1)
		}
	case "down":
		if m.Sel < len(list)-1 {
			m.Sel++
			m.fix(list, 1)
		}
	case "esc", "ctrl-c":
		return folders.Cand{}, true
	case "enter":
		if m.Sel < len(list) && list[m.Sel].Selectable() {
			return list[m.Sel], true
		}
		if folders.IsPath(m.Query) {
			p := filepath.Clean(folders.Expand(m.Query, m.Home))
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				return folders.Cand{Path: p, Kind: folders.KindDir}, true
			}
		}
	case "tab":
		if m.Sel < len(list) && list[m.Sel].Path != "" {
			q := m.Query
			if !folders.IsPath(q) {
				q = "~/" // a candidate becomes the folder to browse
			}
			m.Query, m.Sel, m.Top = folders.Complete(q, m.Home, list[m.Sel]), 0, 0
		}
	case "backspace":
		if r := []rune(m.Query); len(r) > 0 {
			m.Query, m.Sel, m.Top = string(r[:len(r)-1]), 0, 0
		}
	case "ctrl-u":
		m.Query, m.Sel, m.Top = "", 0, 0
	default:
		if utf8.RuneCountInString(k) == 1 {
			if c := folders.Clean(k); c != "" {
				m.Query, m.Sel, m.Top = m.Query+c, 0, 0
			}
		}
	}
	return folders.Cand{}, false
}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		if w == 1 {
			return "…"
		}
		return "…" + string(r[len(r)-w+1:]) // keep the end of a path
	}
	return s + strings.Repeat(" ", w-len(r))
}

// rowsFor is how many list rows fit in h terminal rows.
func rowsFor(h int) int { return max(h-4, 1) }

// Render draws the picker on a w×h terminal: title, query, rule, list,
// hint.
func (m *Model) Render(w, h int) []string {
	list := m.List()
	rows := rowsFor(h)
	m.w, m.rows = w, rows
	m.fix(list, 1)
	if m.Sel < m.Top {
		m.Top = m.Sel
		if m.Top > 0 && !list[m.Top-1].Selectable() {
			m.Top-- // keep the section's header in view
		}
	}
	if m.Sel >= m.Top+rows {
		m.Top = m.Sel - rows + 1
	}
	out := []string{
		" " + bold + fit(i18n.T("folders.open_title"), w-1) + reset,
		" " + cyan + ">" + reset + " " + fit(m.Query+"▏", w-3),
		faint + strings.Repeat("─", w) + reset,
	}
	for i := m.Top; i < len(list) && i < m.Top+rows; i++ {
		c := list[i]
		if c.Kind == folders.KindHeader {
			out = append(out, " "+faint+bold+fit(c.Label, w-1)+reset)
			continue
		}
		label := c.Detail
		if label == "" {
			switch c.Kind {
			case folders.KindOpen:
				label = i18n.T("folders.kind_open")
			case folders.KindRecent:
				label = i18n.T("folders.kind_recent")
			case folders.KindGit:
				label = "git"
			case kindThis:
				label = i18n.T("folders.kind_this")
			}
		}
		lw := utf8.RuneCountInString(label)
		text := m.Shorten(c.Path)
		if c.Label != "" {
			text = c.Label
		}
		mark := "  "
		if i == m.Sel {
			mark = cyan + "▸ " + reset
		}
		if c.Kind == folders.KindRestore {
			text = "↺ " + text
		}
		line := " " + mark + fit(text, w-4-lw-1) + " " + muted + label + reset
		if c.Kind == folders.KindRestore {
			line = " " + mark + cyan + fit(text, w-4-lw-1) + reset + " " + muted + label + reset
		}
		if i == m.Sel {
			line = selBg + strings.ReplaceAll(line, reset, reset+selBg) + reset
		}
		out = append(out, line)
	}
	copy(out[3:], sidebar.WithBar(out[3:], sidebar.ScrollBar(len(list), m.Top, rows), w))
	if len(list) == 0 {
		msg := i18n.T("folders.none")
		if m.Scanning && !folders.IsPath(m.Query) {
			msg = i18n.T("folders.searching")
		}
		out = append(out, "   "+muted+fit(msg, w-3)+reset)
	}
	for len(out) < h-1 {
		out = append(out, "")
	}
	return append(out[:h-1], " "+faint+fit(i18n.T("folders.hint"), w-1)+reset)
}

// Options run the picker on a terminal.
type Options struct {
	In   *os.File
	Out  io.Writer
	Home string
	// Cands are listed at once; More, when not nil, delivers the full list
	// once the git folders are found.
	Cands   []folders.Cand
	More    <-chan []folders.Cand
	Restore *folders.Cand
	Here    string // the folder imux started in: a row after Restore (D49)
}

// Run shows the picker until a row is chosen (ok false = cancelled).
func Run(o Options) (folders.Cand, bool, error) {
	restore, err := sidebar.RawMode(int(o.In.Fd()))
	if err != nil {
		return folders.Cand{}, false, err
	}
	defer restore()
	_, _ = io.WriteString(o.Out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(o.Out, "\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()

	m := &Model{Home: o.Home, Cands: o.Cands, Restore: o.Restore, Here: o.Here, Scanning: o.More != nil, Shorten: func(p string) string {
		if p == o.Home {
			return "~"
		}
		if strings.HasPrefix(p, o.Home+"/") {
			return "~" + p[len(o.Home):]
		}
		return p
	}}
	draw := func() {
		w, h := sidebar.Size(int(o.Out.(*os.File).Fd()))
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
	more := o.More
	var pending []byte
	draw()
	for {
		select {
		case <-winch:
		case c, ok := <-more:
			more = nil
			m.Scanning = false
			if ok {
				m.Cands = c
			}
		case b, ok := <-input:
			if !ok {
				return folders.Cand{}, false, nil
			}
			var evs []sidebar.Event
			evs, pending = sidebar.Parse(append(pending, b...), true)
			for _, ev := range evs {
				if ev.Kind == sidebar.EvScroll {
					for range 3 {
						m.Key(ev.Key)
					}
					continue
				}
				if ev.Kind == sidebar.EvClick && !sidebar.EarlyClick() {
					// A click on the scroll bar scrolls; on a list row opens it.
					list := m.List()
					if ev.X == m.w && len(list) > m.rows && ev.Y >= 4 {
						m.Top = sidebar.ScrollTo(len(list), m.rows, ev.Y-4)
						m.Sel = m.Top
						m.fix(list, 1)
						continue
					}
					if i := m.Top + ev.Y - 4; ev.Y >= 4 && i < len(list) && list[i].Selectable() {
						return list[i], true, nil
					}
					continue
				}
				if ev.Kind != sidebar.EvKey {
					continue
				}
				if choice, done := m.Key(ev.Key); done {
					return choice, choice.Kind == folders.KindRestore || choice.Path != "", nil
				}
			}
		}
		draw()
	}
}
