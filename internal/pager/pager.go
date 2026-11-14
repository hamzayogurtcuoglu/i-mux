// Package pager shows text in a popup as less does, with a scroll bar
// (§9.3 "Scroll bars"): diffs, commits and files. ↑↓ (j k) a line, PgUp PgDn
// (b, space) a page, Home End (g G) the ends, ←→ (h l) sideways; / searches
// and n N go to the next and previous match; the wheel scrolls; a click or a
// drag on the bar moves there; v edits a file (Editable); q or Esc closes.
// With Marks (a diff's changes, D64) two lanes left of the bar show where
// they are; a click on one goes there, and ] [ go to the next and previous.
package pager

import (
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

const (
	reset = "\x1b[0m"
	faint = "\x1b[38;5;243m"
	cyan  = "\x1b[36m"
	hit   = "\x1b[48;5;94m\x1b[38;5;230m"
	// The marks' lanes: removed lines and added ones.
	markRed   = "\x1b[38;5;203m"
	markGreen = "\x1b[38;5;77m"
)

// Mark is what a line changed, for the lanes beside the bar (D64).
type Mark byte

const (
	MarkRemoved Mark = 1 << iota // a line was removed there (the red lane)
	MarkAdded                    // a line was added there (the green lane)
)

// MarksW is the marks' columns, left of the bar.
const MarksW = 2

// Lines a change is shown below the top when one is gone to.
const changeContext = 3

// Pager is what a pager shows and where.
type Pager struct {
	Header []string // drawn rows above the text, not scrolled
	Lines  []string // the text, drawn (colors kept)
	Top    int      // the first line shown
	Left   int      // the first column shown
	// Editable: line i is the file's line i+1, and v closes the pager with
	// Edit set, for the caller to open the editor at the first line shown.
	Editable bool
	Edit     bool
	// Live, when set, gives the header and the text anew for w columns
	// (a run's output while it grows): at the start, at a resize, and every
	// LiveEvery while it says more is to come. A pager at its end stays at
	// the end; one scrolled up stays where it is.
	Live      func(w int) (header, lines []string, more bool)
	LiveEvery time.Duration
	// Keys are more keys; each returns a note for the bottom row.
	Keys map[string]func() string
	// Hint replaces the bottom row's hint ("" keeps the pager's).
	Hint string
	// Marks are what each line changed (nil: no lanes): beside the bar,
	// the whole text in two lanes, red where lines were removed and green
	// where they were added; a click on a mark goes to its change, and ] [
	// go to the next and previous change (a run of marked lines).
	Marks []Mark

	liveW    int       // the width Live last drew for (0: not yet)
	liveMore bool      // Live said more is to come
	liveAt   time.Time // when Live was last asked

	plain    []string // Lines without colors, for search
	query    string   // the last search
	input    *string  // the search being typed
	matches  []int    // lines with the query
	note     string   // a message on the bottom row
	view, w  int      // the text's rows and the columns of the last Render
	dragging bool     // the bar's thumb is being dragged
	// The change last gone to (its index + 1; 0: none yet) and the top
	// then: ] [ go on from it while the text has not moved since.
	changeAt, changeTop int
	// The match last gone to, the same way for n N: on the last page the
	// top stays while the matches below it are gone through.
	matchAt, matchTop int
}

// DefaultLiveEvery is how often Live is asked while more is to come.
const DefaultLiveEvery = 250 * time.Millisecond

// due reports whether Live is to be asked for w columns at now.
func (p *Pager) due(w int, now time.Time) bool {
	if p.Live == nil {
		return false
	}
	every := p.LiveEvery
	if every <= 0 {
		every = DefaultLiveEvery
	}
	return p.liveW != w || p.liveMore && now.Sub(p.liveAt) >= every
}

// refresh asks Live for the text at w columns. A pager at the end of its
// text (or not drawn yet) follows it there.
func (p *Pager) refresh(w int, now time.Time) {
	follow := p.liveW == 0 || p.Top >= max(len(p.Lines)-p.view, 0)
	p.Header, p.Lines, p.liveMore = p.Live(w)
	p.liveW, p.liveAt = w, now
	p.plain = nil
	if p.query != "" {
		p.matches, p.matchAt = p.matches[:0], 0
		for i := range p.Lines {
			if strings.Contains(p.plainLine(i), p.query) {
				p.matches = append(p.matches, i)
			}
		}
	}
	if follow {
		p.Top = len(p.Lines) // the last page, once clamped
	}
	p.clamp()
}

// body is how many rows the text gets on an h-row screen.
func (p *Pager) body(h int) int { return max(h-len(p.Header)-1, 1) }

func (p *Pager) clamp() {
	p.Top = min(max(p.Top, 0), max(len(p.Lines)-p.view, 0))
	p.Left = max(p.Left, 0)
}

// Render draws the pager on a w×h screen.
func (p *Pager) Render(w, h int) []string {
	p.view, p.w = p.body(h), w
	p.clamp()
	out := make([]string, 0, h)
	for _, l := range p.Header {
		out = append(out, sidebar.Cut(l, 0, w))
	}
	bar := sidebar.ScrollBar(len(p.Lines), p.Top, p.view)
	lanes := p.lanes()
	tw := w
	if bar != nil {
		tw = w - 1
	}
	if lanes != nil {
		tw -= MarksW
	}
	for i := 0; i < p.view; i++ {
		row := strings.Repeat(" ", tw)
		if n := p.Top + i; n < len(p.Lines) {
			row = sidebar.Cut(p.mark(n), p.Left, tw) + reset
		}
		if lanes != nil {
			row += lanes[i]
		}
		if bar != nil {
			row += bar[i]
		}
		out = append(out, row)
	}
	return append(out, p.footer(w))
}

// lanes draws the marks' two columns for the rows shown (nil without
// marks, or without a bar: everything shows): row r stands for its slice
// of the whole text.
func (p *Pager) lanes() []string {
	if len(p.Marks) == 0 || len(p.Lines) <= p.view || p.view <= 0 {
		return nil
	}
	out := make([]string, p.view)
	for r := range out {
		var m Mark
		from, to := p.slice(r)
		for i := from; i < to && i < len(p.Marks); i++ {
			m |= p.Marks[i]
		}
		left, right := " ", " "
		if m&MarkRemoved != 0 {
			left = markRed + "▐" + reset
		}
		if m&MarkAdded != 0 {
			right = markGreen + "▌" + reset
		}
		out[r] = left + right
	}
	return out
}

// slice is the lines row r of the lanes stands for: from up to to.
func (p *Pager) slice(r int) (from, to int) {
	n := len(p.Lines)
	from = r * n / p.view
	return from, max((r+1)*n/p.view, from+1)
}

// changes are the first lines of the runs of marked lines.
func (p *Pager) changes() []int {
	var out []int
	for i, m := range p.Marks {
		if m != 0 && (i == 0 || p.Marks[i-1] == 0) {
			out = append(out, i)
		}
	}
	return out
}

// goChange shows change k of cs, a few lines below the top, and says which.
func (p *Pager) goChange(cs []int, k int) {
	p.Top = cs[k] - changeContext
	p.clamp()
	p.changeAt, p.changeTop = k+1, p.Top
	p.note = i18n.T("pager.change", k+1, len(cs))
}

// nextChange goes to the next (d 1) or previous (d -1) change, around the
// ends: from the one gone to last while the text has not moved since, else
// from the top shown.
func (p *Pager) nextChange(d int) {
	cs := p.changes()
	if len(cs) == 0 {
		return
	}
	n := len(cs)
	if p.changeAt > 0 && p.changeAt <= n && p.Top == p.changeTop {
		p.goChange(cs, (p.changeAt-1+d+n)%n)
		return
	}
	last := max(len(p.Lines)-p.view, 0)
	at := func(i int) int { return min(max(cs[i]-changeContext, 0), last) } // where it shows
	k := -1
	for i := range cs {
		if d > 0 && at(i) > p.Top && k < 0 {
			k = i
		}
		if d < 0 && at(i) < p.Top {
			k = i
		}
	}
	if k < 0 {
		k = 0
		if d < 0 {
			k = n - 1
		}
	}
	p.goChange(cs, k)
}

// mark is line n with the current search's matches on a background, its
// own colors kept around them.
func (p *Pager) mark(n int) string {
	l := p.Lines[n]
	pl := p.plainLine(n)
	if p.query == "" || !strings.Contains(pl, p.query) {
		return l
	}
	in := make([]bool, utf8.RuneCountInString(pl)) // runes inside a match
	q := utf8.RuneCountInString(p.query)
	for off := 0; ; {
		i := strings.Index(pl[off:], p.query)
		if i < 0 {
			break
		}
		start := utf8.RuneCountInString(pl[:off+i])
		for k := start; k < start+q && k < len(in); k++ {
			in[k] = true
		}
		off += i + len(p.query)
	}
	var b strings.Builder
	var active []string // the colors set since the last reset
	r, on := 0, false
	for i := 0; i < len(l); {
		if l[i] == 0x1b {
			j := i + 1
			for j < len(l) && (l[j] < 0x40 || l[j] > 0x7e || l[j] == '[') {
				j++
			}
			j = min(j+1, len(l))
			seq := l[i:j]
			if seq == reset || seq == "\x1b[m" {
				active = active[:0]
			} else {
				active = append(active, seq)
			}
			b.WriteString(seq)
			if on {
				b.WriteString(hit)
			}
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(l[i:])
		want := r < len(in) && in[r]
		if want != on {
			if want {
				b.WriteString(hit)
			} else {
				b.WriteString(reset + strings.Join(active, ""))
			}
			on = want
		}
		b.WriteString(l[i : i+size])
		r++
		i += size
	}
	if on {
		b.WriteString(reset)
	}
	return b.String()
}

func (p *Pager) plainLine(n int) string {
	if len(p.plain) != len(p.Lines) {
		p.plain = make([]string, len(p.Lines))
		for i, l := range p.Lines {
			p.plain[i] = sidebar.Plain(l)
		}
	}
	return p.plain[n]
}

func (p *Pager) footer(w int) string {
	if p.input != nil {
		return " " + cyan + "/" + reset + sidebar.Fit(*p.input+"▏", w-2)
	}
	if p.note != "" {
		return " " + faint + sidebar.Fit(p.note, w-2) + reset
	}
	where := ""
	if n := len(p.Lines); n > p.view {
		last := min(p.Top+p.view, n)
		where = i18n.T("pager.where", p.Top+1, last, n, last*100/n) + " · "
	}
	hint := p.Hint
	if hint == "" {
		hint = i18n.T("pager.hint")
	}
	return " " + faint + sidebar.Fit(where+hint, w-2) + reset
}

// Key applies a key; it reports whether the pager closes.
func (p *Pager) Key(k string) bool {
	if p.input != nil {
		switch k {
		case "enter":
			p.search(*p.input)
			p.input = nil
		case "esc", "ctrl-c":
			p.input = nil
		case "backspace":
			if r := []rune(*p.input); len(r) > 0 {
				*p.input = string(r[:len(r)-1])
			}
		default:
			if utf8.RuneCountInString(k) == 1 {
				*p.input += k
			}
		}
		return false
	}
	p.note = ""
	if f := p.Keys[k]; f != nil {
		p.note = f()
		return false
	}
	page := max(p.view-1, 1)
	switch k {
	case "q", "esc", "ctrl-c":
		return true
	case "down", "j", "enter":
		p.Top++
	case "up", "k":
		p.Top--
	case "pgdown", " ", "f":
		p.Top += page
	case "pgup", "b":
		p.Top -= page
	case "home", "g":
		p.Top = 0
	case "end", "G":
		p.Top = len(p.Lines)
	case "right", "l":
		p.Left += 8
	case "left", "h":
		p.Left -= 8
	case "/":
		s := ""
		p.input = &s
	case "v":
		if p.Editable {
			p.Edit = true
			return true
		}
	case "n":
		p.next(1)
	case "N":
		p.next(-1)
	case "]":
		p.nextChange(1)
	case "[":
		p.nextChange(-1)
	}
	p.clamp()
	return false
}

// search finds q's lines and goes to the first one from the top shown on.
func (p *Pager) search(q string) {
	p.query, p.matches, p.matchAt = q, nil, 0
	if q == "" {
		return
	}
	for i := range p.Lines {
		if strings.Contains(p.plainLine(i), q) {
			p.matches = append(p.matches, i)
		}
	}
	if len(p.matches) == 0 {
		p.note = i18n.T("pager.not_found", strconv.Quote(q))
		return
	}
	p.Top--
	p.next(1)
}

// next goes to the next (d 1) or previous (d -1) line with a match, around
// the ends.
func (p *Pager) next(d int) {
	if len(p.matches) == 0 {
		return
	}
	n := len(p.matches)
	i := -1
	if p.matchAt > 0 && p.matchAt <= n && p.Top == p.matchTop {
		i = (p.matchAt - 1 + d + n) % n // on from the one gone to last
	} else {
		for k, line := range p.matches {
			if d > 0 && line > p.Top && i < 0 {
				i = k
			}
			if d < 0 && line < p.Top {
				i = k
			}
		}
	}
	if i < 0 {
		i = 0
		if d < 0 {
			i = n - 1
		}
	}
	p.Top = p.matches[i]
	p.note = i18n.T("pager.match", i+1, n, strconv.Quote(p.query))
	p.clamp()
	p.matchAt, p.matchTop = i+1, p.Top
}

// Mouse applies a wheel turn, a click or drag on the bar, or a click on the
// marks' lanes: a mark goes to its change, a row without one as the bar.
func (p *Pager) Mouse(ev sidebar.Event) {
	row := ev.Y - len(p.Header) - 1
	switch ev.Kind {
	case sidebar.EvScroll:
		p.note = ""
		if ev.Key == "up" {
			p.Top -= 3
		} else {
			p.Top += 3
		}
	case sidebar.EvClick:
		shown := len(p.Lines) > p.view && row >= 0 && row < p.view
		if shown && p.lanes() != nil && ev.X >= p.w-MarksW && ev.X < p.w {
			p.note = ""
			from, to := p.slice(row)
			for i := from; i < to && i < len(p.Marks); i++ {
				if p.Marks[i] != 0 {
					// The change of the slice's first marked line: the run it is in.
					cs := p.changes()
					p.goChange(cs, sort.SearchInts(cs, i+1)-1)
					return
				}
			}
			p.Top = sidebar.ScrollTo(len(p.Lines), p.view, row)
			break
		}
		p.dragging = ev.X == p.w && shown
		if p.dragging {
			p.note = ""
			p.Top = sidebar.ScrollTo(len(p.Lines), p.view, row)
		}
	case sidebar.EvMove:
		if p.dragging {
			p.Top = sidebar.ScrollTo(len(p.Lines), p.view, min(max(row, 0), p.view-1))
		}
	}
	p.clamp()
}

// Run shows p on the terminal until it is closed.
func Run(p *Pager, in, out *os.File) error {
	fd := int(in.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return err
	}
	defer restore()
	// Alternate screen, no cursor, the wheel, clicks and drags (1002).
	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1002h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(out, "\x1b[?1002l\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	last := ""
	draw := func() {
		w, h := sidebar.Size(int(out.Fd()))
		if now := time.Now(); p.due(w, now) {
			p.refresh(w, now)
		}
		var sc sidebar.Screen
		for _, l := range p.Render(w, h) {
			sc.Lines = append(sc.Lines, sidebar.Line{Text: l})
		}
		if f := sidebar.Frame(sc, w, h); f != last {
			last = f
			_, _ = io.WriteString(out, f)
		}
	}
	var pending []byte
	draw()
	for {
		b, err := sidebar.ReadInput(fd, 50*time.Millisecond)
		if err != nil {
			return nil // the popup went away
		}
		redraw := p.Live != nil // its text may have grown
		select {
		case <-winch:
			redraw = true
			last = ""
		default:
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
			case sidebar.EvKey:
				if p.Key(ev.Key) {
					return nil
				}
			case sidebar.EvPaste:
				if p.input != nil {
					*p.input += ev.Text
				}
			case sidebar.EvClick:
				if !sidebar.EarlyClick() {
					p.Mouse(ev)
				}
			default:
				p.Mouse(ev)
			}
		}
		draw()
	}
}

// Text splits drawn text into lines for a pager, tabs expanded to every
// fourth column (escape sequences take none).
func Text(s string) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if !strings.Contains(l, "\t") {
			continue
		}
		var b strings.Builder
		col := 0
		for k := 0; k < len(l); {
			if l[k] == 0x1b {
				j := k + 1
				for j < len(l) && (l[j] < 0x40 || l[j] > 0x7e || l[j] == '[') {
					j++
				}
				j = min(j+1, len(l))
				b.WriteString(l[k:j])
				k = j
				continue
			}
			r, size := utf8.DecodeRuneInString(l[k:])
			if r == '\t' {
				n := 4 - col%4
				b.WriteString(strings.Repeat(" ", n))
				col += n
			} else {
				b.WriteString(l[k : k+size])
				col += sidebar.DisplayWidth(string(r))
			}
			k += size
		}
		lines[i] = strings.TrimRight(b.String(), "\r")
	}
	return lines
}
