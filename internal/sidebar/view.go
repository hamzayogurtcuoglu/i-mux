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
	ViewAuto     = "schedule" // the Scheduler: the project's agents and skills on a schedule (D61)
	OptView      = "@imux_view"
	// OptClosed is "1" while the panel is closed (§9.3 "Closing the side
	// panel"): the sidebar is only the rail.
	OptClosed = "@imux_collapsed"
)

// railW is the activity rail's width: an icon with room around it and a
// short label under it (6 columns), then the separator line.
const railW = 9

// ClosedWidth is the sidebar's width while the panel is closed: the rail's
// item without its line (the pane border is the line).
const ClosedWidth = railW - 1

// railTall is the height from which the rail shows labels.
const railTall = 26

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
// folders in open are listed inside. Hidden entries (unless showHidden),
// the always hidden ones and the skip list are left out (folders.Hidden);
// git marks files and the folders above them.
func BuildTree(root string, open map[string]bool, changes []ChangeRow, showHidden bool) []TreeRow {
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
			if folders.Hidden(n, e.IsDir(), showHidden) {
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

// treeRow draws one explorer entry: the indent guides, a folder's arrow,
// the icon of its type (D35) and the name in its git color; the selected
// one on the selection's background.
func treeRow(r TreeRow, w int, ic Icons, theme string, selected bool) Line {
	depth := min(r.Depth, 8)
	guides := ""
	if depth > 0 {
		guides = faint + strings.Repeat("│ ", depth) + reset
	}
	color := gitColor(r.Git)
	mark := "  "
	a := Action{Kind: ActOpenFile, Path: r.Path}
	if r.Dir {
		mark = muted + ic.fold(!r.Open) + " " + reset
		a = Action{Kind: ActToggleDir, Path: r.Path}
	}
	icon, iw := iconOf(r.Name, r.Dir, r.Open).draw(theme)
	right, rw := "", 0
	if r.Git != "" && !r.Dir {
		right, rw = color+r.Git+reset, 1
	}
	// " " guides mark icon " " name (" " status | "  ") " "
	tail := 2
	if rw > 0 {
		tail = 1 + rw
	}
	nw := max(w-1-2*depth-2-iw-1-tail-1, 1)
	name := color + fit(r.Name, nw) + reset
	text := " " + guides + mark + icon + " " + name
	if rw > 0 {
		text += " " + right
	} else {
		text += "  "
	}
	text += " "
	if selected {
		text = onBg(text, selectBg)
	}
	return Line{Text: text, Action: a}
}

// withList puts rows [scroll, scroll+n) of list under head (scrolled) and
// says which part of it shows.
func withList(head, list []Line, scroll, n, w int) ([]Line, ListView) {
	lv := ListView{Start: len(head), Len: len(list)}
	if n > 0 && len(list) > n {
		lv.N, lv.Top = n, min(max(scroll, 0), len(list)-n)
	}
	return append(head, scrolled(list, scroll, n, w)...), lv
}

// scrolled keeps rows [scroll, scroll+n) of list (scroll clamped), with a
// scroll bar in the last of the w columns when the list does not fit: a
// click on it scrolls there (§9.3 "Scroll bars").
func scrolled(list []Line, scroll, n, w int) []Line {
	if n <= 0 || len(list) <= n {
		return list
	}
	scroll = min(max(scroll, 0), len(list)-n)
	bar := ScrollBar(len(list), scroll, n)
	out := make([]Line, n)
	for i, l := range list[scroll : scroll+n] {
		cells := []Cell{{X: 0, W: w - 1, Text: Cut(l.Text, 0, w-1), Action: l.Action}}
		if len(l.Cells) > 0 {
			cells = nil
			for _, c := range l.Cells {
				if c.X < w-1 {
					if c.X+c.W > w-1 {
						c.W = w - 1 - c.X
						c.Text = Cut(c.Text, 0, c.W)
					}
					cells = append(cells, c)
				}
			}
		}
		cells = append(cells, Cell{X: w - 1, W: 1, Text: bar[i], Action: Action{Kind: ActScroll, Section: strconv.Itoa(ScrollTo(len(list), n, i))}})
		out[i] = Line{Text: compose(cells, w), Action: l.Action, Cells: cells}
	}
	return out
}

// The explorer title's text keeps at least titleMin columns beside VS
// Code's buttons, and titleNav beside Go to File too (§9.3 "Explorer title
// and hidden files").
const (
	titleMin = 6
	titleNav = 16
)

// The explorer title's VS Code buttons, in their order: each is an `imux
// __file` operation too, which ⋯ lists when the button does not fit.
const (
	OpNewFile   = "new-file"
	OpNewFolder = "new-folder"
	OpRefresh   = "refresh"
	OpCollapse  = "collapse"
)

// explorerButtons are the explorer title's buttons for w columns, left to
// right: Go to File while the title keeps titleNav columns, VS Code's (New
// File…, New Folder…, Refresh Explorer, Collapse Folders) while it keeps
// titleMin, and ⋯. more lists the VS Code buttons left to ⋯. Search has no
// button here: the rail opens it.
func explorerButtons(ic Icons, w int) (btns []Cell, more []string) {
	tip := func(c Cell, label string) Cell {
		c.Tip = label
		return c
	}
	dots := tip(button(ic.More, muted, Action{Kind: ActFileMenu, Section: MenuMore}), i18n.T("file.more"))
	used := dots.W
	for _, b := range []struct {
		op string
		c  Cell
	}{
		{OpNewFile, tip(button(ic.NewFile, muted, Action{Kind: ActFileOp, Section: OpNewFile}), i18n.T("file.new_file"))},
		{OpNewFolder, tip(button(ic.NewFolder, muted, Action{Kind: ActFileOp, Section: OpNewFolder}), i18n.T("file.new_folder"))},
		{OpRefresh, tip(button(ic.Refresh, muted, Action{Kind: ActRefresh}), i18n.T("file.refresh"))},
		{OpCollapse, tip(button(ic.Collapse, muted, Action{Kind: ActCollapse}), i18n.T("file.collapse"))},
	} {
		if len(more) > 0 || w-used-b.c.W < titleMin {
			more = append(more, b.op) // the ones after it go too: the order stays
			continue
		}
		btns = append(btns, b.c)
		used += b.c.W
	}
	gotoFile := tip(button(ic.GoTo, muted, Action{Kind: ActGoto}), i18n.T("file.goto"))
	if len(more) == 0 && w-used-gotoFile.W >= titleNav {
		btns = append([]Cell{gotoFile}, btns...)
	}
	return append(btns, dots), more
}

// renderExplorer is the Explorer view: a title with the project, VS Code's
// buttons and ⋯ (the label of the button under the pointer in its place
// while it is there, D63), then the project's tree.
func renderExplorer(m Model, w, h int) ([]Line, ListView) {
	btns, _ := explorerButtons(m.icons(), w)
	text, style := " "+i18n.T("sidebar.explorer")+" · "+m.Project, title
	if m.Tip != "" {
		text, style = " "+m.Tip, tipStyle
	}
	out := []Line{buttons(w, Cell{Text: style + fit(text, w-width(btns)) + reset}, btns...)}
	var rows []Line
	for _, r := range m.Tree {
		rows = append(rows, treeRow(r, w, m.icons(), m.Icons, r.Path == m.Selected))
	}
	if len(rows) == 0 {
		rows = append(rows, Line{Text: muted + fit("   "+i18n.T("sidebar.empty_folder"), w) + reset})
	}
	n := 0
	if h > 0 {
		n = h - 1
	}
	return withList(out, rows, m.Scroll, n, w)
}

// buttons lays out a row: a main cell (the rest of the width) and the
// buttons on the right.
func buttons(w int, main Cell, btns ...Cell) Line {
	bw := 0
	for _, b := range btns {
		bw += b.W
	}
	main.X, main.W = 0, w-bw
	cells := []Cell{main}
	x := main.W
	for _, b := range btns {
		b.X = x
		cells = append(cells, b)
		x += b.W
	}
	return Line{Text: compose(cells, w), Cells: cells}
}

// railItem is one icon of the activity rail.
type railItem struct {
	icon   string // an emoji: 2 columns wide
	label  string // i18n key of its label
	view   string // the view it shows ("" for an action)
	action Action
}

// Icon themes of the rail.
const (
	IconsNerd  = "nerd"  // the editor's icons (codicons) from a Nerd Font
	IconsEmoji = "emoji" // colored emoji: every font has them
)

// railIcons are the rail's icons per theme: agents, files, search, git,
// limits, help, settings, jobs. Nerd Font codicons (v3): hubot, files,
// search, source_control, dashboard, question, settings-gear, watch.
var railIcons = map[string][8]string{
	IconsNerd:  {"\ueb08", "\ueaf0", "\uea6d", "\uea68", "\ueacd", "\ueb32", "\ueb51", "\ueb7c"},
	IconsEmoji: {"🤖", "📁", "🔍", "🔀", "📊", "❓", "🔧", "⏰"},
}

// railItems are the rail's icons: the views at the top, then the bottom
// buttons.
func railItems(theme string) (top, bottom []railItem) {
	ic, ok := railIcons[theme]
	if !ok {
		ic = railIcons[IconsEmoji]
	}
	return []railItem{
		{icon: ic[0], label: "rail.agents", view: ViewAgents},
		{icon: ic[1], label: "rail.files", view: ViewExplorer},
		{icon: ic[2], label: "rail.search", action: Action{Kind: ActSearch}},
		{icon: ic[3], label: "rail.git", view: ViewChanges},
		{icon: ic[7], label: "rail.schedule", view: ViewAuto},
	}, []railItem{
		{icon: ic[4], label: "rail.limits", action: Action{Kind: ActLimits}},
		{icon: ic[6], label: "rail.settings", action: Action{Kind: ActSettings}},
		{icon: ic[5], label: "rail.help", action: Action{Kind: ActKeys}},
	}
}

// railCells draws a rail item: its icon row and its label row.
func railCells(it railItem, m Model) (icon, label Cell) {
	cur := m.View
	if cur == "" {
		cur = ViewAgents
	}
	active := !m.Closed && it.view != "" && it.view == cur
	a := it.action
	if it.view != "" {
		a = Action{Kind: ActView, Section: it.view}
	}
	text := i18n.T(it.label)
	color := muted
	if it.view == ViewChanges && m.Changed > 0 {
		text += " " + strconv.Itoa(min(m.Changed, 99))
		color = yellow
	}
	alert := it.action.Kind == ActSettings && m.Update != "" // a newer i-mux is out
	if alert {
		color = yellow
	}
	const iw = railW - 1 // the item's columns; the last one is the line
	r := []rune(text)
	if len(r) > iw {
		r = r[:iw]
	}
	pad := iw - len(r)
	lt := strings.Repeat(" ", pad/2) + string(r) + strings.Repeat(" ", pad-pad/2)
	// The icon centered as 2 columns: an emoji takes 2, a Nerd Font icon 1
	// (drawn wider, into the next).
	left := (iw - 2) / 2
	gap := strings.Repeat(" ", iw-2-left+2-DisplayWidth(it.icon))
	iconColor := ""
	if !active {
		iconColor = muted
	}
	if alert {
		iconColor = yellow
	}
	bg := ""
	if active {
		bg = selBg
	}
	// The Agents icon's badge (D46): how many agents work, in the item's
	// 7th and 8th columns, one blank after the icon's two. The Schedule
	// icon's: how many scheduled runs go on (D61).
	n := 0
	switch it.view {
	case ViewAgents:
		n = workingCount(m)
	case ViewAuto:
		n = m.AutoRunning
	}
	if n > 0 {
		num := strconv.Itoa(n)
		if n > 9 {
			num = "9+"
		}
		pre := max(badgeAt-left-DisplayWidth(it.icon), 0)
		gap = strings.Repeat(" ", pre) + badgeStyle + num + reset + bg + strings.Repeat(" ", max(iw-badgeAt-len(num), 0))
	}
	line, cw := faint+"│"+reset, railW
	if m.Closed {
		line, cw = "", ClosedWidth
	}
	if active {
		return Cell{X: 0, W: cw, Text: selBg + cyan + "▌" + reset + selBg + strings.Repeat(" ", left-1) + bold + it.icon + reset + selBg + gap + reset + line, Action: a},
			Cell{X: 0, W: cw, Text: selBg + bold + lt + reset + line, Action: a}
	}
	return Cell{X: 0, W: cw, Text: strings.Repeat(" ", left) + iconColor + it.icon + reset + gap + line, Action: a},
		Cell{X: 0, W: cw, Text: color + lt + reset + line, Action: a}
}

// The badge of a rail icon: its column in the item, and its colors (bold
// white on the editor's blue).
const (
	badgeAt    = 6
	badgeStyle = "\x1b[1;38;5;231;48;5;25m"
)

// withRail puts the activity rail on the left of the view's lines: the views
// at the top, Limits and Help at the bottom; labels under the icons when the
// sidebar is tall enough.
func withRail(content []Line, m Model, w, h int) []Line {
	top, bottom := railItems(m.Icons)
	tall := h == 0 || h >= railTall
	step := 2
	if tall {
		step = 3
	}
	rows := len(content)
	if h > 0 {
		rows = h
	}
	at := map[int]Cell{}
	place := func(row int, it railItem) {
		icon, label := railCells(it, m)
		at[row] = icon
		if tall {
			at[row+1] = label
		}
	}
	for i, it := range top {
		place(i*step, it)
	}
	bottomAt := rows - len(bottom)*step + step - 2
	if !tall {
		bottomAt = rows - len(bottom)*step + 1
	}
	if h == 0 {
		bottomAt = max(rows, len(top)*step) + 1
	}
	// A pane too short for both: the bottom items go below the top ones
	// (and off the pane) rather than over them.
	bottomAt = max(bottomAt, len(top)*step)
	for i, it := range bottom {
		place(bottomAt+i*step, it)
	}
	n := rows
	for r := range at {
		n = max(n, r+1)
	}
	if h > 0 {
		n = h
	}
	out := make([]Line, 0, n)
	for i := 0; i < n; i++ {
		cell, ok := at[i]
		if m.Closed {
			// Only the rail; the pane border is its line.
			if !ok {
				cell = Cell{X: 0, W: ClosedWidth, Text: strings.Repeat(" ", ClosedWidth)}
			}
			out = append(out, Line{Text: compose([]Cell{cell}, w), Cells: []Cell{cell}})
			continue
		}
		var c Line
		if i < len(content) {
			c = content[i]
		} else {
			c = Line{Text: strings.Repeat(" ", max(w-railW, 0))}
		}
		if !ok {
			cell = Cell{X: 0, W: railW, Text: strings.Repeat(" ", railW-1) + faint + "│" + reset}
		}
		cells := []Cell{cell}
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
