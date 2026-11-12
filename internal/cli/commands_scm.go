package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/fileops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/quickpick"
	"github.com/hamzayogurtcuoglu/i-mux/internal/scmui"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__scm",
		Args:   "menu|pick|ask|confirm|popup|set <name> [value] [--cwd D] [--client C] [--pane P] [--sidebar S] [--hash H] [--initial T]",
		Hidden: true,
		Run:    runSCM,
	})
	register(&command{
		Name:   "__quickpick",
		Args:   "[--run] <id>",
		Hidden: true,
		Run:    runQuickPick,
	})
}

// scm is one `imux __scm` call: the Source Control view's menus, Quick
// Picks, prompts, confirmations and popups (D58).
type scm struct {
	e         *Env
	w         *workspace.WS
	ctx       context.Context
	c         scmui.Ctx
	arg       string // the value this step was given (a ref, a remote, a stash, a path)
	initial   string // a prompt's text to start from (what was typed in a Quick Pick)
	hash      string
	fromOpt   bool
	untracked bool
	side      string
	status    string
	x, y      int // a menu's place in the sidebar (1-based); 0: centered
}

func runSCM(e *Env, args []string) error {
	fs := newFlags(e, "__scm")
	cwd := fs.String("cwd", "", "the project folder (default: @imux_scm_cwd)")
	client := fs.String("client", "", "the client")
	pane := fs.String("pane", "", "the pane the sidebar follows")
	sidebarF := fs.String("sidebar", "", "the sidebar's pane")
	hash := fs.String("hash", "", "a commit of HISTORY")
	arg := fs.String("arg", "", "a value for this step")
	initial := fs.String("initial", "", "the prompt's text to start from")
	fromOpt := fs.Bool("from-option", false, "the answer is in @imux_git")
	untracked := fs.Bool("untracked", false, "the file is untracked")
	side := fs.String("side", "", "a file's side: staged, worktree, merge")
	status := fs.String("status", "", "a file's status letter")
	x := fs.Int("x", 0, "the menu's column in the sidebar")
	y := fs.Int("y", 0, "the menu's row in the sidebar")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return &UsageError{Msg: i18n.T("cli.usage", "__scm menu|pick|ask|confirm|popup|set <name>")}
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	s := &scm{
		e: e, w: w, ctx: context.Background(), arg: *arg, initial: *initial, hash: *hash, fromOpt: *fromOpt,
		untracked: *untracked, side: *side, status: *status, x: *x, y: *y,
	}
	if len(pos) > 2 {
		s.arg = pos[2]
	}
	if s.hash != "" && !scmui.ValidHash(s.hash) {
		return &UsageError{Msg: "bad commit: " + s.hash}
	}
	dir := *cwd
	if dir == "" {
		dir = s.opt(scmui.OptCwd)
	} else {
		s.setOpt(scmui.OptCwd, dir)
	}
	if dir == "" {
		return &UsageError{Msg: "no folder"}
	}
	self, err := workspace.Self()
	if err != nil {
		return err
	}
	s.c = scmui.Ctx{Self: self, Dir: dir, Client: *client, Pane: *pane, Sidebar: *sidebarF, X: *x, Y: *y, Now: time.Now()}
	switch verb, name := pos[0], pos[1]; verb {
	case "menu":
		return s.menu(name)
	case "pick":
		return s.pick(name)
	case "ask":
		return s.ask(name)
	case "confirm":
		return s.confirm(name)
	case "popup":
		return s.popup(name)
	case "set":
		return s.set(name, s.arg)
	}
	return &UsageError{Msg: i18n.T("cli.usage", "__scm menu|pick|ask|confirm|popup|set <name>")}
}

func (s *scm) opt(name string) string {
	out, _ := s.w.T.Run(s.ctx, tmux.Cmd{"show-options", "-gqv", name})
	return strings.TrimSpace(out)
}

// setOpt sets a global option ("" unsets it).
func (s *scm) setOpt(name, v string) {
	if v == "" {
		_, _ = s.w.T.Run(s.ctx, tmux.Cmd{"set-option", "-gu", name})
		return
	}
	_, _ = s.w.T.Run(s.ctx, tmux.Cmd{"set-option", "-g", name, v})
}

// answer takes the prompt's answer out of @imux_git.
func (s *scm) answer() string {
	v := s.opt("@imux_git")
	s.setOpt("@imux_git", "")
	return v
}

func (s *scm) say(msg string) {
	if s.c.Client != "" {
		s.w.Message(s.ctx, s.c.Client, "", msg)
	} else {
		fmt.Fprintln(s.e.Stderr, msg)
	}
}

func (s *scm) read() *gitops.Status {
	st, err := gitops.Read(s.ctx, s.c.Dir)
	if err != nil {
		return nil
	}
	return st
}

// show shows a menu: at the pointer (x, y in the sidebar) or centered.
func (s *scm) show(m *workspace.Menu) error {
	if s.x > 0 && s.y > 0 && s.c.Sidebar != "" {
		c := m.Cmd(s.c.Client, s.c.Sidebar)
		px, py := sidebar.MenuAt(s.x, s.y)
		for i := 0; i+1 < len(c); i++ {
			switch c[i] {
			case "-x":
				c[i+1] = px
			case "-y":
				c[i+1] = py
			}
			if c[i] == "--" {
				break
			}
		}
		return s.w.T.ShowMenu(s.ctx, append(s.w.T.Menu(s.ctx), c[1:]...))
	}
	return s.w.Show(s.ctx, m, s.c.Client, s.c.Pane)
}

// file is the path of @imux_file relative to the folder.
func (s *scm) file() (abs, rel string) {
	abs = s.opt(sidebar.OptFile)
	rel, err := filepath.Rel(s.c.Dir, abs)
	if err != nil {
		rel = abs
	}
	return abs, rel
}

func (s *scm) menu(name string) error {
	c := s.c
	var m *workspace.Menu
	switch name {
	case "more":
		m = scmui.More(c)
	case "view":
		m = scmui.ViewSort(c, s.opt(scmui.OptView), s.opt(scmui.OptSort))
	case "commit":
		m = scmui.Commit(c, s.read())
	case "changes":
		m = scmui.Changes(c)
	case "pullpush":
		m = scmui.PullPush(c)
	case "branch":
		m = scmui.Branch(c, s.read())
	case "remote":
		m = scmui.Remote(c)
	case "stash":
		m = scmui.Stash(c, gitops.AtLeast(s.ctx, 2, 35))
	case "tags":
		m = scmui.Tags(c)
	case "worktrees":
		m = scmui.Worktrees(c)
	case "commit-button":
		m = scmui.CommitButton(c, s.read())
	case "file":
		_, rel := s.file()
		m = scmui.File(c, rel, s.side, s.status, s.untracked, (&fileops.System{}).Reveal())
	case "commit-row":
		if s.hash == "" {
			return &UsageError{Msg: "commit-row needs --hash"}
		}
		subject, _ := gitOutput(s.ctx, s.c.Dir, "log", "-1", "--format=%s", s.hash)
		m = scmui.CommitRow(c, s.hash, subject)
	default:
		return &UsageError{Msg: "no menu " + name}
	}
	return s.show(m)
}

// gitOutput runs a git read in dir and returns its trimmed stdout.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// clientSize is the client's size (0, 0 when unknown).
func (s *scm) clientSize() (int, int) {
	if s.c.Client == "" {
		return 0, 0
	}
	out, err := s.w.T.Run(s.ctx, tmux.Cmd{"display-message", "-p", "-c", s.c.Client, "#{client_width}\t#{client_height}"})
	if err != nil {
		return 0, 0
	}
	f := strings.Split(strings.TrimSpace(out), "\t")
	if len(f) != 2 {
		return 0, 0
	}
	w, _ := strconv.Atoi(f[0])
	h, _ := strconv.Atoi(f[1])
	return w, h
}

func (s *scm) showPick(p *quickpick.Pick) error {
	w, h := s.clientSize()
	return quickpick.Show(s.ctx, s.w.T, s.c.Self, p, w, h)
}

func (s *scm) pick(name string) error {
	c, ctx, dir := s.c, s.ctx, s.c.Dir
	refs := func() []gitops.RefInfo { r, _ := gitops.Refs(ctx, dir); return r }
	remotes := func() []gitops.Remote { r, _ := gitops.Remotes(ctx, dir); return r }
	stashes := func() []gitops.Stash { r, _ := gitops.Stashes(ctx, dir); return r }
	worktrees := func() []gitops.Worktree { r, _ := gitops.Worktrees(ctx, dir); return r }
	all := []string{gitops.RefBranch, gitops.RefRemote, gitops.RefTag}
	branches := []string{gitops.RefBranch, gitops.RefRemote}
	var p *quickpick.Pick
	switch name {
	case "checkout":
		p = scmui.Checkout(c, refs())
	case "checkout-detached":
		p = scmui.RefPick(c, i18n.T("scm.pick_detached"), refs(), all, false, false, "__git", "checkout-ref", "--kind", "commit")
	case "merge":
		p = scmui.RefPick(c, i18n.T("scm.pick_merge"), refs(), branches, true, true, "__git", "merge")
	case "rebase":
		p = scmui.RefPick(c, i18n.T("scm.pick_rebase"), refs(), branches, true, true, "__git", "rebase")
	case "branch-from":
		words := []string{"__scm", "ask", "create-branch"}
		if s.initial != "" {
			words = append(words, "--initial", s.initial)
		}
		p = scmui.RefPick(c, i18n.T("scm.pick_branch_from"), refs(), all, false, false, words...)
	case "delete-branch":
		p = scmui.RefPick(c, i18n.T("scm.pick_delete_branch"), refs(), []string{gitops.RefBranch}, true, false, "__git", "delete-branch")
	case "delete-remote-branch":
		p = scmui.RefPick(c, i18n.T("scm.pick_delete_remote_branch"), refs(), []string{gitops.RefRemote}, false, false, "__scm", "confirm", "delete-remote-branch")
	case "pull-from":
		p = scmui.RemotePick(c, i18n.T("scm.pick_pull_from"), remotes(), false, "__scm", "pick", "pull-from-branch")
	case "pull-from-branch":
		var items []quickpick.Item
		for _, r := range refs() {
			if b, ok := strings.CutPrefix(r.Name, s.arg+"/"); ok && r.Kind == gitops.RefRemote {
				items = append(items, quickpick.Item{Label: b, Description: r.Subject, Argv: c.Argv([]string{"__git", "pull-from"}, s.arg, b), InPopup: true})
			}
		}
		p = &quickpick.Pick{Title: i18n.T("scm.pick_pull_branch", s.arg), Placeholder: i18n.T("scm.pick_filter"), Empty: i18n.T("scm.pick_none"), Items: items, Client: c.Client, Pane: c.Pane}
	case "push-to":
		p = scmui.RemotePick(c, i18n.T("scm.pick_push_to"), remotes(), true, "__git", "push-to")
	case "publish":
		rs := remotes()
		switch len(rs) {
		case 0:
			s.say(i18n.T("scm.no_remotes"))
			return nil
		case 1:
			return s.openPopup([]string{"__git", "publish", "--cwd", dir, "--", rs[0].Name})
		}
		p = scmui.RemotePick(c, i18n.T("scm.pick_publish"), rs, true, "__git", "publish")
	case "remove-remote":
		p = scmui.RemotePick(c, i18n.T("scm.remove_remote"), remotes(), false, "__scm", "confirm", "remove-remote")
	case "stash-apply":
		p = scmui.StashPick(c, i18n.T("scm.stash_apply"), stashes(), false, "__git", "stash-apply")
	case "stash-pop":
		p = scmui.StashPick(c, i18n.T("scm.stash_pop"), stashes(), false, "__git", "stash-pop")
	case "stash-drop":
		p = scmui.StashPick(c, i18n.T("scm.stash_drop"), stashes(), false, "__scm", "confirm", "stash-drop")
	case "stash-view":
		p = scmui.StashPick(c, i18n.T("scm.stash_view"), stashes(), true, "__git", "stash-show")
	case "delete-tag":
		p = scmui.RefPick(c, i18n.T("scm.delete_tag"), refs(), []string{gitops.RefTag}, false, false, "__git", "delete-tag")
	case "delete-remote-tag":
		p = scmui.RefPick(c, i18n.T("scm.delete_remote_tag"), refs(), []string{gitops.RefTag}, false, false, "__scm", "confirm", "delete-remote-tag")
	case "worktree-create":
		p = scmui.WorktreeCreate(c, refs(), worktrees())
	case "worktree-open":
		p = scmui.WorktreePick(c, i18n.T("scm.worktree_open"), worktrees(), false, "__git", "worktree-open")
	case "worktree-delete":
		p = scmui.WorktreePick(c, i18n.T("scm.worktree_delete"), worktrees(), true, "__scm", "confirm", "worktree-remove")
	default:
		return &UsageError{Msg: "no pick " + name}
	}
	return s.showPick(p)
}

// askSpec is a prompt: its label, and the __git op (with flags) or the next
// __scm ask step its answer goes to.
type askSpec struct {
	label string
	git   []string
	next  string
	popup string
}

var asks = map[string]askSpec{
	"commit":                {label: "prompt.commit", git: []string{"commit"}},
	"commit-staged":         {label: "prompt.commit", git: []string{"commit", "--mode", gitops.CommitStaged}},
	"commit-all":            {label: "prompt.commit", git: []string{"commit", "--mode", gitops.CommitAll}},
	"commit-amend":          {label: "scm.prompt_amend", git: []string{"commit", "--amend"}},
	"commit-staged-amend":   {label: "scm.prompt_amend", git: []string{"commit", "--mode", gitops.CommitStaged, "--amend"}},
	"commit-all-amend":      {label: "scm.prompt_amend", git: []string{"commit", "--mode", gitops.CommitAll, "--amend"}},
	"commit-signoff":        {label: "prompt.commit", git: []string{"commit", "--signoff"}},
	"commit-staged-signoff": {label: "prompt.commit", git: []string{"commit", "--mode", gitops.CommitStaged, "--signoff"}},
	"commit-all-signoff":    {label: "prompt.commit", git: []string{"commit", "--mode", gitops.CommitAll, "--signoff"}},
	"commit-push":           {label: "prompt.commit", git: []string{"commit", "--then", "push"}},
	"commit-sync":           {label: "prompt.commit", git: []string{"commit", "--then", "sync"}},
	"create-branch":         {label: "prompt.branch", git: []string{"create-branch", "--arg-option"}},
	"rename-branch":         {label: "scm.prompt_rename_branch", git: []string{"rename-branch"}},
	"add-remote":            {label: "scm.prompt_remote_name", next: "add-remote-url"},
	"add-remote-url":        {label: "scm.prompt_remote_url", git: []string{"add-remote", "--arg-option"}},
	"stash":                 {label: "scm.prompt_stash", git: []string{"stash"}},
	"stash-untracked":       {label: "scm.prompt_stash", git: []string{"stash", "--untracked"}},
	"stash-staged":          {label: "scm.prompt_stash", git: []string{"stash", "--staged"}},
	"create-tag":            {label: "scm.prompt_tag_name", next: "tag-message"},
	"tag-message":           {label: "scm.prompt_tag_message", git: []string{"create-tag", "--arg-option"}},
	"worktree-branch":       {label: "scm.prompt_worktree_branch", git: []string{"worktree-add", "--new"}},
	"clone":                 {label: "scm.prompt_clone", popup: "clone"},
}

// ask opens the client's command prompt for op. The answer reaches imux
// through @imux_git ("%%%" escapes it for tmux), never a shell; the
// template carries no pane id (it would read "%1" as the answer).
func (s *scm) ask(op string) error {
	spec, ok := asks[op]
	if !ok {
		return &UsageError{Msg: "no prompt " + op}
	}
	switch {
	case s.fromOpt:
		// A step of a chain: the last answer is this step's value.
		v := strings.TrimSpace(s.answer())
		if v == "" {
			return nil
		}
		s.setOpt(scmui.OptArg, v)
	case op == "create-tag":
		s.setOpt(scmui.OptCommit, s.hash) // the commit a HISTORY menu named
		s.setOpt(scmui.OptArg, "")
	case op == "create-branch" && s.hash != "":
		s.setOpt(scmui.OptArg, s.hash)
	default:
		s.setOpt(scmui.OptArg, s.arg) // Create Branch From's ref, or nothing
	}
	var then []string
	switch {
	case spec.next != "":
		then = []string{"__scm", "ask", spec.next, "--from-option"}
	case spec.popup != "":
		then = []string{"__scm", "popup", spec.popup, "--from-option"}
	default:
		then = append(append([]string{"__git"}, spec.git...), "--scm", "--from-option")
	}
	if s.c.Client != "" {
		then = append(then, "--client", s.c.Client)
	}
	c := tmux.Cmd{"command-prompt"}
	if s.c.Client != "" {
		c = append(c, "-t", s.c.Client)
	}
	initial := s.initial
	if op == "rename-branch" {
		if st := s.read(); st != nil {
			initial = st.Branch
		}
	}
	// -I is a comma-separated list that tmux expands as a format: a text
	// with a comma or a # is not given.
	if initial = strings.TrimSpace(initial); initial != "" && !strings.ContainsAny(initial, ",#") {
		c = append(c, "-I", initial)
	}
	c = append(c, "-p", i18n.T(spec.label), "set-option -g @imux_git \"%%%\" ; "+tmux.RunShell(s.c.Self, then...))
	_, err := s.w.T.Run(s.ctx, c)
	return err
}

// confirm asks before op; the value it acts on goes to OptArg first.
func (s *scm) confirm(op string) error {
	c := s.c
	var title, yes, run string
	switch op {
	case "discard-all":
		title, yes, run = i18n.T("git.discard_all_title"), i18n.T("git.discard_yes"), c.Git("discard-all")
	case "discard-file":
		_, rel := s.file()
		flags := []string{"--file-option"}
		if s.untracked {
			flags = append(flags, "--untracked")
		}
		title, yes, run = i18n.T("git.discard_title", rel), i18n.T("git.discard_yes"), c.Git("discard", flags...)
	case "discard-dir":
		title, yes, run = i18n.T("scm.discard_dir_title", s.arg), i18n.T("git.discard_yes"), c.Git("discard-dir", "--arg-option")
	case "stash-clear":
		title, yes, run = i18n.T("scm.confirm_drop_all"), i18n.T("scm.yes_drop"), c.Git("stash-clear")
	case "stash-drop":
		title, yes, run = i18n.T("scm.confirm_drop", s.arg), i18n.T("scm.yes_drop"), c.Git("stash-drop", "--arg-option")
	case "remove-remote":
		title, yes, run = i18n.T("scm.confirm_remove_remote", s.arg), i18n.T("scm.yes_remove"), c.Git("remove-remote", "--arg-option")
	case "delete-remote-branch":
		title, yes, run = i18n.T("scm.confirm_delete_remote_branch", s.arg), i18n.T("scm.yes_delete"), c.PopupCmd("delete-remote-branch")
	case "delete-remote-tag":
		title, yes, run = i18n.T("scm.confirm_delete_remote_tag", s.arg), i18n.T("scm.yes_delete"), c.PopupCmd("delete-remote-tag")
	case "worktree-remove":
		title, yes, run = i18n.T("scm.confirm_worktree_remove", s.arg), i18n.T("scm.yes_delete"), c.Git("worktree-remove", "--arg-option")
	case "worktree-remove-force":
		title, yes, run = i18n.T("scm.confirm_worktree_force", s.opt(scmui.OptArg)), i18n.T("scm.yes_delete"), c.Git("worktree-remove", "--arg-option", "--force")
		return s.show(scmui.Confirm(title, yes, run))
	case "delete-branch-force":
		title, yes, run = i18n.T("scm.confirm_force_delete", s.opt(scmui.OptArg)), i18n.T("scm.yes_delete"), c.Git("delete-branch", "--arg-option", "--force")
		return s.show(scmui.Confirm(title, yes, run))
	default:
		return &UsageError{Msg: "nothing to confirm: " + op}
	}
	s.setOpt(scmui.OptArg, s.arg)
	return s.show(scmui.Confirm(title, yes, run))
}

// openPopup runs imux with argv in a popup in the folder (git's output
// shows; it waits for Enter).
func (s *scm) openPopup(argv []string) error {
	c := tmux.Cmd{"display-popup"}
	if s.c.Client != "" {
		c = append(c, "-c", s.c.Client)
	}
	c = append(c, "-E", "-w", "80%", "-h", "70%", "-d", tmux.EscapeFormat(s.c.Dir))
	return s.w.T.Spawn(append(c, tmux.ShJoin(append([]string{s.c.Self}, argv...)...)))
}

// popup opens op's popup.
func (s *scm) popup(op string) error {
	dir := s.c.Dir
	g := func(gitOp string, data ...string) []string {
		a := []string{"__git", gitOp, "--cwd", dir}
		if s.c.Client != "" {
			a = append(a, "--client", s.c.Client)
		}
		if len(data) > 0 {
			a = append(append(a, "--"), data...)
		}
		return a
	}
	switch op {
	case "pull", "push", "sync", "fetch", "fetch-prune", "fetch-all", "pull-rebase", "push-tags", "continue", "output":
		return s.openPopup(g(op))
	case "show", "cherry-pick", "revert":
		if s.hash == "" {
			return &UsageError{Msg: op + " needs --hash"}
		}
		return s.openPopup(g(op, s.hash))
	case "show-head":
		_, rel := s.file()
		return s.openPopup(g("show-head", rel))
	case "diff":
		_, rel := s.file()
		c := tmux.Cmd{"display-popup"}
		if s.c.Client != "" {
			c = append(c, "-c", s.c.Client)
		}
		c = append(c, "-E", "-w", "95%", "-h", "90%", "-d", tmux.EscapeFormat(dir))
		return s.w.T.Spawn(append(c, tmux.ShJoin(s.c.Self, "__diff", "--"+s.side, s.status, rel)))
	case "delete-remote-branch", "delete-remote-tag":
		v := s.opt(scmui.OptArg)
		if v == "" {
			return nil
		}
		return s.openPopup(g(op, v))
	case "clone":
		url := strings.TrimSpace(s.answer())
		if url == "" {
			return nil
		}
		return s.openPopup(g("clone", url))
	}
	return &UsageError{Msg: "no popup " + op}
}

// set changes View & Sort for every sidebar.
func (s *scm) set(what, v string) error {
	switch what {
	case "view":
		if v != "tree" {
			v = ""
		}
		s.setOpt(scmui.OptView, v)
	case "sort":
		if v != "name" && v != "status" {
			v = ""
		}
		s.setOpt(scmui.OptSort, v)
	default:
		return &UsageError{Msg: "set view|sort <value>"}
	}
	sidebar.RefreshAll(s.ctx, s.w.T)
	return nil
}

// runQuickPick shows the Quick Pick Show wrote as id and runs the chosen
// item in the popup, or keeps it for `--run id`, which tmux runs once the
// popup has closed.
func runQuickPick(e *Env, args []string) error {
	self, err := workspace.Self()
	if err != nil {
		return err
	}
	if len(args) == 2 && args[0] == "--run" {
		_ = quickpick.RunLater(self, args[1]) // from run-shell: nothing to show
		return nil
	}
	if len(args) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__quickpick [--run] <id>")}
	}
	p, err := quickpick.Load(args[0])
	if err != nil {
		return err
	}
	i, query, err := quickpick.Run(p, os.Stdin, os.Stdout)
	if err != nil || i < 0 {
		return err
	}
	it := p.Items[i]
	argv := quickpick.Argv(it, query)
	if !it.InPopup {
		return quickpick.Later(args[0], argv)
	}
	fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H\x1b[?25h")
	return quickpick.Exec(self, argv, os.Stdin, os.Stdout)
}
