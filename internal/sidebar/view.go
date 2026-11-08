package sidebar

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
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
	// OptClosed is "1" while the panel is closed (§9.3 "Closing the side
	// panel"): the sidebar is only the rail.
	OptClosed = "@imux_collapsed"
)

// railW is the activity rail's width: an icon with room around it and a
// short label under it (6 columns), then the separator line.
const railW = 7

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
func treeRow(r TreeRow, w int, ic Icons) Line {
	indent := strings.Repeat("  ", min(r.Depth, 8))
	color := gitColor(r.Git)
	mark := "  "
	a := Action{Kind: ActOpenFile, Path: r.Path}
	if r.Dir {
		mark = muted + ic.fold(!r.Open) + " " + reset
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
	search := button(m.icons().Search, muted, Action{Kind: ActSearch})
	out := []Line{buttons(w, Cell{Text: title + fit(" "+i18n.T("sidebar.explorer")+" · "+m.Project, w-search.W) + reset}, search)}
	var rows []Line
	for _, r := range m.Tree {
		rows = append(rows, treeRow(r, w, m.icons()))
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

// Fold names of the Source Control view (in @imux_fold).
const (
	SecStaged   = "staged"
	SecUnstaged = "unstaged"
	SecHistory  = "history"
)

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

// scmRow is one changed file of the Source Control view with its buttons.
func scmRow(c gitops.Change, staged bool, w int, ic Icons) Line {
	cr := ChangeRow{Status: c.Status, Path: c.Path, Added: c.Added, Removed: c.Removed}
	side := "worktree"
	var bs []Cell
	if staged {
		side = "staged"
		bs = append(bs, button(ic.Remove, muted, Action{Kind: ActGit, Section: "unstage", Path: c.Path}))
	} else {
		st := ""
		if c.Untracked {
			st = "?"
		}
		bs = append(bs, button(ic.Discard, muted, Action{Kind: ActGitConfirm, Section: "discard", Path: c.Path, Status: st}),
			button(ic.Add, muted, Action{Kind: ActGit, Section: "stage", Path: c.Path}))
	}
	inner := changeRow(cr, w-width(bs))
	main := Cell{Text: inner.Text, Action: Action{Kind: ActDiff, Path: c.Path, Status: c.Status, Section: side}}
	return buttons(w, main, bs...)
}

// renderChanges is the Source Control view, like an editor's (D27).
func renderChanges(m Model, w, h int) []Line {
	st := m.SCM
	if st == nil {
		return []Line{
			{Text: title + fit(" "+i18n.T("sidebar.source_control"), w) + reset},
			{Text: muted + fit("   "+i18n.T("sidebar.no_git"), w) + reset},
		}
	}
	folded := func(n string) bool { return m.Fold[n] }
	ic := m.icons()
	refresh := button(ic.Refresh, muted, Action{Kind: ActRefresh})
	out := []Line{buttons(w, Cell{Text: title + fit(" "+i18n.T("sidebar.source_control"), w-refresh.W) + reset}, refresh)}

	// Branch, ahead / behind, sync.
	branch := st.Branch
	if branch == "" {
		branch = i18n.T("git.detached")
	}
	sync := ""
	if st.Upstream != "" {
		sync = "↑" + strconv.Itoa(st.Ahead) + " ↓" + strconv.Itoa(st.Behind)
	}
	syncBtns := []Cell{
		button(ic.Pull, muted, Action{Kind: ActGitPopup, Section: "pull"}),
		button(ic.Push, muted, Action{Kind: ActGitPopup, Section: "push"}),
		button(ic.Sync, cyan, Action{Kind: ActGitPopup, Section: "sync"}),
	}
	// " ⎇ " name " " sync, left of the three buttons.
	nw := max(w-width(syncBtns)-(DisplayWidth(ic.Branch)+2)-1-runes(sync), 4)
	left := " " + cyan + ic.Branch + reset + " " + fit(branch+" "+ic.Open, nw) + " " + muted + sync + reset
	out = append(out, buttons(w, Cell{Text: left, Action: Action{Kind: ActBranchMenu}}, syncBtns...))

	// Commit button, and ✨: an agent writes the message (D29).
	suggest := button(ic.Suggest, cyan, Action{Kind: ActSuggest})
	label := ic.Commit + " " + i18n.T("git.commit_button")
	cw := w - suggest.W
	pad := max(cw-2-DisplayWidth(label), 0)
	commit := Cell{
		W:      cw,
		Text:   " " + "\x1b[48;5;24m\x1b[38;5;255m" + strings.Repeat(" ", pad/2) + label + strings.Repeat(" ", pad-pad/2) + reset + " ",
		Action: Action{Kind: ActCommit},
	}
	suggest.X = cw
	out = append(out, Line{Text: compose([]Cell{commit, suggest}, w), Cells: []Cell{commit, suggest}})

	var rows []Line
	section := func(name, text string, n int, btns ...Cell) {
		arrow := ic.fold(folded(name))
		head := Cell{
			Text:   " " + muted + arrow + reset + " " + title + fit(text+"  "+strconv.Itoa(n), w-3-width(btns)) + reset,
			Action: Action{Kind: ActFold, Section: name},
		}
		rows = append(rows, buttons(w, head, btns...))
	}
	if len(st.Staged) > 0 {
		section(SecStaged, i18n.T("git.staged"), len(st.Staged), button(ic.Remove, muted, Action{Kind: ActGit, Section: "unstage-all"}))
		if !folded(SecStaged) {
			for _, c := range st.Staged {
				rows = append(rows, scmRow(c, true, w, ic))
			}
		}
	}
	section(SecUnstaged, i18n.T("git.changes"), len(st.Changes),
		button(ic.Discard, muted, Action{Kind: ActGitConfirm, Section: "discard-all"}), button(ic.Add, muted, Action{Kind: ActGit, Section: "stage-all"}))
	if !folded(SecUnstaged) {
		if len(st.Changes) == 0 {
			rows = append(rows, Line{Text: muted + fit("   "+i18n.T("sidebar.no_changes"), w) + reset})
		}
		for _, c := range st.Changes {
			rows = append(rows, scmRow(c, false, w, ic))
		}
	}
	if len(st.History) > 0 {
		section(SecHistory, i18n.T("git.history"), len(st.History))
		if !folded(SecHistory) {
			for _, c := range st.History {
				when := age(m.Now, c.When)
				sw := max(w-12-runes(when), 1) // "   " hash " " subject " " when
				rows = append(rows, Line{
					Text:   "   " + yellow + fit(c.Hash, 7) + reset + " " + fit(c.Subject, sw) + " " + muted + when + reset,
					Action: Action{Kind: ActGitPopup, Section: "show", Path: c.Hash},
				})
			}
		}
	}
	n := 0
	if h > 0 {
		n = h - len(out)
	}
	return append(out, scrolled(rows, m.Scroll, n)...)
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
// limits, help. Nerd Font codicons (v3): hubot, files, search,
// source_control, dashboard, question.
var railIcons = map[string][6]string{
	IconsNerd:  {"\ueb08", "\ueaf0", "\uea6d", "\uea68", "\ueacd", "\ueb32"},
	IconsEmoji: {"🤖", "📁", "🔍", "🔀", "📊", "❓"},
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
	}, []railItem{
		{icon: ic[4], label: "rail.limits", action: Action{Kind: ActLimits}},
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
	const iw = railW - 1 // the item's columns; the last one is the line
	r := []rune(text)
	if len(r) > iw {
		r = r[:iw]
	}
	pad := iw - len(r)
	lt := strings.Repeat(" ", pad/2) + string(r) + strings.Repeat(" ", pad-pad/2)
	// "  X   ": an emoji takes 2 columns, a Nerd Font icon 1 (drawn wider).
	gap := "  "
	if DisplayWidth(it.icon) == 1 {
		gap = "   "
	}
	iconColor := ""
	if !active {
		iconColor = muted
	}
	line, cw := faint+"│"+reset, railW
	if m.Closed {
		line, cw = "", ClosedWidth
	}
	if active {
		return Cell{X: 0, W: cw, Text: selBg + cyan + "▌" + reset + selBg + " " + bold + it.icon + reset + selBg + gap + reset + line, Action: a},
			Cell{X: 0, W: cw, Text: selBg + bold + lt + reset + line, Action: a}
	}
	return Cell{X: 0, W: cw, Text: "  " + iconColor + it.icon + reset + gap + line, Action: a},
		Cell{X: 0, W: cw, Text: color + lt + reset + line, Action: a}
}

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
