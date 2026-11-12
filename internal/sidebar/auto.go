package sidebar

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
)

// The Scheduler view (§9.3 "Scheduler", D61): the project's agents and
// skills, the ones with a schedule first with their last run; ⏰ schedules
// one, 🚀 runs it now. `imux __auto` does the rest.

// Fold names of the Scheduler view (in @imux_fold).
const (
	SecAutoScheduled = "auto-scheduled"
	SecAutoAgents    = "auto-agents"
	SecAutoSkills    = "auto-skills"
)

// AutoItem is one definition of the project (an agent or a skill) and its
// schedule.
type AutoItem struct {
	Def     auto.Def
	Sched   *auto.Schedule // nil: it has none
	Gone    bool           // its definition is not there any more
	Last    *auto.Run      // its newest run
	Running bool
	Since   time.Time // when the run going on started
	Next    time.Time // when it runs next (zero: by hand, or paused)
}

// AutoItems are the definitions of the project at root (personal agents
// from home) with their schedules in ss; a schedule whose definition went
// is listed too, to remove it.
func AutoItems(root, home string, ss []auto.Schedule, now time.Time) []AutoItem {
	mine := map[string]auto.Schedule{}
	var order []string
	for _, s := range ss {
		if s.Project == root {
			mine[s.ID] = s
			order = append(order, s.ID)
		}
	}
	var items []AutoItem
	seen := map[string]bool{}
	for _, d := range auto.List(root, home) {
		it := AutoItem{Def: d}
		if s, ok := mine[d.ID]; ok {
			it.Sched = &s
		}
		seen[d.ID] = true
		items = append(items, it)
	}
	for _, id := range order {
		if !seen[id] {
			s := mine[id]
			items = append(items, AutoItem{Def: auto.Def{ID: id, Kind: s.Kind, Name: s.Name, File: s.File}, Sched: &s, Gone: true})
		}
	}
	for i := range items {
		it := &items[i]
		s := it.Sched
		if s == nil {
			continue // runs need a schedule
		}
		it.Last = auto.Last(s.ID)
		if c, ok := auto.Running(s.ID); ok {
			it.Running, it.Since = true, c.Start
		}
		if w := s.Parsed(); !s.Paused && !it.Gone && !w.Manual() {
			_, it.Next = w.Due(s.Created, auto.LastStart(s.ID), now)
		}
	}
	return items
}

// AutoRunning counts the runs of ss going on (the rail's badge).
func AutoRunning(ss []auto.Schedule) int {
	n := 0
	for _, s := range ss {
		if _, ok := auto.Running(s.ID); ok {
			n++
		}
	}
	return n
}

// gatherAuto reads the Scheduler's state: the runs going on, and in its
// view the project's definitions.
func gatherAuto(m *Model, cwd string, root func(string) string) {
	ss, _ := auto.Load()
	m.AutoRunning = AutoRunning(ss)
	if m.View == ViewAuto && cwd != "" {
		home, _ := os.UserHomeDir()
		m.AutoRoot = root(cwd)
		m.Auto = AutoItems(m.AutoRoot, home, ss, m.Now)
	}
}

// autoIcon is an item's icon in two columns: the glyph of the CLI that
// runs it (or that it is written for), else a skill's or an agent's.
func autoIcon(it AutoItem, ic Icons) string {
	cli := it.Def.Owner
	if it.Sched != nil && it.Sched.CLI != "" {
		cli = session.AgentID(it.Sched.CLI)
	}
	g := ""
	switch {
	case cli != "":
		sym, color := icon(cli)
		g = color + sym + reset
	case it.Def.Kind == auto.KindSkill:
		g = muted + ic.Skill + reset
	default:
		g = muted + ic.Robot + reset
	}
	if DisplayWidth(Plain(g)) < 2 {
		g += " "
	}
	return g
}

// statusColor is the color of a run's mark.
func statusColor(st auto.Status) string {
	switch st {
	case auto.StatusOK:
		return green
	case auto.StatusFailed, auto.StatusTimeout:
		return red
	case auto.StatusStopped, auto.StatusSkipped:
		return yellow
	}
	return muted
}

// autoName is an item's first row: its icon and name (a personal agent
// says so when there is room), then its buttons; a click on it does click.
func autoName(it AutoItem, w int, ic Icons, style string, click Action, btns ...Cell) Line {
	nw := max(w-width(btns)-6, 1) // "   " icon " " name
	name := style + fit(it.Def.Name, nw) + reset
	if t := i18n.T("auto.personal"); it.Def.Personal && DisplayWidth(it.Def.Name)+2+DisplayWidth(t) <= nw {
		name = style + it.Def.Name + reset + "  " + faint + t + reset + strings.Repeat(" ", nw-DisplayWidth(it.Def.Name)-2-DisplayWidth(t))
	}
	return buttons(w, Cell{Text: "   " + autoIcon(it, ic) + " " + name, Action: click}, btns...)
}

// scheduledRows draw a scheduled item: its name with 🚀 (🛑 while it runs)
// and ⋯; its time, CLI and model; its run going on or its last run, and
// when it runs next. A click shows its last output.
func (m Model) scheduledRows(it AutoItem, w int, ic Icons) []Line {
	id, s := it.Def.ID, it.Sched
	click := Action{Kind: ActAuto, Section: "log", Path: id}
	if it.Last == nil && !it.Running {
		click = Action{Kind: ActAutoMenu, Path: id}
	}
	more := button(ic.More, muted, Action{Kind: ActAutoMenu, Path: id})
	btns := []Cell{button(ic.Run, cyan, Action{Kind: ActAuto, Section: "run", Path: id}), more}
	switch {
	case it.Running:
		btns[0] = button(ic.Stop, red, Action{Kind: ActAuto, Section: "stop", Path: id})
	case it.Gone:
		btns = btns[1:]
	}
	style := bold
	if it.Gone || s.Paused {
		style = muted
	}
	out := []Line{autoName(it, w, ic, style, click, btns...)}

	parts := []string{s.Parsed().Label()}
	if s.CLI != "" {
		p := session.ShortName(session.AgentID(s.CLI))
		if s.Model != "" {
			p += " · " + s.Model
		}
		parts = append(parts, p)
	}
	if s.ReadOnly() {
		parts = append(parts, i18n.T("auto.read_only"))
	}
	if s.Paused {
		parts = append(parts, i18n.T("auto.paused_row"))
	}
	if it.Gone {
		parts = []string{i18n.T("auto.gone_row")}
	}
	// Its time, CLI and model, and when it runs next on the right.
	right, rw := "", 0
	if !it.Next.IsZero() && !it.Running {
		nx := "→ " + textfmt.Clock(m.Now, it.Next)
		if !it.Next.After(m.Now) {
			nx = "→ " + i18n.T("auto.now")
		}
		if w-6-DisplayWidth(nx)-1 >= 8 {
			right, rw = " "+muted+nx+reset, DisplayWidth(nx)+1
		}
	}
	out = append(out, Line{Text: "     " + muted + fit(strings.Join(parts, " · "), max(w-6-rw, 1)) + reset + right + " ", Action: click})
	if it.Gone && it.Last == nil {
		return out
	}

	// The run going on, or the last one: its news.
	room := w - 6
	var body string
	switch r := it.Last; {
	case it.Running:
		body = spinFg + spinner[m.Tick%len(spinner)] + " " + fit(i18n.T("auto.running", auto.Took(m.Now.Sub(it.Since))), max(room-2, 1)) + reset
	case r != nil:
		clock := textfmt.Clock(m.Now, r.Start)
		textColor := ""
		switch r.Status {
		case auto.StatusFailed, auto.StatusTimeout:
			textColor = red
		case auto.StatusQuiet, auto.StatusStopped, auto.StatusSkipped:
			textColor = muted
		}
		body = statusColor(r.Status) + r.Status.Mark() + reset + " " + muted + clock + reset + " " + textColor + fit(r.Text(), max(room-3-DisplayWidth(clock), 1)) + reset
	default:
		body = muted + fit(i18n.T("auto.never"), max(room, 1)) + reset
	}
	return append(out, Line{Text: "     " + body + " ", Action: click})
}

// defRows draw a definition without a schedule: its name with 🚀 and ⏰,
// and its description. A click opens it.
func (m Model) defRows(it AutoItem, w int, ic Icons) []Line {
	id := it.Def.ID
	open := Action{Kind: ActAuto, Section: "open", Path: id}
	out := []Line{autoName(it, w, ic, "", open,
		button(ic.Run, cyan, Action{Kind: ActAuto, Section: "run", Path: id}),
		button(ic.Schedule, cyan, Action{Kind: ActAuto, Section: "menu when", Path: id}))}
	if d := it.Def.Description; d != "" {
		out = append(out, Line{Text: "     " + muted + fit(d, max(w-6, 1)) + reset + " ", Action: open})
	}
	return out
}

// autoRows are the view's scrolling rows: SCHEDULED, AGENTS and SKILLS.
func (m Model) autoRows(w int) []Line {
	ic := m.icons()
	var sched, agents, sks []AutoItem
	for _, it := range m.Auto {
		switch {
		case it.Sched != nil:
			sched = append(sched, it)
		case it.Def.Kind == auto.KindSkill:
			sks = append(sks, it)
		default:
			agents = append(agents, it)
		}
	}
	var rows []Line
	section := func(name, text string, n int) bool {
		rows = append(rows, Line{
			Text:   " " + muted + ic.fold(m.Fold[name]) + reset + " " + title + fit(text+"  "+strconv.Itoa(n), w-3) + reset,
			Action: Action{Kind: ActFold, Section: name},
		})
		return !m.Fold[name]
	}
	if len(m.Auto) == 0 {
		return []Line{
			{Text: muted + fit("   "+i18n.T("auto.empty"), w) + reset},
			{Text: faint + fit("   .claude/agents/<name>.md", w) + reset},
			{Text: cyan + fit("   "+i18n.T("auto.empty_hint"), w) + reset, Action: Action{Kind: ActAutoNew}},
		}
	}
	if len(sched) == 0 {
		rows = append(rows, Line{Text: faint + fit("   "+i18n.T("auto.hint", ic.Schedule, ic.Run), w) + reset})
	}
	if len(sched) > 0 && section(SecAutoScheduled, i18n.T("auto.sec_scheduled"), len(sched)) {
		for _, it := range sched {
			rows = append(rows, m.scheduledRows(it, w, ic)...)
		}
	}
	if len(agents) > 0 && section(SecAutoAgents, i18n.T("auto.sec_agents"), len(agents)) {
		for _, it := range agents {
			rows = append(rows, m.defRows(it, w, ic)...)
		}
	}
	if len(sks) > 0 && section(SecAutoSkills, i18n.T("auto.sec_skills"), len(sks)) {
		for _, it := range sks {
			rows = append(rows, m.defRows(it, w, ic)...)
		}
	}
	return rows
}

// renderAuto is the Scheduler view: its title with ✨ New agent… and
// refresh, then its rows.
func renderAuto(m Model, w, h int) []Line {
	ic := m.icons()
	refresh := button(ic.Refresh, muted, Action{Kind: ActRefresh})
	add := button(ic.Suggest, cyan, Action{Kind: ActAutoNew})
	head := i18n.T("sidebar.scheduler")
	if m.Project != "" {
		head += " · " + m.Project
	}
	out := []Line{buttons(w, Cell{Text: title + fit(" "+head, w-refresh.W-add.W) + reset}, add, refresh)}
	n := 0
	if h > 0 {
		n = h - len(out)
	}
	return append(out, scrolled(m.autoRows(w), m.Scroll, n, w)...)
}

// autoRunning reports a run going on in the view (its spinner turns).
func (m Model) autoRunning() bool {
	if m.View != ViewAuto || m.Closed {
		return false
	}
	for _, it := range m.Auto {
		if it.Running {
			return true
		}
	}
	return false
}

// autoArgs is `imux __auto words [id]` for the project, the sidebar, the
// client and the pane the sidebar follows (D61).
func (s *sidebar) autoArgs(client string, words []string, id string, extra ...string) []string {
	args := append([]string{"__auto"}, words...)
	if id != "" {
		args = append(args, id)
	}
	args = append(args, "--cwd", s.model.AutoRoot, "--sidebar", s.o.Pane)
	if client != "" {
		args = append(args, "--client", client)
	}
	if s.model.Target != "" {
		args = append(args, "--pane", s.model.Target)
	}
	return append(args, extra...)
}

// autoDo runs a Scheduler action: `imux __auto`, its menus where the click
// was; ✨ opens the New agent dialog.
func (s *sidebar) autoDo(ctx context.Context, client string, a Action) {
	var at []string
	if s.menuX > 0 && s.menuY > 0 {
		at = []string{"--x", strconv.Itoa(s.menuX), "--y", strconv.Itoa(s.menuY)}
	}
	switch a.Kind {
	case ActAutoNew:
		s.skillStart(ctx, client, "dialog", "--agents")
	case ActAutoMenu:
		if a.Path == "" {
			s.spawn(s.autoArgs(client, []string{"menu", "root"}, "", at...))
			return
		}
		s.spawn(s.autoArgs(client, []string{"menu", "item"}, a.Path, at...))
	case ActAuto:
		words := strings.Fields(a.Section)
		if len(words) == 0 || words[0] != "menu" && words[0] != "run" {
			at = nil
		}
		s.spawn(s.autoArgs(client, words, a.Path, at...))
	}
}
