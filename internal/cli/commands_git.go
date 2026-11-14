package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/fileops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/pager"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/scmui"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__git",
		Args:   "<op> [--cwd D | --scm] [--client C] [--pane P] [--untracked] [--from-option] [--draft] [--file-option] [--arg-option] [path...]",
		Hidden: true,
		Run:    runGit,
	})
}

// gitPopupOps run in a popup: their output shows, and a password prompt
// or a conflict with it (D27, D58).
var gitPopupOps = map[string]bool{
	"push": true, "pull": true, "sync": true, "show": true, "fetch": true, "fetch-prune": true, "fetch-all": true,
	"pull-rebase": true, "pull-from": true, "push-to": true, "push-tags": true, "publish": true, "merge": true,
	"rebase": true, "cherry-pick": true, "revert": true, "delete-remote-branch": true, "delete-remote-tag": true,
	"continue": true, "clone": true, "output": true, "show-head": true, "stash-show": true,
}

// gitCall is one `imux __git` call.
type gitCall struct {
	e      *Env
	w      *workspace.WS
	ctx    context.Context
	cwd    string
	client string
	pane   string
	side   string // --sidebar
}

func (g *gitCall) opt(name string) string {
	out, _ := g.w.T.Run(g.ctx, tmux.Cmd{"show-options", "-gqv", name})
	return strings.TrimSpace(out)
}

func (g *gitCall) setOpt(name, v string) {
	if v == "" {
		_, _ = g.w.T.Run(g.ctx, tmux.Cmd{"set-option", "-gu", name})
		return
	}
	_, _ = g.w.T.Run(g.ctx, tmux.Cmd{"set-option", "-g", name, v})
}

// take reads a global option and unsets it.
func (g *gitCall) take(name string) string {
	v := g.opt(name)
	g.setOpt(name, "")
	return v
}

func (g *gitCall) say(msg string) {
	if g.client != "" {
		g.w.Message(g.ctx, g.client, "", msg)
	} else {
		fmt.Fprintln(g.e.Stderr, msg)
	}
}

// scmCtx is the context of the menus a __git call opens.
func (g *gitCall) scmCtx() scmui.Ctx {
	self, _ := workspace.Self()
	return scmui.Ctx{Self: self, Dir: g.cwd, Client: g.client, Pane: g.pane, Sidebar: g.side, Now: time.Now()}
}

// runGit runs one source-control action of the sidebar (D27, D58) in --cwd
// (--scm: the folder in @imux_scm_cwd) and makes the sidebars redraw.
// Network actions, and the ones that may stop on a conflict, run in a popup:
// their output stays until Enter.
func runGit(e *Env, args []string) error {
	fs := newFlags(e, "__git")
	cwd := fs.String("cwd", ".", "the project folder")
	scmF := fs.Bool("scm", false, "the folder is in @imux_scm_cwd")
	client := fs.String("client", "", "client for messages")
	untracked := fs.Bool("untracked", false, "the files are untracked (discard deletes them); stash: untracked files too")
	fromOpt := fs.Bool("from-option", false, "read the message or name from @imux_git")
	draft := fs.Bool("draft", false, "commit: the message is the one in the commit box (D65)")
	fileOpt := fs.Bool("file-option", false, "the path is @imux_file")
	argOpt := fs.Bool("arg-option", false, "a value from an earlier step is in @imux_scm_arg")
	paneF := fs.String("pane", "", "the pane the sidebar follows")
	sidebarF := fs.String("sidebar", "", "the sidebar's pane (reveal)")
	mode := fs.String("mode", "", "commit: staged or all")
	amend := fs.Bool("amend", false, "commit: amend the last commit")
	signoff := fs.Bool("signoff", false, "commit: add Signed-off-by")
	then := fs.String("then", "", "commit: then push or sync")
	kind := fs.String("kind", gitops.RefBranch, "checkout-ref: branch, remote, tag or commit")
	force := fs.Bool("force", false, "delete-branch, worktree-remove: even if not merged or changed")
	newBranch := fs.Bool("new", false, "worktree-add: a new branch")
	hash := fs.String("hash", "", "a commit")
	stagedOnly := fs.Bool("staged", false, "stash: the staged changes only")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return &UsageError{Msg: i18n.T("cli.usage", "__git <op> [path...]")}
	}
	op, files := pos[0], pos[1:]
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	g := &gitCall{e: e, w: w, ctx: context.Background(), cwd: *cwd, client: *client, pane: *paneF, side: *sidebarF}
	ctx := g.ctx
	if *scmF {
		if g.cwd = g.opt(scmui.OptCwd); g.cwd == "" {
			return &UsageError{Msg: "no folder in " + scmui.OptCwd}
		}
	}
	if *hash != "" && !scmui.ValidHash(*hash) {
		return &UsageError{Msg: "bad commit: " + *hash}
	}
	gitops.LogPath = filepath.Join(paths.Home(), "git.log")
	if gitPopupOps[op] {
		return g.popup(op, files, *hash)
	}
	arg := strings.Join(files, " ")
	if *fromOpt {
		arg = strings.TrimSpace(g.take("@imux_git"))
	}
	if *fileOpt {
		abs := g.opt(sidebar.OptFile)
		rel, err := filepath.Rel(g.cwd, abs)
		if err != nil || abs == "" {
			return errors.New("no file")
		}
		files = []string{rel}
	}
	argVal := ""
	if *argOpt {
		argVal = g.take(scmui.OptArg)
	}
	defer sidebar.RefreshAll(ctx, w.T)
	switch op {
	case "stage":
		err = gitops.Stage(ctx, g.cwd, files...)
	case "unstage":
		err = gitops.Unstage(ctx, g.cwd, files...)
	case "discard":
		err = gitops.Discard(ctx, g.cwd, *untracked, files...)
	case "stage-all":
		err = gitops.StageAll(ctx, g.cwd)
	case "unstage-all":
		err = gitops.UnstageAll(ctx, g.cwd)
	case "discard-all":
		err = gitops.DiscardAll(ctx, g.cwd)
	case "discard-dir":
		if argVal == "" {
			return nil
		}
		err = gitops.DiscardPath(ctx, g.cwd, argVal)
	case "commit":
		msg := arg
		if *draft {
			msg = sidebar.CommitDraft(g.cwd)
		}
		var done bool
		done, err = g.commit(msg, gitops.CommitOpts{Message: msg, Mode: *mode, Amend: *amend, SignOff: *signoff}, *then)
		if done && *draft {
			sidebar.ClearCommitDraft(g.cwd, msg) // the box empties
		}
	case "undo-commit":
		var subject string
		if subject, err = gitops.UndoLastCommit(ctx, g.cwd); err == nil {
			g.say(i18n.T("scm.undone", subject))
		}
	case "abort":
		st, rerr := gitops.Read(ctx, g.cwd)
		if rerr != nil {
			return rerr
		}
		if err = gitops.Abort(ctx, g.cwd, st.Op); err == nil {
			g.say(i18n.T("scm.aborted"))
		}
	case "checkout", "checkout-ref":
		ref, k := arg, *kind
		if *hash != "" {
			ref, k = *hash, "commit"
		}
		if op == "checkout" {
			k = gitops.RefBranch
		}
		if ref == "" {
			return nil
		}
		if err = gitops.CheckoutRef(ctx, g.cwd, ref, k); err == nil {
			g.say(i18n.T("git.switched", ref))
		}
	case "new-branch", "create-branch":
		if arg == "" {
			return nil
		}
		if err = gitops.CreateBranch(ctx, g.cwd, arg, argVal); err == nil {
			g.say(i18n.T("git.switched", arg))
		}
	case "rename-branch":
		if arg == "" {
			return nil
		}
		if err = gitops.RenameBranch(ctx, g.cwd, arg); err == nil {
			g.say(i18n.T("scm.renamed", arg))
		}
	case "delete-branch":
		name := arg
		if *argOpt {
			name = argVal
		}
		err = gitops.DeleteBranch(ctx, g.cwd, name, *force)
		switch {
		case errors.Is(err, gitops.ErrNotMerged):
			g.setOpt(scmui.OptArg, name)
			return g.confirmMenu("delete-branch-force")
		case err == nil:
			g.say(i18n.T("scm.branch_deleted", name))
		}
	case "add-remote":
		if arg == "" || argVal == "" {
			return nil
		}
		if err = gitops.AddRemote(ctx, g.cwd, argVal, arg); err == nil {
			g.say(i18n.T("scm.remote_added", argVal))
		}
	case "remove-remote":
		if err = gitops.RemoveRemote(ctx, g.cwd, argVal); err == nil {
			g.say(i18n.T("scm.remote_removed", argVal))
		}
	case "stash":
		err = gitops.StashPush(ctx, g.cwd, arg, *untracked, *stagedOnly)
		switch {
		case errors.Is(err, gitops.ErrNoChanges):
			g.say(i18n.T("scm.nothing_to_stash"))
			return nil
		case err == nil:
			g.say(i18n.T("scm.stashed"))
		}
	case "stash-apply", "stash-pop":
		apply := gitops.StashApply
		if op == "stash-pop" {
			apply = gitops.StashPop
		}
		if err = apply(ctx, g.cwd, arg); err == nil {
			g.say(i18n.T("scm.stash_applied"))
		}
	case "stash-drop":
		if err = gitops.StashDrop(ctx, g.cwd, argVal); err == nil {
			g.say(i18n.T("scm.stash_dropped", argVal))
		}
	case "stash-clear":
		if err = gitops.StashClear(ctx, g.cwd); err == nil {
			g.say(i18n.T("scm.stashes_dropped"))
		}
	case "create-tag":
		ref := g.take(scmui.OptCommit)
		if argVal == "" {
			return nil
		}
		if err = gitops.CreateTag(ctx, g.cwd, argVal, arg, ref); err == nil {
			g.say(i18n.T("scm.tag_created", argVal))
		}
	case "delete-tag":
		if err = gitops.DeleteTag(ctx, g.cwd, arg); err == nil {
			g.say(i18n.T("scm.tag_deleted", arg))
		}
	case "worktree-add":
		if arg == "" {
			return nil
		}
		top, terr := gitops.TopLevel(ctx, g.cwd)
		if terr != nil {
			return terr
		}
		path := gitops.WorktreePath(top, arg)
		if err = gitops.AddWorktree(ctx, g.cwd, path, arg, *newBranch); err == nil {
			g.setOpt(scmui.OptArg, path)
			c := g.scmCtx()
			return g.w.Show(ctx, scmui.Confirm(i18n.T("scm.worktree_made", path), i18n.T("scm.worktree_open_yes"),
				c.Git("worktree-open", "--arg-option")), g.client, g.pane)
		}
	case "worktree-open":
		path := arg
		if *argOpt {
			path = argVal
		}
		return g.open(path)
	case "worktree-remove":
		err = gitops.RemoveWorktree(ctx, g.cwd, argVal, *force)
		switch {
		case err != nil && !*force && strings.Contains(err.Error(), "--force"):
			g.setOpt(scmui.OptArg, argVal)
			return g.confirmMenu("worktree-remove-force")
		case err == nil:
			g.say(i18n.T("scm.worktree_removed", argVal))
		}
	case "ignore":
		if len(files) != 1 {
			return errors.New("ignore <path>")
		}
		if err = gitops.Ignore(ctx, g.cwd, files[0]); err == nil {
			g.say(i18n.T("scm.ignored", filepath.ToSlash(files[0])))
		}
	case "reveal":
		if len(files) != 1 {
			return errors.New("reveal <path>")
		}
		return g.reveal(filepath.Join(g.cwd, files[0]))
	case "copy-id", "copy-message":
		format := "%H"
		if op == "copy-message" {
			format = "%B"
		}
		out, gerr := gitOutput(ctx, g.cwd, "log", "-1", "--format="+format, *hash)
		if gerr != nil {
			return gerr
		}
		if err = (&fileops.System{}).CopyText(ctx, out); err == nil {
			g.say(i18n.T("scm.copied"))
		}
	case "init":
		if err = gitops.Init(ctx, g.cwd); err == nil {
			g.say(i18n.T("scm.initialized"))
		}
	default:
		return &UsageError{Msg: i18n.T("cli.usage", "__git <op>")}
	}
	if err != nil {
		g.say(i18n.T("git.failed", firstLine(err.Error())))
	}
	return err
}

// commit commits as o says (an empty message keeps an amend's), then
// pushes or syncs in a popup; it reports whether it committed.
func (g *gitCall) commit(arg string, o gitops.CommitOpts, then string) (bool, error) {
	if strings.TrimSpace(arg) == "" && !o.Amend {
		return false, nil // the prompt or the box was left empty
	}
	staged, err := gitops.CommitWith(g.ctx, g.cwd, o)
	if err != nil {
		if errors.Is(err, gitops.ErrNothingStaged) {
			g.say(i18n.T("scm.nothing_staged"))
			return false, nil
		}
		return false, err
	}
	subject, _, _ := strings.Cut(strings.TrimSpace(arg), "\n")
	if subject == "" {
		subject, _, _ = gitops.LastCommit(g.ctx, g.cwd)
	}
	msg := i18n.T("git.committed", strings.TrimSpace(subject))
	if staged {
		msg = i18n.T("git.committed_all", strings.TrimSpace(subject))
	}
	g.say(msg)
	if then == "push" || then == "sync" {
		self, err := workspace.Self()
		if err != nil {
			return true, err
		}
		c := tmux.Cmd{"display-popup"}
		if g.client != "" {
			c = append(c, "-c", g.client)
		}
		c = append(c, "-E", "-w", "80%", "-h", "70%", "-d", tmux.EscapeFormat(g.cwd))
		return true, g.w.T.Spawn(append(c, tmux.ShJoin(self, "__git", then, "--cwd", g.cwd)))
	}
	return true, nil
}

// confirmMenu asks the confirmation `imux __scm confirm op` builds (its
// value is in @imux_scm_arg).
func (g *gitCall) confirmMenu(op string) error {
	c := g.scmCtx()
	return exec.Command(c.Self, append([]string{"__scm", "confirm", op, "--cwd", g.cwd}, c.Flags()...)...).Run()
}

// open opens a folder as an i-mux project on the client.
func (g *gitCall) open(path string) error {
	if path == "" {
		return nil
	}
	self, err := workspace.Self()
	if err != nil {
		return err
	}
	args := []string{"open"}
	if g.client != "" {
		args = append(args, "--client", g.client)
	}
	return exec.Command(self, append(args, "--", path)...).Run()
}

// reveal shows abs in the explorer: the sidebar turns to it, the folders
// above open and it is selected (OptReveal).
func (g *gitCall) reveal(abs string) error {
	if g.side != "" {
		_, _ = g.w.T.Run(g.ctx, tmux.Cmd{"set-option", "-w", "-t", g.side, sidebar.OptView, sidebar.ViewExplorer})
	}
	v := fmt.Sprintf("%d:reveal:%s", time.Now().UnixNano(), abs)
	_, _ = g.w.T.Run(g.ctx, tmux.Cmd{"set-option", "-g", sidebar.OptReveal, v})
	return nil
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// popup runs a network action, one that may stop on a conflict, or a page
// to read, on the popup's terminal; an action waits for Enter at the end.
func (g *gitCall) popup(op string, args []string, hash string) error {
	ctx, cwd := g.ctx, g.cwd
	// run shows git's output and keeps it for the log; git writes to a
	// pipe then, so the network commands are asked for their progress.
	run := func(env []string, a ...string) error {
		fmt.Fprintf(os.Stdout, "\x1b[1m$ git %s\x1b[0m\n", strings.Join(a, " "))
		argv := append([]string{"-C", cwd}, a...)
		switch a[0] {
		case "push", "pull", "fetch", "clone":
			argv = append(append(append([]string{}, argv[:3]...), "--progress"), argv[3:]...)
		}
		said := &tailWriter{max: 64 << 10}
		cmd := exec.Command("git", argv...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, io.MultiWriter(os.Stdout, said), io.MultiWriter(os.Stderr, said)
		cmd.Env = append(os.Environ(), env...)
		err := cmd.Run()
		gitops.Log(cwd, a, string(said.b), err)
		return err
	}
	one := func() (string, error) {
		if len(args) != 1 || args[0] == "" {
			return "", fmt.Errorf("%s needs one argument", op)
		}
		return args[0], nil
	}
	st, _ := gitops.Read(ctx, cwd)
	var err error
	switch op {
	case "push":
		err = run(nil, "push")
	case "pull":
		err = run(nil, "pull")
	case "pull-rebase":
		err = run(nil, "pull", "--rebase")
	case "sync":
		if err = run(nil, "pull"); err == nil {
			err = run(nil, "push")
		}
	case "fetch":
		err = run(nil, "fetch")
	case "fetch-prune":
		err = run(nil, "fetch", "--prune")
	case "fetch-all":
		err = run(nil, "fetch", "--all")
	case "push-tags":
		err = run(nil, "push", "--tags")
	case "pull-from":
		if len(args) != 2 {
			return errors.New("pull-from <remote> <branch>")
		}
		err = run(nil, "pull", args[0], args[1])
	case "push-to", "publish":
		remote, aerr := one()
		if aerr != nil {
			return aerr
		}
		if st == nil || st.Branch == "" {
			err = errors.New(i18n.T("scm.detached_push"))
			fmt.Println(err)
			break
		}
		a := []string{"push", remote, st.Branch}
		if op == "publish" || st.Upstream == "" {
			a = []string{"push", "-u", remote, st.Branch}
		}
		err = run(nil, a...)
	case "merge":
		ref, aerr := one()
		if aerr != nil {
			return aerr
		}
		err = run(nil, "merge", "--no-edit", ref)
	case "rebase":
		ref, aerr := one()
		if aerr != nil {
			return aerr
		}
		err = run(nil, "rebase", ref)
	case "cherry-pick", "revert":
		h := hash
		if h == "" {
			if h, err = one(); err != nil {
				return err
			}
		}
		if op == "revert" {
			err = run(nil, "revert", "--no-edit", h)
		} else {
			err = run(nil, "cherry-pick", h)
		}
	case "continue":
		if st == nil || st.Op == "" {
			fmt.Println(i18n.T("scm.nothing_to_continue"))
			break
		}
		err = run([]string{"GIT_EDITOR=true"}, gitops.ContinueArgs(st.Op)...)
	case "delete-remote-branch":
		ref, aerr := one()
		if aerr != nil {
			return aerr
		}
		remote, b, ok := strings.Cut(ref, "/")
		if !ok {
			return fmt.Errorf("not a remote branch: %s", ref)
		}
		err = run(nil, "push", remote, "--delete", b)
	case "delete-remote-tag":
		tag, aerr := one()
		if aerr != nil {
			return aerr
		}
		err = run(nil, "push", tagRemote(ctx, cwd, st), "--delete", "refs/tags/"+tag)
	case "clone":
		url, aerr := one()
		if aerr != nil {
			return aerr
		}
		dest := cloneDest(filepath.Dir(cwd), url)
		if err = run(nil, "clone", "--", url, dest); err == nil {
			fmt.Println()
			fmt.Print(i18n.T("scm.clone_open", dest) + " ")
			ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if a := strings.ToLower(strings.TrimSpace(ans)); a == "" || a == "y" || a == "e" {
				return g.open(dest)
			}
			return nil
		}
	case "show":
		h := hash
		if h == "" {
			if h, err = one(); err != nil {
				return err
			}
		}
		out, _ := exec.Command("git", "-C", cwd, "show", "--stat", "--patch", "--color=always", h).Output()
		return page(g.e, nil, pager.Text(string(out)), 0)
	case "stash-show":
		ref, aerr := one()
		if aerr != nil {
			return aerr
		}
		out, _ := exec.Command("git", "-C", cwd, "stash", "show", "--stat", "--patch", "--include-untracked", "--color=always", ref).CombinedOutput()
		return page(g.e, nil, pager.Text(string(out)), 0)
	case "show-head":
		rel, aerr := one()
		if aerr != nil {
			return aerr
		}
		out, oerr := exec.Command("git", "-C", cwd, "show", "HEAD:./"+filepath.ToSlash(rel)).CombinedOutput()
		if oerr != nil {
			out = []byte(i18n.T("scm.not_in_head", filepath.ToSlash(rel)) + "\n" + string(out))
		}
		return page(g.e, []string{"HEAD: " + filepath.ToSlash(rel)}, pager.Text(string(out)), 0)
	case "output":
		b, rerr := os.ReadFile(gitops.LogPath)
		if rerr != nil || len(b) == 0 {
			b = []byte(i18n.T("scm.output_empty") + "\n")
		}
		lines := pager.Text(string(b))
		return page(g.e, []string{i18n.T("scm.show_output")}, lines, max(len(lines)-20, 0))
	}
	sidebar.RefreshAll(ctx, g.w.T)
	fmt.Println()
	if err != nil {
		fmt.Println(theme.S().Red + i18n.T("git.popup_failed") + "\x1b[0m")
	} else {
		fmt.Println(theme.S().Green + i18n.T("git.popup_done") + "\x1b[0m")
	}
	fmt.Print(i18n.T("limits.press_enter"))
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return nil
}

// tagRemote is the remote Delete Remote Tag deletes from: the upstream's,
// else origin, else the first one.
func tagRemote(ctx context.Context, dir string, st *gitops.Status) string {
	if st != nil {
		if r, _, ok := strings.Cut(st.Upstream, "/"); ok {
			return r
		}
	}
	rs, _ := gitops.Remotes(ctx, dir)
	for _, r := range rs {
		if r.Name == "origin" {
			return "origin"
		}
	}
	if len(rs) > 0 {
		return rs[0].Name
	}
	return "origin"
}

// tailWriter keeps the last max bytes written to it.
type tailWriter struct {
	b   []byte
	max int
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if over := len(t.b) - t.max; over > 0 {
		t.b = t.b[over:]
	}
	return len(p), nil
}

// cloneDest is where Clone puts url: a folder in parent named after the
// repository, with -2, -3 ... when the name is taken.
func cloneDest(parent, url string) string {
	name := strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	if i := strings.LastIndexAny(name, "/:\\"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		name = "repo"
	}
	dest := filepath.Join(parent, name)
	for n := 2; ; n++ {
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			return dest
		}
		dest = filepath.Join(parent, name+"-"+strconv.Itoa(n))
	}
}
