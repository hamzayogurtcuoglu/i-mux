package sidebar

import (
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// The Source Control view like VS Code's (§9.3 "Source Control like VS
// Code", D58).

// Global options of the view (`imux __scm set`).
const (
	OptSCMView = "@imux_scm_view" // "tree", or "" for a list
	OptSCMSort = "@imux_scm_sort" // "name", "status", or "" for the path
)

// Fold names of the Source Control view (in @imux_fold).
const (
	SecMerge    = "merge"
	SecStaged   = "staged"
	SecUnstaged = "unstaged"
	SecHistory  = "history"
)

// Sides of a changed file: the menus' and the actions'.
const (
	sideStaged   = "staged"
	sideWorktree = "worktree"
	sideMerge    = "merge"
)

// scmAction is `imux __scm verb name` (a menu, a Quick Pick, a prompt).
func scmAction(verb, name string) Action { return Action{Kind: ActSCM, Section: verb, Path: name} }

// scmLetter is a change's letter as VS Code shows it: U for an untracked
// file, ! for a conflict.
func scmLetter(c gitops.Change, side string) string {
	switch {
	case side == sideMerge:
		return "!"
	case c.Untracked:
		return "U"
	}
	return c.Status
}

// scmButtons are a changed file's (or a folder's) buttons on its side.
func scmButtons(p string, untracked bool, side string, ic Icons, dir bool) []Cell {
	switch side {
	case sideStaged:
		return []Cell{button(ic.Remove, muted, Action{Kind: ActGit, Section: "unstage", Path: p})}
	case sideMerge:
		return []Cell{button(ic.Add, muted, Action{Kind: ActGit, Section: "stage", Path: p})}
	}
	st := ""
	if untracked {
		st = "?"
	}
	discard := Action{Kind: ActGitConfirm, Section: "discard", Path: p, Status: st}
	if dir {
		discard = Action{Kind: ActGitConfirm, Section: "discard-dir", Path: p}
	}
	return []Cell{button(ic.Discard, muted, discard), button(ic.Add, muted, Action{Kind: ActGit, Section: "stage", Path: p})}
}

// scmRow is one changed file with its buttons: a list's row (the name, then
// its folder in gray) or a tree's (name set, indent levels deep).
func scmRow(c gitops.Change, side string, w int, ic Icons, name string, indent int) Line {
	cr := ChangeRow{Status: c.Status, Path: c.Path, Added: c.Added, Removed: c.Removed, Letter: scmLetter(c, side), Name: name, Indent: indent}
	bs := scmButtons(c.Path, c.Untracked, side, ic, false)
	inner := changeRow(cr, w-width(bs))
	main := Cell{Text: inner.Text, Action: Action{Kind: ActDiff, Path: c.Path, Status: c.Status, Section: side}}
	return buttons(w, main, bs...)
}

// sortChanges orders a side's changes as View & Sort says: by path (the
// default), name or status.
func sortChanges(cs []gitops.Change, by string, side string) []gitops.Change {
	out := append([]gitops.Change(nil), cs...)
	switch by {
	default:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	case "name":
		sort.SliceStable(out, func(i, j int) bool {
			a, b := path.Base(out[i].Path), path.Base(out[j].Path)
			if a != b {
				return a < b
			}
			return out[i].Path < out[j].Path
		})
	case "status":
		sort.SliceStable(out, func(i, j int) bool {
			a, b := scmLetter(out[i], side), scmLetter(out[j], side)
			if a != b {
				return a < b
			}
			return out[i].Path < out[j].Path
		})
	}
	return out
}

// scmNode is a folder of the tree view.
type scmNode struct {
	name  string
	path  string // from the project folder; "" for it
	dirs  map[string]*scmNode
	files []gitops.Change
}

func scmTree(cs []gitops.Change) *scmNode {
	root := &scmNode{dirs: map[string]*scmNode{}}
	for _, c := range cs {
		n := root
		parts := strings.Split(strings.TrimSuffix(c.Path, "/"), "/")
		for _, d := range parts[:len(parts)-1] {
			next, ok := n.dirs[d]
			if !ok {
				next = &scmNode{name: d, path: path.Join(n.path, d), dirs: map[string]*scmNode{}}
				n.dirs[d] = next
			}
			n = next
		}
		n.files = append(n.files, c)
	}
	return root
}

// treeRows draws n's folders (joined while a folder has only a folder in
// it, as VS Code's compact folders) then its files, depth levels deep.
func (m Model) treeRows(n *scmNode, side string, depth, w int, ic Icons, rows *[]Line) {
	names := make([]string, 0, len(n.dirs))
	for k := range n.dirs {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		d := n.dirs[k]
		label := d.name
		for len(d.dirs) == 1 && len(d.files) == 0 {
			for _, only := range d.dirs {
				d = only
			}
			label += "/" + d.name
		}
		key := side + ":" + d.path
		closed := m.SCMClosed[key]
		bs := scmButtons(d.path, false, side, ic, true)
		indent := strings.Repeat("  ", min(depth, 8))
		lw := max(w-width(bs)-5-2*min(depth, 8), 1)
		text := "   " + indent + muted + ic.fold(closed) + reset + " " + fitMiddle(label, lw)
		*rows = append(*rows, buttons(w, Cell{Text: text, Action: Action{Kind: ActSCMDir, Path: key}}, bs...))
		if !closed {
			m.treeRows(d, side, depth+1, w, ic, rows)
		}
	}
	for _, c := range sortChanges(n.files, m.SCMSort, side) {
		name := path.Base(strings.TrimSuffix(c.Path, "/"))
		if strings.HasSuffix(c.Path, "/") {
			name += "/"
		}
		*rows = append(*rows, scmRow(c, side, w, ic, name, depth))
	}
}

// changesRows are a side's rows: a list, or a tree (View & Sort).
func (m Model) changesRows(cs []gitops.Change, side string, w int, ic Icons) []Line {
	var rows []Line
	if m.SCMView == "tree" {
		m.treeRows(scmTree(cs), side, 0, w, ic, &rows)
		return rows
	}
	for _, c := range sortChanges(cs, m.SCMSort, side) {
		rows = append(rows, scmRow(c, side, w, ic, "", 0))
	}
	return rows
}

// laneColors draws a graph row's lanes: the commit's ● in cyan, the lines
// faint.
func laneColors(lanes string) string {
	return faint + strings.ReplaceAll(lanes, "●", cyan+"●"+reset+faint) + reset
}

// maxLanes bounds the graph's width.
const maxLanes = 8

// graphRow is one row of HISTORY: lanes, ↑ (not pushed) or ↓ (incoming),
// the hash (from 40 columns), the refs, the subject and its age.
func graphRow(r gitops.GraphRow, now time.Time, w int) Line {
	lanes := fit(r.Lanes, min(DisplayWidth(r.Lanes), maxLanes))
	if r.Hash == "" {
		return Line{Text: "   " + laneColors(lanes) + strings.Repeat(" ", max(w-3-DisplayWidth(lanes), 0))}
	}
	mark := " "
	switch {
	case r.Out:
		mark = green + "↑" + reset
	case r.In:
		mark = cyan + "↓" + reset
	}
	when := age(now, r.When)
	text := "   " + laneColors(lanes) + " " + mark
	used := 3 + DisplayWidth(lanes) + 2 + 1 + runes(when) + 1
	if w >= 40 {
		text += " " + yellow + fit(r.Hash, 7) + reset
		used += 8
	}
	rest := max(w-used, 1)
	// The refs that fit in two thirds of the row, the subject after them.
	refs, rw := "", 0
	var colored []string
	for _, f := range r.Refs {
		n := DisplayWidth(f.Name) + 2 // ", " or "(" and ") "
		if rw+n+1 > rest*2/3 {
			break
		}
		colored = append(colored, refColor(f.Kind)+f.Name+reset)
		rw += n
	}
	if len(colored) > 0 {
		refs = "(" + strings.Join(colored, ", ") + ") "
		rest -= rw + 1
	}
	text += " " + refs + fit(r.Subject, max(rest, 1)) + " " + muted + when + reset
	return Line{Text: text, Action: Action{Kind: ActGitPopup, Section: "show", Path: r.Hash}}
}

func refColor(kind string) string {
	switch kind {
	case gitops.RefHead:
		return bold + cyan
	case gitops.RefBranch:
		return cyan
	case gitops.RefTag:
		return yellow
	}
	return faint
}

// wideButton is a full-width button row (Commit, Initialize Repository);
// a label longer than the row is cut.
func wideButton(label string, w int, a Action) Cell {
	if DisplayWidth(label) > w-2 {
		label = fit(label, max(w-2, 1))
	}
	pad := max(w-2-DisplayWidth(label), 0)
	return Cell{
		W:      w,
		Text:   " " + "\x1b[48;5;24m\x1b[38;5;255m" + strings.Repeat(" ", pad/2) + label + strings.Repeat(" ", pad-pad/2) + reset + " ",
		Action: a,
	}
}

// commitButton is the commit button's label and action: Commit (the
// commit box's message), Commit Merge (the box's message, else git's), or
// Continue during a rebase, cherry-pick or revert.
func commitButton(m Model) (string, Action) {
	ic := m.icons()
	label, act := ic.Commit+" "+i18n.T("git.commit_button"), Action{Kind: ActCommit}
	if m.SCM == nil {
		return label, act
	}
	switch m.SCM.Op {
	case gitops.OpMerge:
		label = ic.Commit + " " + i18n.T("scm.commit_merge")
		if m.Commit.Empty() {
			act = Action{Kind: ActGitPopup, Section: "continue"}
		}
	case gitops.OpRebase, gitops.OpCherryPick, gitops.OpRevert:
		label, act = ic.Commit+" "+i18n.T("scm.continue"), Action{Kind: ActGitPopup, Section: "continue"}
	}
	return label, act
}

// renderChanges is the Source Control view, like VS Code's (D27, D58).
func renderChanges(m Model, w, h int) ([]Line, ListView) {
	ic := m.icons()
	refresh := button(ic.Refresh, muted, Action{Kind: ActRefresh})
	more := button(ic.More, muted, scmAction("menu", "more"))
	st := m.SCM
	if st == nil {
		out := []Line{
			buttons(w, Cell{Text: title + fit(" "+i18n.T("sidebar.source_control"), w-refresh.W) + reset}, refresh),
			{Text: muted + fit("   "+i18n.T("sidebar.no_git"), w) + reset},
		}
		if m.CWD != "" {
			initRepo := wideButton(i18n.T("scm.init_repo"), w, Action{Kind: ActGit, Section: "init"})
			clone := wideButton(i18n.T("scm.clone_repo"), w, scmAction("ask", "clone"))
			out = append(out, Line{},
				Line{Text: compose([]Cell{initRepo}, w), Cells: []Cell{initRepo}},
				Line{Text: compose([]Cell{clone}, w), Cells: []Cell{clone}})
		}
		return out, ListView{}
	}
	out := []Line{buttons(w, Cell{Text: title + fit(" "+i18n.T("sidebar.source_control"), w-refresh.W-more.W) + reset}, refresh, more)}

	// Branch (a click: Checkout to…), ahead / behind, pull, push, sync or
	// publish.
	branch := st.Branch
	if branch == "" {
		branch = i18n.T("git.detached")
	}
	sync := ""
	if st.Upstream != "" {
		sync = "↑" + strconv.Itoa(st.Ahead) + " ↓" + strconv.Itoa(st.Behind)
	}
	last := button(ic.Sync, cyan, Action{Kind: ActGitPopup, Section: "sync"})
	if st.Upstream == "" && st.Branch != "" {
		last = button(ic.Publish, cyan, scmAction("pick", "publish"))
	}
	syncBtns := []Cell{
		button(ic.Pull, muted, Action{Kind: ActGitPopup, Section: "pull"}),
		button(ic.Push, muted, Action{Kind: ActGitPopup, Section: "push"}),
		last,
	}
	nw := max(w-width(syncBtns)-(DisplayWidth(ic.Branch)+2)-1-runes(sync), 4)
	left := " " + cyan + ic.Branch + reset + " " + fit(branch+" "+ic.Open, nw) + " " + muted + sync + reset
	out = append(out, buttons(w, Cell{Text: left, Action: scmAction("pick", "checkout")}, syncBtns...))

	// The commit message box (D65), then the commit button (Continue /
	// Commit Merge during an operation) and ▾, its dropdown.
	rows := maxBoxRows
	if h > 0 {
		rows = min(rows, max(1, (h-8)/3))
	}
	out = append(out, m.commitBoxRows(w, rows)...)
	label, act := commitButton(m)
	drop := button(ic.Open, muted, scmAction("menu", "commit-button"))
	commit := wideButton(label, w-drop.W, act)
	drop.X = commit.W
	cells := []Cell{commit, drop}
	out = append(out, Line{Text: compose(cells, w), Cells: cells})

	n := 0
	if h > 0 {
		n = h - len(out)
	}
	return withList(out, m.scmRows(w), m.Scroll, n, w)
}

// scmRows are the view's scrolling rows: MERGE CHANGES, STAGED CHANGES,
// CHANGES and HISTORY.
func (m Model) scmRows(w int) []Line {
	st, ic := m.SCM, m.icons()
	if st == nil {
		return nil
	}
	folded := func(n string) bool { return m.Fold[n] }
	var rows []Line
	section := func(name, text string, n int, btns ...Cell) {
		head := Cell{
			Text:   " " + muted + ic.fold(folded(name)) + reset + " " + title + fit(text+"  "+strconv.Itoa(n), w-3-width(btns)) + reset,
			Action: Action{Kind: ActFold, Section: name},
		}
		rows = append(rows, buttons(w, head, btns...))
	}
	if len(st.Conflicts) > 0 {
		section(SecMerge, i18n.T("scm.merge_changes"), len(st.Conflicts), button(ic.Add, muted, Action{Kind: ActGit, Section: "stage-merge"}))
		if !folded(SecMerge) {
			rows = append(rows, m.changesRows(st.Conflicts, sideMerge, w, ic)...)
		}
	}
	if len(st.Staged) > 0 {
		section(SecStaged, i18n.T("git.staged"), len(st.Staged), button(ic.Remove, muted, Action{Kind: ActGit, Section: "unstage-all"}))
		if !folded(SecStaged) {
			rows = append(rows, m.changesRows(st.Staged, sideStaged, w, ic)...)
		}
	}
	section(SecUnstaged, i18n.T("git.changes"), len(st.Changes),
		button(ic.Discard, muted, Action{Kind: ActGitConfirm, Section: "discard-all"}), button(ic.Add, muted, Action{Kind: ActGit, Section: "stage-all"}))
	if !folded(SecUnstaged) {
		if len(st.Changes) == 0 {
			rows = append(rows, Line{Text: muted + fit("   "+i18n.T("sidebar.no_changes"), w) + reset})
		}
		rows = append(rows, m.changesRows(st.Changes, sideWorktree, w, ic)...)
	}
	commits := 0
	for _, r := range st.Graph {
		if r.Hash != "" {
			commits++
		}
	}
	if commits > 0 {
		section(SecHistory, i18n.T("git.history"), commits)
		if !folded(SecHistory) {
			for _, r := range st.Graph {
				rows = append(rows, graphRow(r, m.Now, w))
			}
		}
	}
	return rows
}
