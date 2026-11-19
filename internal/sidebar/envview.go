package sidebar

import (
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/envfile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// EnvRow is one environment in the Env view (D84): its name and how many
// variables it sets; never a value.
type EnvRow struct {
	Name     string
	Vars     int
	Problems int // lines that were skipped
}

// gatherEnvs reads the environments for the Env view.
func gatherEnvs() []EnvRow {
	var out []EnvRow
	for _, e := range envfile.List() {
		out = append(out, EnvRow{Name: e.Name, Vars: len(e.Vars), Problems: len(e.Problems)})
	}
	return out
}

// renderEnv is the Env view (D84): ENV and + (a new environment); a row
// per environment, its number of variables on the right, or "● this pane"
// for the target pane's. A click applies it to the target pane (on the
// target's own, its menu); a right-click opens its menu.
func renderEnv(m Model, w, h int) ([]Line, ListView) {
	ic := m.icons()
	add := button(ic.Add, muted, Action{Kind: ActEnvNew})
	add.Tip = i18n.T("env.add_tip")
	head, style := " "+i18n.T("sidebar.env"), title
	if m.Tip != "" {
		head, style = " "+m.Tip, tipStyle
	}
	out := []Line{buttons(w, Cell{Text: style + fit(head, w-add.W) + reset}, add)}
	var rows []Line
	if len(m.Envs) == 0 {
		rows = []Line{{Text: strings.Repeat(" ", w)}}
		for _, l := range wrapWords(i18n.T("env.empty"), w-3) {
			rows = append(rows, Line{Text: muted + fit("  "+l, w) + reset})
		}
		rows = append(rows, Line{Text: strings.Repeat(" ", w)},
			Line{Text: cyan + fit("  "+ic.Add+" "+i18n.T("env.add_row"), w) + reset, Action: Action{Kind: ActEnvNew}})
	}
	for _, e := range m.Envs {
		rows = append(rows, envRow(e, e.Name == m.TargetEnv && m.Target != "", w))
	}
	if len(m.Envs) > 0 && m.Target != "" {
		rows = append(rows, Line{Text: strings.Repeat(" ", w)})
		hint := i18n.T("env.hint")
		if m.TargetEnv != "" {
			hint = i18n.T("env.hint_on", m.TargetEnv)
		}
		for _, l := range wrapWords(hint, w-3) {
			rows = append(rows, Line{Text: muted + fit("  "+l, w) + reset})
		}
	}
	n := 0
	if h > 0 {
		n = h - len(out)
	}
	return withList(out, rows, m.Scroll, n, w)
}

// envRow is an environment's row: 🔑 and its name (bold when it is the
// target pane's), ⚠ when lines were skipped, and on the right its number
// of variables or "● this pane".
func envRow(e EnvRow, here bool, w int) Line {
	right, color := strconv.Itoa(e.Vars), muted
	if here {
		right, color = "● "+i18n.T("env.this_pane"), green
	}
	rw := runes(right)
	if rw > w/2 {
		right, rw = string([]rune(right)[:1]), 1 // the dot alone in a narrow sidebar
	}
	warn, ww := "", 0
	if e.Problems > 0 {
		warn, ww = " "+yellow+"⚠"+reset, 2
	}
	const lead = 1 + 2 + 1 // " 🔑 "
	nw := max(w-lead-ww-rw-1, 1)
	name := fit(e.Name, min(runes(e.Name), nw))
	if here {
		name = bold + name + reset
	}
	left := " 🔑 " + name + warn
	a := Action{Kind: ActEnvApply, Path: e.Name}
	if here {
		a = Action{Kind: ActEnvMenu, Path: e.Name}
	}
	return Line{Text: spread(left, lead+min(runes(e.Name), nw)+ww, color+right+reset+" ", rw+1, w), Action: a}
}
