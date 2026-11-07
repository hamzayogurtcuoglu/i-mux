package sidebar

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// Views of the sidebar (§9.3 "Sidebar v6"), chosen on the activity rail and
// kept in the window option @imux_view.
const (
	ViewAgents   = "agents"
	ViewExplorer = "explorer"
	ViewSearch   = "search" // a popup: the rail opens it, the view stays
	ViewChanges  = "changes"
	OptView      = "@imux_view"
)

// railW is the activity rail's width.
const railW = 3

// TreeRow is one entry of the explorer.
type TreeRow struct {
	Path  string // relative to the project folder
	Name  string
	Depth int
	Dir   bool
	Open  bool   // a folder shown open
	Git   string // a file's git status ("M", "A", "?" ...); "*" on a folder with changes
}

// maxTree bounds the explorer rows.
const maxTree = 3000

// BuildTree lists root as a tree: folders first, then files, by name; the
// folders in open are listed inside. Hidden entries and the skip list are
// left out; git marks files and the folders above them.
func BuildTree(root string, open map[string]bool, changes []ChangeRow) []TreeRow {
	status := map[string]string{}
	dirty := map[string]bool{}
	for _, c := range changes {
		p := filepath.Clean(strings.TrimSuffix(c.Path, "/"))
		status[p] = c.Status
		for d := filepath.Dir(p); d != "." && d != "/" && d != ".."; d = filepath.Dir(d) {
			dirty[d] = true
		}
		if strings.HasSuffix(c.Path, "/") { // an untracked folder
			dirty[p] = true
		}
	}
	var out []TreeRow
	var walk func(rel string, depth int)
	walk = func(rel string, depth int) {
		ents, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			return
		}
		sort.SliceStable(ents, func(i, j int) bool {
			di, dj := ents[i].IsDir(), ents[j].IsDir()
			if di != dj {
				return di
			}
			return strings.ToLower(ents[i].Name()) < strings.ToLower(ents[j].Name())
		})
		for _, e := range ents {
			if len(out) >= maxTree {
				return
			}
			n := e.Name()
			if strings.HasPrefix(n, ".") || e.IsDir() && folders.Skip[n] {
				continue
			}
			p := filepath.Join(rel, n)
			row := TreeRow{Path: p, Name: n, Depth: depth, Dir: e.IsDir(), Git: status[p]}
			if row.Dir {
				row.Open = open[p]
				if dirty[p] {
					row.Git = "*"
				}
			}
			out = append(out, row)
			if row.Dir && row.Open {
				walk(p, depth+1)
			}
		}
	}
	walk("", 0)
	return out
}

// gitColor is the color of a git status in the explorer.
func gitColor(st string) string {
	switch st {
	case "":
		return ""
	case "A", "?":
		return green
	case "D":
		return red
	}
	return yellow
}

// treeRow draws one explorer entry.
func treeRow(r TreeRow, w int) Line {
	indent := strings.Repeat("  ", min(r.Depth, 8))
	color := gitColor(r.Git)
	mark := "  "
	a := Action{Kind: ActOpenFile, Path: r.Path}
	if r.Dir {
		mark = muted + "▸ " + reset
		if r.Open {
			mark = muted + "▾ " + reset
		}
		a = Action{Kind: ActToggleDir, Path: r.Path}
	}
	right, rw := "", 0
	if r.Git != "" && !r.Dir {
		right, rw = color+r.Git+reset, 1
	}
	// " " indent mark name (" " status | "  ") " "
	tail := 2
	if rw > 0 {
		tail = 1 + rw
	}
	nw := max(w-1-runes(indent)-2-tail-1, 1)
	name := color + fit(r.Name, nw) + reset
	text := " " + indent + mark + name
	if rw > 0 {
		text += " " + right
	} else {
		text += "  "
	}
	return Line{Text: text + " ", Action: a}
}

// scrolled keeps rows [scroll, scroll+n) of list (scroll clamped).
func scrolled(list []Line, scroll, n int) []Line {
	if n <= 0 || len(list) <= n {
		return list
	}
	scroll = min(max(scroll, 0), len(list)-n)
	return list[scroll : scroll+n]
}

// renderExplorer is the Explorer view: a title with the search button and
// the project's tree.
func renderExplorer(m Model, w, h int) []Line {
	cells := []Cell{{X: 0, W: w - 3, Text: title + fit(" "+i18n.T("sidebar.explorer")+" · "+m.Project, w-3) + reset}}
	cells = append(cells, Cell{X: w - 3, W: 3, Text: muted + " ⌕ " + reset, Action: Action{Kind: ActSearch}})
	out := []Line{{Text: compose(cells, w), Cells: cells}}
	var rows []Line
	for _, r := range m.Tree {
		rows = append(rows, treeRow(r, w))
	}
	if len(rows) == 0 {
		rows = append(rows, Line{Text: muted + fit("   "+i18n.T("sidebar.empty_folder"), w) + reset})
	}
	n := 0
	if h > 0 {
		n = h - 1
	}
	return append(out, scrolled(rows, m.Scroll, n)...)
}

// renderChanges is the Source Control view: every changed file.
func renderChanges(m Model, w, h int) []Line {
	if !m.Git {
		return []Line{
			{Text: title + fit(" "+i18n.T("sidebar.source_control"), w) + reset},
			{Text: muted + fit("   "+i18n.T("sidebar.no_git"), w) + reset},
		}
	}
	text := i18n.T("sidebar.source_control")
	if m.Branch != "" {
		text += " · " + m.Branch
	}
	count := ""
	if m.Changed > 0 {
		count = strconv.Itoa(m.Changed) + " "
	}
	l, r := text, count
	out := []Line{{Text: " " + title + fit(l, w-1-runes(r)) + reset + muted + r + reset}}
	var rows []Line
	for _, c := range m.Changes {
		rows = append(rows, changeRow(c, w))
	}
	if more := m.Changed - len(m.Changes); more > 0 {
		rows = append(rows, Line{Text: muted + fit("   "+i18n.T("sidebar.more_files", more), w) + reset, Action: Action{Kind: ActChanges}})
	}
	if len(rows) == 0 {
		rows = append(rows, Line{Text: muted + fit("   "+i18n.T("sidebar.no_changes"), w) + reset})
	}
	n := 0
	if h > 0 {
		n = h - 1
	}
	return append(out, scrolled(rows, m.Scroll, n)...)
}

// railItem is one icon of the activity rail.
type railItem struct {
	icon   string
	view   string // the view it shows ("" for an action)
	action Action
}

// railCell draws a rail icon (3 columns).
func railCell(it railItem, m Model) Cell {
	cur := m.View
	if cur == "" {
		cur = ViewAgents
	}
	active := it.view != "" && it.view == cur
	icon := muted + it.icon + reset
	third := " "
	if it.view == ViewChanges && m.Changed > 0 {
		icon = yellow + it.icon + reset
		if m.Changed < 10 {
			third = yellow + strconv.Itoa(m.Changed) + reset
		} else {
			third = yellow + "+" + reset
		}
	}
	text := " " + icon + third
	if active {
		text = selBg + cyan + "▌" + reset + selBg + bold + it.icon + reset + selBg + " " + reset
		if it.view == ViewChanges && m.Changed > 0 {
			text = selBg + cyan + "▌" + reset + selBg + yellow + it.icon + reset + selBg + third + reset
		}
	}
	a := it.action
	if it.view != "" {
		a = Action{Kind: ActView, Section: it.view}
	}
	return Cell{X: 0, W: railW, Text: text, Action: a}
}

// withRail puts the activity rail on the left of the view's lines: the views
// at the top, Limits and Help at the bottom.
func withRail(content []Line, m Model, w, h int) []Line {
	top := []railItem{
		{icon: "✻", view: ViewAgents},
		{icon: "▤", view: ViewExplorer},
		{icon: "⌕", action: Action{Kind: ActSearch}},
		{icon: "⑂", view: ViewChanges},
	}
	bottom := []railItem{{icon: "◔", action: Action{Kind: ActLimits}}, {icon: "?", action: Action{Kind: ActKeys}}}
	rows := len(content)
	if h > 0 {
		rows = h
	}
	at := map[int]railItem{}
	for i, it := range top {
		at[i*2] = it
	}
	bottomAt := rows - 3
	if h == 0 {
		bottomAt = max(rows, len(top)*2+1)
	}
	for i, it := range bottom {
		at[bottomAt+i*2] = it
	}
	n := max(rows, bottomAt+len(bottom)*2-1)
	out := make([]Line, 0, n)
	for i := 0; i < n; i++ {
		var c Line
		if i < len(content) {
			c = content[i]
		} else {
			c = Line{Text: strings.Repeat(" ", w-railW)}
		}
		var cells []Cell
		if it, ok := at[i]; ok {
			cells = append(cells, railCell(it, m))
		} else {
			cells = append(cells, Cell{X: 0, W: railW, Text: strings.Repeat(" ", railW)})
		}
		if len(c.Cells) == 0 {
			cells = append(cells, Cell{X: railW, W: w - railW, Text: c.Text, Action: c.Action})
		} else {
			for _, cc := range c.Cells {
				cc.X += railW
				cells = append(cells, cc)
			}
		}
		out = append(out, Line{Text: compose(cells, w), Cells: cells})
	}
	return out
}
