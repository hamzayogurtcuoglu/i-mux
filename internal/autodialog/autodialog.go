// Package autodialog is the Scheduler's Run panel (§9.3 "Scheduler",
// D61): one popup that shows, chosen already, which agent CLI runs an agent
// or a skill, with which model, when (now, or on a schedule) and with what
// access; Enter runs it. It only asks: `imux __auto` saves the choice,
// starts the run and shows its output.
package autodialog

import (
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
)

// Colors from the theme (D68), set again when it changes.
var cyan, yellow, muted, faint, selBg, keptBg string

func init() {
	theme.Watch(func(p theme.Palette) {
		s := theme.StylesOf(p)
		cyan = s.Accent
		yellow = s.Yellow
		muted = s.Muted
		faint = s.Faint
		selBg = s.Selected
		keptBg = s.Pill
	})
}

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	// The chosen chip of the row being changed, and of the other rows.
)

// CLI is an agent CLI the panel offers (an installed one).
type CLI struct {
	ID      session.AgentID
	Name    string // "Claude"
	Key     string // its letter
	Limited string // "15:00" while its usage limit lasts
}

// Options fill the panel.
type Options struct {
	Name        string
	Description string
	Project     string // the project's folder name
	Skill       bool   // a skill (else an agent)
	CLIs        []CLI  // the installed ones, the definition's own first
	CLI         session.AgentID
	Model       string          // "": the default
	AgentModel  string          // the agent's own model (its front matter)
	Owner       session.AgentID // the CLI the agent is written for: AgentModel is its default
	When        string          // its schedule (stored form), "" none
	AtWhen      bool            // open at its time (Change schedule…), else at Now
	ReadOnly    bool
	Used        map[session.AgentID][]string // models used before, per CLI
	Created     time.Time                    // when its schedule was made (zero: none yet)
	LastStart   time.Time                    // its last run's start (zero: never)
	Now         time.Time
}

// Result is the panel's answer.
type Result struct {
	Run      bool // false: closed
	CLI      session.AgentID
	Model    string // "": the default
	When     string // "": now, once; else the stored form
	ReadOnly bool
}

// Rows of the panel.
const (
	rowCLI = iota
	rowModel
	rowWhen
	rowAccess
	rowButtons
	rows
)

// chip is one choice of a row.
type chip struct {
	label  string // drawn (colors allowed)
	value  string
	custom bool // the one typed into
}

// hit is a place a click chooses.
type hit struct{ y, x0, x1, row, chip int }

// Model is the panel's state.
type Model struct {
	o        Options
	row      int
	cli      int
	model    string // the chosen model when not typed
	whenV    string // the chosen time when not typed
	typing   [rows]bool
	text     [rows][]rune
	readOnly bool
	button   int // 0: Run, 1: Cancel
	err      string
	models   map[session.AgentID][]string
	asked    map[session.AgentID]bool
	from     [rows]int // the first chip shown of a row too long
	hits     []hit
}

// New makes the panel for o.
func New(o Options) *Model {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	m := &Model{o: o, model: o.Model, readOnly: o.ReadOnly, models: map[session.AgentID][]string{}, asked: map[session.AgentID]bool{}}
	for i, c := range o.CLIs {
		if c.ID == o.CLI {
			m.cli = i
		}
	}
	if o.AtWhen {
		m.whenV = o.When
		m.row = rowWhen
	}
	return m
}

func (m *Model) cliID() session.AgentID {
	if len(m.o.CLIs) == 0 {
		return ""
	}
	return m.o.CLIs[m.cli].ID
}

// Pending is the CLI whose models are to be asked for (once each).
func (m *Model) Pending() []session.AgentID {
	id := m.cliID()
	if id == "" || m.asked[id] {
		return nil
	}
	m.asked[id] = true
	return []session.AgentID{id}
}

// SetModels gives a CLI's models.
func (m *Model) SetModels(id session.AgentID, ms []string) {
	if ms == nil {
		ms = []string{}
	}
	m.models[id] = ms
}

// chips are a row's choices.
func (m *Model) chips(row int) []chip {
	var cs []chip
	switch row {
	case rowCLI:
		for _, c := range m.o.CLIs {
			sym, color := sidebar.AgentIcon(c.ID)
			cs = append(cs, chip{label: color + sym + reset + " " + c.Name, value: string(c.ID)})
		}
	case rowModel:
		cs = append(cs, chip{label: i18n.T("auto.panel.default")})
		seen := map[string]bool{"": true}
		id := m.cliID()
		for _, list := range [][]string{m.o.Used[id], m.models[id], {m.model}} {
			for _, v := range list {
				if !seen[v] {
					seen[v] = true
					cs = append(cs, chip{label: v, value: v})
				}
			}
		}
		cs = append(cs, chip{custom: true})
	case rowWhen:
		cs = append(cs, chip{label: i18n.T("auto.panel.now")})
		seen := map[string]bool{"": true}
		for _, v := range []string{"5m", "15m", "hourly", "09:00", m.o.When, m.whenV} {
			if w, err := auto.ParseWhen(v); err == nil && !seen[w.String()] {
				seen[w.String()] = true
				cs = append(cs, chip{label: w.Label(), value: w.String()})
			}
		}
		cs = append(cs, chip{custom: true})
	case rowAccess:
		cs = []chip{{label: i18n.T("auto.panel.full"), value: auto.AccessFull}, {label: i18n.T("auto.panel.read"), value: auto.AccessRead}}
	}
	return cs
}

// sel is the chosen chip of a row.
func (m *Model) sel(row int) int {
	switch row {
	case rowCLI:
		return m.cli
	case rowAccess:
		if m.readOnly {
			return 1
		}
		return 0
	case rowModel, rowWhen:
		cs := m.chips(row)
		if m.typing[row] {
			return len(cs) - 1
		}
		v := m.model
		if row == rowWhen {
			v = m.whenV
		}
		for i, c := range cs {
			if !c.custom && c.value == v {
				return i
			}
		}
	}
	return 0
}

// choose chooses chip i of a row.
func (m *Model) choose(row, i int) {
	cs := m.chips(row)
	if len(cs) == 0 {
		return
	}
	i = min(max(i, 0), len(cs)-1)
	switch row {
	case rowCLI:
		m.cli = i
	case rowAccess:
		m.readOnly = i == 1
	case rowModel, rowWhen:
		m.typing[row] = cs[i].custom
		if !cs[i].custom {
			if row == rowModel {
				m.model = cs[i].value
			} else {
				m.whenV = cs[i].value
			}
		}
	}
}

// Key applies a key; it reports whether the panel ends.
func (m *Model) Key(k string) (Result, bool) {
	m.err = ""
	switch k {
	case "esc", "ctrl-c":
		return Result{}, true
	case "enter":
		return m.submit()
	case "up", "backtab":
		m.row = (m.row + rows - 1) % rows
	case "down", "tab":
		m.row = (m.row + 1) % rows
	case "left", "right":
		d := 1
		if k == "left" {
			d = -1
		}
		if m.row == rowButtons {
			m.button = min(max(m.button+d, 0), 1)
			break
		}
		m.choose(m.row, m.sel(m.row)+d)
	case "home":
		m.choose(m.row, 0)
	case "end":
		m.choose(m.row, len(m.chips(m.row))-1)
	case "backspace":
		if t := m.text[m.row]; m.typing[m.row] && len(t) > 0 {
			m.text[m.row] = t[:len(t)-1]
		}
	case "ctrl-u":
		m.text[m.row] = nil
	default:
		r, size := utf8.DecodeRuneInString(k)
		if size != len(k) || r < ' ' {
			break
		}
		switch m.row {
		case rowModel, rowWhen:
			if r == ' ' && m.row == rowModel {
				break
			}
			if !m.typing[m.row] {
				m.typing[m.row], m.text[m.row] = true, nil
			}
			m.text[m.row] = append(m.text[m.row], r)
		case rowCLI:
			for i, c := range m.o.CLIs {
				if c.Key == k {
					m.cli = i
				}
			}
		}
	}
	return Result{}, false
}

// Paste types pasted text into a typed row.
func (m *Model) Paste(s string) {
	for _, r := range strings.TrimSpace(s) {
		if r != '\n' && r != '\r' {
			m.Key(string(r))
		}
	}
}

// Click chooses what is at x, y (1-based): a chip, or a button.
func (m *Model) Click(x, y int) (Result, bool) {
	for _, h := range m.hits {
		if y != h.y || x < h.x0 || x >= h.x1 {
			continue
		}
		m.err = ""
		m.row = h.row
		if h.row == rowButtons {
			m.button = h.chip
			if h.chip == 1 {
				return Result{}, true
			}
			return m.submit()
		}
		m.choose(h.row, h.chip)
		return Result{}, false
	}
	return Result{}, false
}

// chosen is the panel's choice as it stands, or what is wrong with it and
// on which row.
func (m *Model) chosen() (Result, string, int) {
	if len(m.o.CLIs) == 0 {
		return Result{}, i18n.T("auto.panel.no_cli"), rowCLI
	}
	r := Result{Run: true, CLI: m.cliID(), Model: m.model, When: m.whenV, ReadOnly: m.readOnly}
	if m.typing[rowModel] {
		r.Model = strings.TrimSpace(string(m.text[rowModel]))
	}
	if strings.ContainsAny(r.Model, " \t") {
		return r, i18n.T("auto.bad_model", r.Model), rowModel
	}
	if m.typing[rowWhen] {
		typed := strings.TrimSpace(string(m.text[rowWhen]))
		w, err := auto.ParseWhen(typed)
		switch {
		case err == auto.ErrTooOften:
			return r, i18n.T("auto.too_often"), rowWhen
		case err != nil || typed == "":
			return r, i18n.T("auto.bad_when", typed), rowWhen
		}
		r.When = w.String()
	}
	return r, "", 0
}

func (m *Model) submit() (Result, bool) {
	if m.row == rowButtons && m.button == 1 {
		return Result{}, true
	}
	r, err, row := m.chosen()
	if err != "" {
		m.err, m.row = err, row
		return Result{}, false
	}
	return r, true
}

// summary says what Enter does.
func (m *Model) summary() string {
	r, err, _ := m.chosen()
	if err != "" {
		return ""
	}
	c := m.o.CLIs[m.cli]
	who := c.Name
	switch {
	case r.Model != "":
		who += " · " + r.Model
	case m.o.AgentModel != "" && c.ID == m.o.Owner:
		who += " · " + m.o.AgentModel
	}
	access := i18n.T("auto.panel.full_long")
	if r.ReadOnly {
		access = i18n.T("auto.panel.read_long")
	}
	if r.When == "" {
		return i18n.T("auto.panel.sum_now", who, access, m.o.Project)
	}
	w, _ := auto.ParseWhen(r.When)
	s := i18n.T("auto.panel.sum_every", w.Label(), who, access, m.o.Project)
	created, last := m.o.Created, m.o.LastStart
	if r.When != m.o.When || created.IsZero() {
		created = m.o.Now // a new time counts from now
		if w.Every == 0 {
			last = time.Time{}
		}
	}
	if due, next := w.Due(created, last, m.o.Now); due {
		s += " " + i18n.T("auto.panel.sum_first")
	} else {
		s += " " + i18n.T("auto.panel.sum_next", textfmt.Clock(m.o.Now, next))
	}
	return s
}

// chipText is a chip as drawn, and its width.
func (m *Model) chipText(row int, c chip, chosen, focused bool) (string, int) {
	label := c.label
	if c.custom {
		switch {
		case m.typing[row]:
			t := string(m.text[row])
			ph := ""
			if t == "" {
				ph = faint + i18n.T("auto.panel.type_model")
				if row == rowWhen {
					ph = faint + i18n.T("auto.panel.type_when")
				}
			}
			cursor := ""
			if focused {
				cursor = "▏"
			}
			label = "✎ " + t + cursor + ph
		default:
			label = "✎ " + i18n.T("auto.panel.other")
		}
	}
	plain := sidebar.Plain(label)
	w := sidebar.DisplayWidth(plain) + 2
	switch {
	case chosen && focused:
		return selBg + " " + plain + " " + reset, w
	case chosen:
		return keptBg + " " + plain + " " + reset, w
	case c.custom:
		return " " + faint + plain + reset + " ", w
	}
	return " " + muted + strings.ReplaceAll(label, reset, reset+muted) + reset + " ", w
}

// chipLine draws a row's chips in avail columns from column x0 (1-based)
// of screen row y; a row too long shows a window around its choice.
func (m *Model) chipLine(row, avail, x0, y int) string {
	cs := m.chips(row)
	sel := m.sel(row)
	focused := m.row == row
	texts := make([]string, len(cs))
	widths := make([]int, len(cs))
	total := 0
	for i, c := range cs {
		texts[i], widths[i] = m.chipText(row, c, i == sel, focused)
		total += widths[i]
	}
	from := 0
	if total > avail {
		from = min(m.from[row], sel)
		for from < sel {
			w := 2 // " ›"
			if from > 0 {
				w += 2 // "‹ "
			}
			for i := from; i <= sel; i++ {
				w += widths[i]
			}
			if w <= avail {
				break
			}
			from++
		}
	}
	m.from[row] = from
	var b strings.Builder
	x := x0
	if from > 0 {
		b.WriteString(faint + "‹ " + reset)
		x += 2
	}
	for i := from; i < len(cs); i++ {
		room := avail - (x - x0)
		if i < len(cs)-1 {
			room -= 2 // " ›"
		}
		if widths[i] > room {
			b.WriteString(faint + " ›" + reset)
			break
		}
		b.WriteString(texts[i])
		m.hits = append(m.hits, hit{y: y, x0: x, x1: x + widths[i], row: row, chip: i})
		x += widths[i]
	}
	if row == rowModel && m.asked[m.cliID()] && m.models[m.cliID()] == nil {
		b.WriteString(faint + " …" + reset)
	}
	return b.String()
}

// Render draws the panel for w columns and h rows.
func (m *Model) Render(w, h int) []string {
	w, h = max(w, 30), max(h, 12)
	m.hits = nil
	kind := i18n.T("auto.panel.agent")
	if m.o.Skill {
		kind = i18n.T("auto.panel.skill")
	}
	head := " 🚀 " + bold + m.o.Name + reset + muted + "  · " + kind
	if m.o.Project != "" {
		head += " · " + m.o.Project
	}
	out := []string{sidebar.Cut(head+reset, 0, w)}
	if d := strings.TrimSpace(m.o.Description); d != "" {
		out = append(out, " "+muted+sidebar.Fit(d, w-2)+reset)
	}
	out = append(out, faint+strings.Repeat("─", w)+reset, "")
	labels := [...]string{i18n.T("auto.panel.cli"), i18n.T("auto.panel.model"), i18n.T("auto.panel.when"), i18n.T("auto.panel.access")}
	lw := 0
	for _, l := range labels {
		lw = max(lw, sidebar.DisplayWidth(l))
	}
	x0 := 1 + 1 + 2 + lw + 2 // the column the chips start at
	for row := range rowButtons {
		marker, style := "  ", muted
		if m.row == row {
			marker, style = cyan+"▸ "+reset, bold+cyan
		}
		line := " " + marker + style + sidebar.Fit(labels[row], lw) + reset + "  "
		if row == rowCLI && len(m.o.CLIs) == 0 {
			line += yellow + i18n.T("auto.panel.no_cli") + reset
		} else {
			line += m.chipLine(row, w-x0, x0, len(out)+1)
		}
		out = append(out, line)
	}
	out = append(out, "")
	switch {
	case m.err != "":
		out = append(out, " "+yellow+"⚠ "+sidebar.Fit(m.err, w-4)+reset)
	case len(m.o.CLIs) > 0 && m.o.CLIs[m.cli].Limited != "":
		c := m.o.CLIs[m.cli]
		out = append(out, " "+yellow+"⚠ "+sidebar.Fit(i18n.T("auto.panel.limited", c.Name, c.Limited), w-4)+reset)
	default:
		sum := wrap(m.summary(), w-2, 2)
		for len(sum) < 2 {
			sum = append(sum, "")
		}
		for _, l := range sum {
			out = append(out, " "+muted+l+reset)
		}
	}
	out = append(out, "")
	// The buttons, on the right: Run (Schedule for a time) and Cancel.
	primary := i18n.T("auto.panel.run_now")
	if r, err, _ := m.chosen(); err == "" && r.When != "" {
		primary = i18n.T("auto.panel.schedule")
	}
	btns := [2]string{" " + primary + " ", " " + i18n.T("click.cancel") + " "}
	bw := sidebar.DisplayWidth(btns[0]) + 2 + sidebar.DisplayWidth(btns[1])
	x := max(w-bw-1, 0)
	line := strings.Repeat(" ", x)
	y := len(out) + 1
	for i, l := range btns {
		style := muted
		switch {
		case m.row == rowButtons && m.button == i:
			style = selBg
		case i == 0 && m.row != rowButtons:
			style = keptBg
		}
		line += style + l + reset
		lw := sidebar.DisplayWidth(l)
		m.hits = append(m.hits, hit{y: y, x0: x + 1, x1: x + 1 + lw, row: rowButtons, chip: i})
		x += lw
		if i == 0 {
			line += "  "
			x += 2
		}
	}
	out = append(out, line)
	for len(out) < h-1 {
		out = append(out, "")
	}
	return append(out[:h-1], " "+faint+sidebar.Fit(i18n.T("auto.panel.hint"), w-2)+reset)
}

// Height is the rows the panel needs for o.
func Height(o Options) int {
	// The title, its rule, a blank, 4 rows, a blank, 2 for the summary, a
	// blank, the buttons, a blank and the hint.
	h := 15
	if strings.TrimSpace(o.Description) != "" {
		h++
	}
	return h
}

// wrap breaks s into at most n lines of w columns, at spaces.
func wrap(s string, w, n int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case sidebar.DisplayWidth(line)+1+sidebar.DisplayWidth(word) <= w:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	if len(out) > n {
		out = out[:n]
		out[n-1] += " …"
	}
	for i := range out {
		out[i] = sidebar.Fit(out[i], w)
	}
	return out
}

// Run shows the panel on the terminal until it ends; models asks a CLI
// for its models (in the background, once each).
func Run(m *Model, models func(session.AgentID) []string, in, out *os.File) (Result, error) {
	fd := int(in.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return Result{}, err
	}
	defer restore()
	// Alternate screen, no cursor, clicks, and pastes in one piece.
	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h\x1b[?2004h")
	defer func() { _, _ = io.WriteString(out, "\x1b[?2004l\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	type loaded struct {
		id session.AgentID
		ms []string
	}
	got := make(chan loaded, 8)
	ask := func() {
		for _, id := range m.Pending() {
			go func(id session.AgentID) { got <- loaded{id, models(id)} }(id)
		}
	}
	last := ""
	draw := func() {
		w, h := sidebar.Size(int(out.Fd()))
		var sc sidebar.Screen
		for _, l := range m.Render(w, h) {
			sc.Lines = append(sc.Lines, sidebar.Line{Text: l})
		}
		if f := sidebar.Frame(sc, w, h); f != last {
			last = f
			_, _ = io.WriteString(out, f)
		}
	}
	var pending []byte
	ask()
	draw()
	for {
		b, err := sidebar.ReadInput(fd, 50*time.Millisecond)
		if err != nil {
			return Result{}, nil // the popup went away
		}
		select {
		case <-winch:
			last = ""
			draw()
		case l := <-got:
			m.SetModels(l.id, l.ms)
			draw()
		default:
		}
		if b == nil {
			continue
		}
		var evs []sidebar.Event
		evs, pending = sidebar.Parse(append(pending, b...), true)
		for _, ev := range evs {
			var res Result
			done := false
			switch ev.Kind {
			case sidebar.EvKey:
				res, done = m.Key(ev.Key)
			case sidebar.EvPaste:
				m.Paste(ev.Text)
			case sidebar.EvClick:
				if !sidebar.EarlyClick() {
					res, done = m.Click(ev.X, ev.Y)
				}
			}
			if done {
				return res, nil
			}
		}
		ask()
		draw()
	}
}
