package workspace

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Options that carry a new worktree's prompt (D72): a prompt's template
// turns "%1" into the answer, so the pane and client cannot be in it.
const (
	OptWTPane   = "@imux_wt_pane"
	OptWTClient = "@imux_wt_client"
	OptWTAgent  = "@imux_wt_agent"
	OptWTBranch = "@imux_wt_branch"
)

// WorktreePrompt asks the branch of a new worktree for agent, started from
// pane's project (the place menu's 🌿 In a new worktree…).
func (w *WS) WorktreePrompt(pane, client, agent string) string {
	c := "set-option -g " + OptWTPane + " " + tmux.Quote(pane) +
		" ; set-option -g " + OptWTClient + " " + tmux.Quote(client) +
		" ; set-option -g " + OptWTAgent + " " + tmux.Quote(agent) + " ; command-prompt"
	if client != "" {
		c += " -t " + tmux.Quote(client)
	}
	tmpl := "set-option -g " + OptWTBranch + " \"%%%\" ; " + tmux.RunShell(w.Self, "worktree", "new", "--from-option")
	return c + " -p " + tmux.Quote(i18n.T("worktree.prompt")) + " " + tmux.Quote(tmpl)
}

// ErrNotGit is a worktree asked for outside a git repository.
var ErrNotGit = errors.New("not a git repository")

// NewWorktree opens branch's worktree of the repository dir is in as a
// project with agent ("" the default): <repo>.worktrees/<branch>, with a
// new branch from HEAD, or the branch when it exists (D72).
func (w *WS) NewWorktree(ctx context.Context, dir, branch string, agent session.AgentID) (*StartResult, string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, "", errors.New(i18n.T("worktree.no_branch"))
	}
	main := gitops.MainWorktree(ctx, dir)
	if main == "" {
		top, err := gitops.TopLevel(ctx, dir)
		if err != nil || top == "" {
			return nil, "", ErrNotGit
		}
		main = top
	}
	path := gitops.WorktreePath(main, branch)
	if _, err := os.Stat(path); err != nil {
		if err := gitops.AddWorktree(ctx, main, path, branch, !gitops.BranchExists(ctx, main, branch)); err != nil {
			return nil, path, err
		}
	}
	res, err := w.Start(ctx, agent, path)
	return res, path, err
}

// Worktree is a project that is a linked worktree: its folder and branch,
// its repository's main worktree and the branch checked out there (the
// base it merges into).
type Worktree struct {
	Path, Branch, Main, Base string
}

// WorktreeOf is the linked worktree dir is (ok false for a main worktree,
// a detached one, or no git).
func WorktreeOf(ctx context.Context, dir string) (Worktree, bool) {
	main := gitops.MainWorktree(ctx, dir)
	if main == "" {
		return Worktree{}, false
	}
	top, _ := gitops.TopLevel(ctx, dir)
	wt := Worktree{Path: top, Branch: gitops.CurrentBranch(ctx, dir), Main: main, Base: gitops.CurrentBranch(ctx, main)}
	if wt.Path == "" || wt.Branch == "" || wt.Base == "" {
		return Worktree{}, false
	}
	return wt, true
}

// worktreeItems are a worktree project's menu items: diff, merge, remove.
func (w *WS) worktreeItems(wt Worktree, s, client, pane string) []MenuItem {
	tgt := append([]string{"--session", s}, uiTarget(pane, client)...)
	diff := "display-popup -E -w 90% -h 90%"
	if client != "" {
		diff += " -c " + tmux.Quote(client)
	}
	diff += " -d " + tmux.Quote(tmux.EscapeFormat(wt.Main)) + " " + tmux.Quote(tmux.ShJoin(w.Self, "worktree", "diff", "--session", s))
	return []MenuItem{
		Separator(),
		{Name: i18n.T("worktree.diff", wt.Base), Key: "d", Cmd: diff},
		{Name: i18n.T("worktree.merge", wt.Base), Key: "m", Cmd: w.ui(append([]string{"worktree", "ask", "merge"}, tgt...)...)},
		{Name: i18n.T("worktree.remove"), Key: "r", Cmd: w.ui(append([]string{"worktree", "ask", "remove"}, tgt...)...)},
	}
}

// WorktreeConfirm asks before merging (merge) or removing (remove, and
// remove-force when it has changes) worktree project s.
func (w *WS) WorktreeConfirm(wt Worktree, what, s, client, pane string) *Menu {
	tgt := append([]string{"--session", s}, uiTarget(pane, client)...)
	m := &Menu{}
	switch what {
	case "merge":
		m.Title = i18n.T("worktree.merge_title", wt.Branch, wt.Base)
		m.Items = []MenuItem{{Name: i18n.T("worktree.merge_yes"), Key: "y", Cmd: w.ui(append([]string{"worktree", "merge"}, tgt...)...)}}
	case "remove":
		m.Title = i18n.T("worktree.remove_title", wt.Branch)
		m.Items = []MenuItem{{Name: i18n.T("worktree.remove_yes"), Key: "y", Cmd: w.ui(append([]string{"worktree", "remove"}, tgt...)...)}}
	case "remove-force":
		m.Title = i18n.T("worktree.force_title", wt.Branch)
		m.Items = []MenuItem{{Name: i18n.T("worktree.force_yes"), Key: "y", Cmd: w.ui(append([]string{"worktree", "remove", "--force"}, tgt...)...)}}
	}
	m.Items = append(m.Items, MenuItem{Name: i18n.T("click.cancel"), Key: "n"})
	return m
}
