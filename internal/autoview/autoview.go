// Package autoview draws a Scheduler run's output (§9.3 "Scheduler", D61):
// what the agent said, each tool it called on a line with what the tool
// gave back under it, and a header that says how the run goes.
package autoview

import (
	"regexp"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
)

// Colors from the theme (D68), set again when it changes.
var cyan, green, red, yel, muted, faint, spin, code string

func init() {
	theme.Watch(func(p theme.Palette) {
		s := theme.StylesOf(p)
		cyan = s.Accent
		green = s.Green
		red = s.Red
		yel = s.Yellow
		muted = s.Muted
		faint = s.Faint
		spin = s.Blue
		code = s.Teal
	})
}

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Lines draws o for w columns: the agent's words (their Markdown drawn),
// each tool on a line with its result's first line under it, a blank line
// between steps.
func Lines(o *auto.Output, w int) []string {
	w = max(w, 20)
	var out []string
	gap := func() {
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
	}
	for _, it := range o.Items {
		text := Safe(it.Text)
		switch it.Kind {
		case auto.ItemText:
			gap()
			out = append(out, markdown(text, w)...)
			gap()
		case auto.ItemTool:
			gap()
			line := " " + cyan + "●" + reset + " " + bold + Safe(it.Name) + reset
			if text != "" {
				line += " " + muted + strings.ReplaceAll(text, "\n", " ") + reset
			}
			out = append(out, cut(line, w))
		case auto.ItemResult:
			if text == "" && !it.Failed {
				continue
			}
			color := faint
			if it.Failed {
				color = red
			}
			line := "   " + faint + "⎿ " + reset + color + text + reset
			if it.More > 0 {
				line += faint + "  " + i18n.T("auto.view.more", it.More) + reset
			}
			out = append(out, cut(line, w))
		case auto.ItemNote:
			for _, p := range wrap(text, w-2) {
				out = append(out, " "+faint+p+reset)
			}
		case auto.ItemError:
			gap()
			for i, p := range wrap(text, w-4) {
				lead := "   "
				if i == 0 {
					lead = " ✗ "
				}
				out = append(out, red+lead+p+reset)
			}
		case auto.ItemPlain:
			style := "" // a text CLI's output
			if o.Streamed {
				style = faint // a warning among the events
			}
			for _, p := range wrap(text, w-2) {
				out = append(out, " "+style+p+reset)
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

var escRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[@-_]`)

// Safe is text an agent or a tool gave, safe to draw: no escapes or other
// control characters (a terminal would move or color on them), tabs as
// spaces to the next fourth column.
func Safe(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r < ' ' && r != '\n' || r == 0x7f || r >= 0x80 && r < 0xa0 }) {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range escRE.ReplaceAllString(s, "") {
		switch {
		case r == '\n':
			col = 0
		case r == '\t':
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		case r < ' ' || r == 0x7f || r >= 0x80 && r < 0xa0:
			continue
		default:
			col += sidebar.DisplayWidth(string(r))
		}
		b.WriteRune(r)
	}
	return b.String()
}

var (
	listRE = regexp.MustCompile(`^(\s*)([-*+•]|\d{1,3}[.)])\s+`)
	ruleRE = regexp.MustCompile(`^\s*([-*_])(\s*[-*_]){2,}\s*$`)
)

// markdown draws an agent's Markdown for w columns: headings bold, **bold**
// and `code` set off, list items with a bullet and their lines under their
// text, quotes behind a bar, code blocks in color.
func markdown(text string, w int) []string {
	var out []string
	blank := func() {
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
	}
	fence := false
	for _, l := range strings.Split(strings.Trim(text, "\n"), "\n") {
		l = strings.TrimRight(l, " ")
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			var s styled
			s.add(l, code)
			out = append(out, s.block("   ", "   ", w)...)
			continue
		}
		h := strings.TrimLeft(l, "#")
		switch m := listRE.FindStringSubmatch(l); {
		case strings.TrimSpace(l) == "":
			blank()
		case h != l && strings.HasPrefix(h, " ") && len(l)-len(h) <= 6:
			out = append(out, inline(strings.TrimSpace(h), bold).block(" ", " ", w)...)
		case ruleRE.MatchString(l):
			out = append(out, " "+faint+strings.Repeat("─", min(w-2, 40))+reset)
		case m != nil:
			marker := m[2]
			if !strings.ContainsAny(marker, ".)") {
				marker = "•"
			}
			first := " " + m[1] + marker + " "
			rest := strings.Repeat(" ", sidebar.DisplayWidth(first))
			out = append(out, inline(l[len(m[0]):], "").block(first, rest, w)...)
		case strings.HasPrefix(l, ">"):
			bar := " " + faint + "│" + reset + " "
			out = append(out, inline(strings.TrimSpace(strings.TrimLeft(l, "> ")), muted).block(bar, bar, w)...)
		default:
			out = append(out, inline(l, "").block(" ", " ", w)...)
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// styled is text with a style (SGR) for each rune.
type styled struct {
	rs []rune
	st []string
}

func (s *styled) add(text, style string) {
	for _, r := range text {
		s.rs = append(s.rs, r)
		s.st = append(s.st, style)
	}
}

// inline reads a Markdown line's **bold** and `code` on base; a marker
// with no closing one stays as it is.
func inline(l, base string) styled {
	var s styled
	rs := []rune(l)
	closes := func(from int, m string) bool { return strings.Contains(string(rs[min(from, len(rs)):]), m) }
	isBold, isCode := false, false
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '`' && (isCode || closes(i+1, "`")):
			isCode = !isCode
			continue
		case !isCode && rs[i] == '*' && i+1 < len(rs) && rs[i+1] == '*' && (isBold || closes(i+2, "**")):
			isBold = !isBold
			i++
			continue
		}
		st := base
		if isBold {
			st += bold
		}
		if isCode {
			st += code
		}
		s.rs = append(s.rs, rs[i])
		s.st = append(s.st, st)
	}
	return s
}

// lines breaks s into lines of at most w columns, at spaces where it can.
func (s styled) lines(w int) []styled {
	w = max(w, 1)
	var out []styled
	start, col, space := 0, 0, -1
	for i, r := range s.rs {
		cw := sidebar.DisplayWidth(string(r))
		if col+cw > w && i > start {
			if r == ' ' { // the line ends right here
				out = append(out, styled{s.rs[start:i], s.st[start:i]})
				start, space, col = i+1, -1, 0
				continue
			}
			end, next := i, i
			if space > start {
				end, next = space, space+1
			}
			out = append(out, styled{s.rs[start:end], s.st[start:end]})
			start, space, col = next, -1, sidebar.DisplayWidth(string(s.rs[next:i]))
		}
		if r == ' ' {
			space = i
		}
		col += cw
	}
	return append(out, styled{s.rs[start:], s.st[start:]})
}

// String draws s with its styles.
func (s styled) String() string {
	var b strings.Builder
	cur := ""
	for i, r := range s.rs {
		if s.st[i] != cur {
			b.WriteString(reset + s.st[i])
			cur = s.st[i]
		}
		b.WriteRune(r)
	}
	if cur != "" {
		b.WriteString(reset)
	}
	return b.String()
}

// block wraps s for w columns, its first line after first and the others
// after rest (drawn indents of the same width).
func (s styled) block(first, rest string, w int) []string {
	lead := sidebar.DisplayWidth(sidebar.Plain(first))
	var out []string
	for i, l := range s.lines(w - 1 - lead) {
		p := first
		if i > 0 {
			p = rest
		}
		out = append(out, p+l.String())
	}
	return out
}

// Run is what the header says about a run.
type Run struct {
	Name     string
	CLI      session.AgentID
	Model    string    // the one chosen, or the one the stream named
	Command  string    // its command line, the prompt as <prompt>
	Start    time.Time // zero while it starts
	End      time.Time // zero while it runs
	Result   *auto.Run // how it ended (nil while it runs)
	Starting bool      // launched, not running yet
	GaveUp   bool      // it did not start
	Tick     int       // the spinner's frame
}

// Header draws a run's header for w columns: its state, name, CLI and
// model, and its time; then its command line and a rule.
func Header(r Run, now time.Time, w int) []string {
	w = max(w, 20)
	who := ""
	if r.CLI != "" {
		who = session.ShortName(r.CLI)
	}
	if r.Model != "" {
		who += " · " + Safe(r.Model)
	}
	mark, state := spin+spinner[r.Tick%len(spinner)]+reset, ""
	switch res := r.Result; {
	case r.GaveUp:
		mark, state = yel+"↷"+reset, yel+i18n.T("auto.view.not_started")+reset
	case res != nil:
		color := green
		switch res.Status {
		case auto.StatusFailed, auto.StatusTimeout:
			color = red
		case auto.StatusQuiet:
			color = muted
		case auto.StatusStopped, auto.StatusSkipped:
			color = yel
		}
		mark = color + res.Status.Mark() + reset
		state = muted + textfmt.Clock(now, res.Start)
		if !res.End.IsZero() {
			state += " · " + i18n.T("auto.view.took", auto.Took(res.Duration()))
		}
		state += reset
		if t := Safe(res.Text()); t != "" && res.Status != auto.StatusOK {
			state += "  " + color + t + reset
		} else if res.Status == auto.StatusOK {
			state += "  " + green + i18n.T("auto.view.done") + reset
		}
	case r.Starting || r.Start.IsZero():
		state = muted + i18n.T("auto.view.starting") + reset
	default:
		state = muted + i18n.T("auto.running", auto.Took(now.Sub(r.Start))) + reset
	}
	line := " " + mark + " " + bold + Safe(r.Name) + reset
	if who != "" {
		line += muted + " · " + who + reset
	}
	out := []string{cut(line+"   "+state, w)}
	if r.Command != "" {
		out = append(out, cut(" "+faint+"$ "+strings.ReplaceAll(Safe(r.Command), "\n", " ")+reset, w))
	}
	return append(out, faint+strings.Repeat("─", w)+reset)
}

// Hint is the bottom row while the run goes on, and after.
func Hint(running bool) string {
	if running {
		return i18n.T("auto.view.hint_running")
	}
	return i18n.T("auto.view.hint_done")
}

// cut cuts a drawn line to w columns, with "…" when it is longer.
func cut(s string, w int) string {
	if sidebar.DisplayWidth(sidebar.Plain(s)) <= w {
		return s
	}
	return sidebar.Cut(s, 0, w-1) + reset + "…"
}

// wrap breaks plain text into lines of at most w columns, at spaces where
// it can.
func wrap(s string, w int) []string {
	var t styled
	t.add(s, "")
	var out []string
	for _, l := range t.lines(w) {
		out = append(out, string(l.rs))
	}
	return out
}
