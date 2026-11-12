// Package scmui builds the Source Control view's menus and Quick Picks as
// VS Code has them (§9.3 "Source Control like VS Code", D58): ⋯ More Actions
// and its submenus, the commit button's dropdown, the file and commit menus,
// and the lists to choose a branch, tag, stash, remote or worktree from.
//
// A menu's commands carry no repository data: they run imux with fixed
// words, the folder is in OptCwd and a chosen value in OptArg. A Quick
// Pick's items are argv, which no shell or tmux parser reads.
package scmui

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/quickpick"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// Global options the menus and prompts pass values through.
const (
	OptCwd    = "@imux_scm_cwd"    // the project folder a menu is about
	OptArg    = "@imux_scm_arg"    // a value chosen in one step for the next: a ref, a remote, a name
	OptCommit = "@imux_scm_commit" // the commit a commit menu's prompt is about (a hash)
	OptView   = "@imux_scm_view"   // "tree", or "" for a list
	OptSort   = "@imux_scm_sort"   // "name", "status", or "" for the path
)

// Ctx is where the menus act.
type Ctx struct {
	Self    string // the imux binary
	Dir     string // the project folder
	Client  string
	Pane    string // the pane the sidebar follows
	Sidebar string // the sidebar's pane
	X, Y    int    // where the menus open in the sidebar (1-based); 0: centered
	Now     time.Time
}

// Flags are the context's flags for imux commands.
func (c Ctx) Flags() []string {
	var a []string
	if c.Client != "" {
		a = append(a, "--client", c.Client)
	}
	if c.Pane != "" {
		a = append(a, "--pane", c.Pane)
	}
	if c.Sidebar != "" {
		a = append(a, "--sidebar", c.Sidebar)
	}
	return a
}

// cmd is a menu item's command: imux with fixed words and the context's
// flags (the folder: OptCwd).
func (c Ctx) cmd(args ...string) string {
	return tmux.RunShell(c.Self, append(args, c.Flags()...)...)
}

// scm is the command `imux __scm args`: a menu, prompt, pick or popup,
// which opens where this menu was (a submenu replaces its menu).
func (c Ctx) scm(args ...string) string {
	a := append(append([]string{"__scm"}, args...), c.Flags()...)
	if c.X > 0 && c.Y > 0 && c.Sidebar != "" {
		a = append(a, "--x", strconv.Itoa(c.X), "--y", strconv.Itoa(c.Y))
	}
	return tmux.RunShell(c.Self, a...)
}

// file is the command `imux __file op` on @imux_file, as the explorer's
// menu runs it.
func (c Ctx) file(op string) string {
	a := []string{"__file", op}
	if c.Client != "" {
		a = append(a, "--client", c.Client)
	}
	if c.Pane != "" {
		a = append(a, "--pane", c.Pane)
	}
	return tmux.RunShell(c.Self, a...)
}

// Argv is a Quick Pick item's argv: words, the folder and the context, then
// data after "--".
func (c Ctx) Argv(words []string, data ...string) []string {
	a := append(append(append([]string{}, words...), "--cwd", c.Dir), c.Flags()...)
	if len(data) > 0 {
		a = append(append(a, "--"), data...)
	}
	return a
}

type item = workspace.MenuItem

func it(key, label, cmd string) item { return item{Name: i18n.T(label), Key: key, Cmd: cmd} }

// sub is an item that opens a submenu.
func sub(key, label, cmd string) item { return item{Name: i18n.T(label) + " ›", Key: key, Cmd: cmd} }

func sep() item { return workspace.Separator() }

func (c Ctx) back() item {
	return item{Name: "‹ " + i18n.T("scm.back"), Key: "BSpace", Cmd: c.scm("menu", "more")}
}

func (c Ctx) menu(name string) string  { return c.scm("menu", name) }
func (c Ctx) ask(op string) string     { return c.scm("ask", op) }
func (c Ctx) pick(name string) string  { return c.scm("pick", name) }
func (c Ctx) popup(op string) string   { return c.scm("popup", op) }
func (c Ctx) confirm(op string) string { return c.scm("confirm", op) }

// Git is the command that runs `imux __git op` on the folder in OptCwd.
func (c Ctx) Git(op string, flags ...string) string {
	return c.cmd(append([]string{"__git", op, "--scm"}, flags...)...)
}

// PopupCmd is the command that opens op's popup (`imux __scm popup op`).
func (c Ctx) PopupCmd(op string, flags ...string) string {
	return c.scm(append([]string{"popup", op}, flags...)...)
}

func disabled(i item, off bool) item {
	i.Disabled = off
	return i
}

// More is ⋯ More Actions.
func More(c Ctx) *workspace.Menu {
	return &workspace.Menu{Title: i18n.T("sidebar.source_control"), Items: []item{
		sub("v", "scm.view_sort", c.menu("view")),
		sep(),
		it("p", "scm.pull", c.popup("pull")),
		it("P", "scm.push", c.popup("push")),
		it("l", "scm.clone", c.ask("clone")),
		it("o", "scm.checkout_to", c.pick("checkout")),
		it("f", "scm.fetch", c.popup("fetch")),
		sep(),
		sub("c", "scm.commit", c.menu("commit")),
		sub("h", "scm.changes", c.menu("changes")),
		sub("u", "scm.pull_push", c.menu("pullpush")),
		sub("b", "scm.branch", c.menu("branch")),
		sub("r", "scm.remote", c.menu("remote")),
		sub("s", "scm.stash", c.menu("stash")),
		sub("t", "scm.tags", c.menu("tags")),
		sub("w", "scm.worktrees", c.menu("worktrees")),
		sep(),
		it("g", "scm.show_output", c.popup("output")),
	}}
}

// ViewSort is View & Sort: ● marks the current view and sort.
func ViewSort(c Ctx, view, sortBy string) *workspace.Menu {
	mark := func(on bool, label string) string {
		if on {
			return "● " + i18n.T(label)
		}
		return "  " + i18n.T(label)
	}
	set := func(opt, v string) string { return c.scm("set", opt, v) }
	return &workspace.Menu{Title: i18n.T("scm.view_sort"), Items: []item{
		c.back(),
		{Name: mark(view != "tree", "scm.view_list"), Key: "l", Cmd: set("view", "list")},
		{Name: mark(view == "tree", "scm.view_tree"), Key: "t", Cmd: set("view", "tree")},
		sep(),
		{Name: mark(sortBy == "name", "scm.sort_name"), Key: "n", Cmd: set("sort", "name")},
		{Name: mark(sortBy != "name" && sortBy != "status", "scm.sort_path"), Key: "p", Cmd: set("sort", "path")},
		{Name: mark(sortBy == "status", "scm.sort_status"), Key: "s", Cmd: set("sort", "status")},
	}}
}

// abortLabel is the Commit menu's Abort item for the operation under way.
var abortLabel = map[string]string{
	gitops.OpMerge: "scm.abort_merge", gitops.OpRebase: "scm.abort_rebase",
	gitops.OpCherryPick: "scm.abort_cherry_pick", gitops.OpRevert: "scm.abort_revert",
}

// Commit is the Commit submenu.
func Commit(c Ctx, st *gitops.Status) *workspace.Menu {
	none := st != nil && st.NoCommits
	items := []item{
		c.back(),
		it("c", "scm.commit_default", c.ask("commit")),
		it("s", "scm.commit_staged", c.ask("commit-staged")),
		it("a", "scm.commit_all", c.ask("commit-all")),
		disabled(it("u", "scm.undo_commit", c.Git("undo-commit")), none),
	}
	if st != nil && st.Op != "" {
		items = append(items, it("x", abortLabel[st.Op], c.Git("abort")))
	}
	items = append(items, sep(),
		disabled(it("A", "scm.commit_amend", c.ask("commit-amend")), none),
		disabled(it("S", "scm.commit_staged_amend", c.ask("commit-staged-amend")), none),
		disabled(it("L", "scm.commit_all_amend", c.ask("commit-all-amend")), none),
		sep(),
		it("o", "scm.commit_signoff", c.ask("commit-signoff")),
		it("O", "scm.commit_staged_signoff", c.ask("commit-staged-signoff")),
		it("I", "scm.commit_all_signoff", c.ask("commit-all-signoff")),
	)
	return &workspace.Menu{Title: i18n.T("scm.commit"), Items: items}
}

// Changes is the Changes submenu.
func Changes(c Ctx) *workspace.Menu {
	return &workspace.Menu{Title: i18n.T("scm.changes"), Items: []item{
		c.back(),
		it("a", "scm.stage_all", c.Git("stage-all")),
		it("u", "scm.unstage_all", c.Git("unstage-all")),
		it("d", "scm.discard_all", c.confirm("discard-all")),
	}}
}

// PullPush is the Pull, Push submenu.
func PullPush(c Ctx) *workspace.Menu {
	return &workspace.Menu{Title: i18n.T("scm.pull_push"), Items: []item{
		c.back(),
		it("s", "scm.sync", c.popup("sync")),
		sep(),
		it("p", "scm.pull", c.popup("pull")),
		it("r", "scm.pull_rebase", c.popup("pull-rebase")),
		it("f", "scm.pull_from", c.pick("pull-from")),
		sep(),
		it("P", "scm.push", c.popup("push")),
		it("t", "scm.push_to", c.pick("push-to")),
		it("T", "scm.push_tags", c.popup("push-tags")),
		sep(),
		it("e", "scm.fetch", c.popup("fetch")),
		it("n", "scm.fetch_prune", c.popup("fetch-prune")),
		it("a", "scm.fetch_all", c.popup("fetch-all")),
	}}
}

// Branch is the Branch submenu.
func Branch(c Ctx, st *gitops.Status) *workspace.Menu {
	detached := st == nil || st.Branch == ""
	return &workspace.Menu{Title: i18n.T("scm.branch"), Items: []item{
		c.back(),
		it("m", "scm.merge", c.pick("merge")),
		it("r", "scm.rebase", c.pick("rebase")),
		sep(),
		it("c", "scm.create_branch", c.ask("create-branch")),
		it("f", "scm.create_branch_from", c.pick("branch-from")),
		sep(),
		disabled(it("n", "scm.rename_branch", c.ask("rename-branch")), detached),
		it("d", "scm.delete_branch", c.pick("delete-branch")),
		it("D", "scm.delete_remote_branch", c.pick("delete-remote-branch")),
		sep(),
		disabled(it("p", "scm.publish", c.pick("publish")), detached || st.Upstream != ""),
	}}
}

// Remote is the Remote submenu.
func Remote(c Ctx) *workspace.Menu {
	return &workspace.Menu{Title: i18n.T("scm.remote"), Items: []item{
		c.back(),
		it("a", "scm.add_remote", c.ask("add-remote")),
		it("r", "scm.remove_remote", c.pick("remove-remote")),
	}}
}

// Stash is the Stash submenu; Stash Staged needs git 2.35.
func Stash(c Ctx, stagedOK bool) *workspace.Menu {
	return &workspace.Menu{Title: i18n.T("scm.stash"), Items: []item{
		c.back(),
		it("s", "scm.stash_push", c.ask("stash")),
		it("u", "scm.stash_untracked", c.ask("stash-untracked")),
		disabled(it("S", "scm.stash_staged", c.ask("stash-staged")), !stagedOK),
		sep(),
		it("a", "scm.stash_apply_latest", c.Git("stash-apply")),
		it("A", "scm.stash_apply", c.pick("stash-apply")),
		sep(),
		it("p", "scm.stash_pop_latest", c.Git("stash-pop")),
		it("P", "scm.stash_pop", c.pick("stash-pop")),
		sep(),
		it("d", "scm.stash_drop", c.pick("stash-drop")),
		it("D", "scm.stash_drop_all", c.confirm("stash-clear")),
		sep(),
		it("v", "scm.stash_view", c.pick("stash-view")),
	}}
}

// Tags is the Tags submenu.
func Tags(c Ctx) *workspace.Menu {
	return &workspace.Menu{Title: i18n.T("scm.tags"), Items: []item{
		c.back(),
		it("c", "scm.create_tag", c.ask("create-tag")),
		it("d", "scm.delete_tag", c.pick("delete-tag")),
		it("r", "scm.delete_remote_tag", c.pick("delete-remote-tag")),
	}}
}

// Worktrees is the Worktrees submenu.
func Worktrees(c Ctx) *workspace.Menu {
	return &workspace.Menu{Title: i18n.T("scm.worktrees"), Items: []item{
		c.back(),
		it("c", "scm.worktree_create", c.pick("worktree-create")),
		it("o", "scm.worktree_open", c.pick("worktree-open")),
		it("d", "scm.worktree_delete", c.pick("worktree-delete")),
	}}
}

// CommitButton is the dropdown next to the Commit button.
func CommitButton(c Ctx, st *gitops.Status) *workspace.Menu {
	none := st != nil && st.NoCommits
	return &workspace.Menu{Title: i18n.T("scm.commit"), Items: []item{
		it("c", "scm.commit_default", c.ask("commit")),
		disabled(it("a", "scm.commit_amend", c.ask("commit-amend")), none),
		sep(),
		it("p", "scm.commit_push", c.ask("commit-push")),
		it("s", "scm.commit_sync", c.ask("commit-sync")),
	}}
}

// Sides of a file's menu.
const (
	SideStaged   = "staged"
	SideWorktree = "worktree"
	SideMerge    = "merge"
)

// File is a changed file's menu. It acts on @imux_file (and
// @imux_file_root), which the sidebar sets as it opens it; status is the
// file's letter; reveal the i18n key of Reveal in Finder / File Explorer
// ("" without a file manager).
func File(c Ctx, title, side, status string, untracked bool, reveal string) *workspace.Menu {
	fileOp := c.file
	items := []item{
		disabled(it("o", "scm.open_changes", c.PopupCmd("diff", "--side", side, "--status", status)), side == SideMerge),
		it("f", "scm.open_file", fileOp("open")),
		disabled(it("h", "scm.open_file_head", c.popup("show-head")), untracked || status == "A"),
		sep(),
	}
	switch side {
	case SideStaged:
		items = append(items, it("u", "scm.unstage", c.Git("unstage", "--file-option")))
	case SideMerge:
		items = append(items, it("s", "scm.stage_resolved", c.Git("stage", "--file-option")))
	default:
		discard := c.confirm("discard-file")
		if untracked {
			discard = c.scm("confirm", "discard-file", "--untracked")
		}
		items = append(items, it("s", "scm.stage", c.Git("stage", "--file-option")), it("d", "scm.discard", discard))
	}
	if untracked {
		items = append(items, sep(), it("i", "scm.ignore", c.Git("ignore", "--file-option")))
	}
	items = append(items, sep(), it("e", "scm.reveal_explorer", c.Git("reveal", "--file-option")))
	if reveal != "" {
		items = append(items, it("r", reveal, fileOp("reveal")))
	}
	items = append(items, sep(), it("P", "file.copy_path", fileOp("copy-path")), it("l", "file.copy_relative_path", fileOp("copy-relative-path")))
	return &workspace.Menu{Title: title, Items: items}
}

var hashRE = regexp.MustCompile(`^[0-9a-f]{4,64}$`)

// ValidHash reports whether h is a commit hash a menu may carry.
func ValidHash(h string) bool { return hashRE.MatchString(h) }

// CommitRow is a HISTORY commit's menu (hash: ValidHash).
func CommitRow(c Ctx, hash, subject string) *workspace.Menu {
	s := func(args ...string) string { return c.scm(append(args, "--hash", hash)...) }
	g := func(op string, flags ...string) string { return c.Git(op, append(flags, "--hash", hash)...) }
	return &workspace.Menu{Title: hash + "  " + subject, Items: []item{
		it("o", "scm.open_changes", s("popup", "show")),
		it("c", "scm.checkout_detached_here", g("checkout-ref", "--kind", "commit")),
		it("b", "scm.create_branch_here", s("ask", "create-branch")),
		it("t", "scm.create_tag_here", s("ask", "create-tag")),
		sep(),
		it("p", "scm.cherry_pick", s("popup", "cherry-pick")),
		it("v", "scm.revert", s("popup", "revert")),
		sep(),
		it("i", "scm.copy_commit_id", g("copy-id")),
		it("m", "scm.copy_commit_message", g("copy-message")),
	}}
}

// Confirm asks before something that cannot be undone: yes runs run.
func Confirm(title, yes, run string) *workspace.Menu {
	return &workspace.Menu{Title: title, Items: []item{
		{Name: yes, Key: "y", Cmd: run},
		{Name: i18n.T("click.cancel"), Key: "n"},
	}}
}

// describe is a ref's description in a Quick Pick: its hash, subject and
// age.
func (c Ctx) describe(r gitops.RefInfo) string {
	var parts []string
	if r.Hash != "" {
		parts = append(parts, r.Hash)
	}
	if r.Subject != "" {
		parts = append(parts, r.Subject)
	}
	if r.When.Unix() > 0 {
		parts = append(parts, textfmt.Ago(c.Now, r.When))
	}
	return strings.Join(parts, " · ")
}

// groupKey names the groups of ref kinds.
var groupKey = map[string]string{
	gitops.RefBranch: "scm.group_branches", gitops.RefRemote: "scm.group_remote_branches", gitops.RefTag: "scm.group_tags",
}

// RefItems lists refs of kinds (in that order), not the current branch when
// noCurrent; each runs words with the ref as data (with --kind before it
// when withKind).
func (c Ctx) RefItems(refs []gitops.RefInfo, kinds []string, noCurrent, withKind, inPopup bool, words ...string) []quickpick.Item {
	var items []quickpick.Item
	for _, k := range kinds {
		for _, r := range refs {
			if r.Kind != k || noCurrent && r.Current {
				continue
			}
			w := words
			if withKind {
				w = append(append([]string{}, words...), "--kind", r.Kind)
			}
			label := r.Name
			if r.Current {
				label += "  ✓"
			}
			items = append(items, quickpick.Item{
				Label: label, Description: c.describe(r), Group: i18n.T(groupKey[k]),
				Argv: c.Argv(w, r.Name), InPopup: inPopup,
			})
		}
	}
	return items
}

func (c Ctx) pickOf(title, empty string, items []quickpick.Item) *quickpick.Pick {
	return &quickpick.Pick{Title: title, Placeholder: i18n.T("scm.pick_filter"), Empty: i18n.T(empty), Items: items, Client: c.Client, Pane: c.Pane}
}

// Checkout is Checkout to…: create new branch (from), checkout detached,
// then the branches, remote branches and tags. What was typed names the
// new branch, as in VS Code.
func Checkout(c Ctx, refs []gitops.RefInfo) *quickpick.Pick {
	items := []quickpick.Item{
		{Label: "+ " + i18n.T("scm.create_new_branch"), Always: true, Query: true, Argv: c.Argv([]string{"__scm", "ask", "create-branch"})},
		{Label: "+ " + i18n.T("scm.create_new_branch_from"), Always: true, Query: true, Argv: c.Argv([]string{"__scm", "pick", "branch-from"})},
		{Label: i18n.T("scm.checkout_detached"), Always: true, Argv: c.Argv([]string{"__scm", "pick", "checkout-detached"})},
	}
	items = append(items, c.RefItems(refs, []string{gitops.RefBranch, gitops.RefRemote, gitops.RefTag}, false, true, false, "__git", "checkout-ref")...)
	return c.pickOf(i18n.T("scm.pick_checkout"), "scm.pick_none", items)
}

// RefPick is a Quick Pick of refs that runs words with the chosen one.
func RefPick(c Ctx, title string, refs []gitops.RefInfo, kinds []string, noCurrent, inPopup bool, words ...string) *quickpick.Pick {
	return c.pickOf(title, "scm.pick_none", c.RefItems(refs, kinds, noCurrent, false, inPopup, words...))
}

// RemotePick is a Quick Pick of remotes that runs words with the chosen
// one's name.
func RemotePick(c Ctx, title string, rems []gitops.Remote, inPopup bool, words ...string) *quickpick.Pick {
	var items []quickpick.Item
	for _, r := range rems {
		items = append(items, quickpick.Item{Label: r.Name, Description: r.FetchURL, Argv: c.Argv(words, r.Name), InPopup: inPopup})
	}
	return c.pickOf(title, "scm.pick_no_remotes", items)
}

// StashPick is a Quick Pick of stashes that runs words with the chosen one.
func StashPick(c Ctx, title string, ss []gitops.Stash, inPopup bool, words ...string) *quickpick.Pick {
	var items []quickpick.Item
	for _, s := range ss {
		items = append(items, quickpick.Item{Label: s.Message, Description: s.Ref + " · " + textfmt.Ago(c.Now, s.When), Argv: c.Argv(words, s.Ref), InPopup: inPopup})
	}
	return c.pickOf(title, "scm.pick_no_stashes", items)
}

// WorktreePick is a Quick Pick of worktrees (not the current one, nor the
// main one when noMain) that runs words with the chosen one's path.
func WorktreePick(c Ctx, title string, ws []gitops.Worktree, noMain bool, words ...string) *quickpick.Pick {
	var items []quickpick.Item
	for _, w := range ws {
		if w.Current || noMain && w.Main || w.Bare {
			continue
		}
		desc := w.Branch
		if w.Detached {
			desc = i18n.T("git.detached")
		}
		items = append(items, quickpick.Item{Label: w.Path, Description: desc, Argv: c.Argv(words, w.Path)})
	}
	return c.pickOf(title, "scm.pick_no_worktrees", items)
}

// WorktreeCreate is Create Worktree…: a new branch, or a local branch no
// worktree has.
func WorktreeCreate(c Ctx, refs []gitops.RefInfo, ws []gitops.Worktree) *quickpick.Pick {
	used := map[string]bool{}
	for _, w := range ws {
		used[w.Branch] = true
	}
	items := []quickpick.Item{{Label: "+ " + i18n.T("scm.create_new_branch"), Always: true, Query: true, Argv: c.Argv([]string{"__scm", "ask", "worktree-branch"})}}
	var free []gitops.RefInfo
	for _, r := range refs {
		if r.Kind == gitops.RefBranch && !used[r.Name] {
			free = append(free, r)
		}
	}
	items = append(items, c.RefItems(free, []string{gitops.RefBranch}, false, false, false, "__git", "worktree-add")...)
	return c.pickOf(i18n.T("scm.worktree_create"), "scm.pick_none", items)
}
