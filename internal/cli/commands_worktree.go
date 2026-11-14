package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/pager"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "worktree",
		Args:    "new <branch> [agent] | diff | merge | remove [--force]  [--session S]",
		Summary: "cmd.worktree",
		Run:     runWorktree,
	})
}

// runWorktree is an agent's worktree (D72): new opens <repo>.worktrees/
// <branch> as a project with the agent; diff, merge and remove act on a
// worktree project (--session, else this pane's or this folder's).
func runWorktree(e *Env, args []string) error {
	fs := newFlags(e, "worktree")
	sessionF := fs.String("session", "", "the worktree's project (default: this pane's)")
	paneF := fs.String("pane", "", "a pane of the project (new: where it starts from)")
	clientF := fs.String("client", "", "the client to show it on")
	fromOpt := fs.Bool("from-option", false, "new: the branch, pane, client and agent from the prompt's options")
	force := fs.Bool("force", false, "remove: even with changes")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", "worktree new <branch> [agent] | diff | merge | remove [--force]")}
	if len(pos) == 0 {
		return usage
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	wc := &worktreeCmd{e: e, w: w, ctx: ctx, client: *clientF, pane: *paneF}
	switch pos[0] {
	case "new":
		branch, agentID := "", ""
		if len(pos) > 1 {
			branch = pos[1]
		}
		if len(pos) > 2 {
			agentID = pos[2]
		}
		if *fromOpt {
			opt := func(k string) string {
				out, _ := w.T.Run(ctx, tmux.Cmd{"show-options", "-gqv", k})
				return strings.TrimSpace(out)
			}
			branch, agentID, wc.pane, wc.client = opt(workspace.OptWTBranch), opt(workspace.OptWTAgent), opt(workspace.OptWTPane), opt(workspace.OptWTClient)
			_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-gu", workspace.OptWTBranch})
			if branch == "" {
				return nil // the prompt was left empty
			}
		}
		return wc.create(branch, session.AgentID(agentID))
	case "diff", "merge", "remove", "ask":
		s, err := wc.session(*sessionF)
		if err != nil {
			return err
		}
		pi, err := w.Project(ctx, s)
		if err != nil {
			return err
		}
		wt, ok := workspace.WorktreeOf(ctx, pi.Dir)
		if !ok {
			return wc.fail(errors.New(i18n.T("worktree.not_one", pi.Name)))
		}
		switch pos[0] {
		case "diff":
			return wc.diff(wt)
		case "merge":
			return wc.merge(wt)
		case "remove":
			return wc.remove(wt, s, *force)
		default:
			if len(pos) < 2 || pos[1] != "merge" && pos[1] != "remove" {
				return usage
			}
			return w.Show(ctx, w.WorktreeConfirm(wt, pos[1], s, wc.client, wc.pane), wc.client, wc.pane)
		}
	}
	return usage
}

type worktreeCmd struct {
	e            *Env
	w            *workspace.WS
	ctx          context.Context
	client, pane string
}

// say tells the client, or the terminal.
func (c *worktreeCmd) say(msg string) {
	if c.client != "" {
		c.w.Message(c.ctx, c.client, c.pane, msg)
		return
	}
	fmt.Fprintln(c.e.Stderr, msg)
}

func (c *worktreeCmd) fail(err error) error {
	c.say(err.Error())
	return err
}

// session is the worktree's project: given, this pane's, or this folder's.
func (c *worktreeCmd) session(s string) (string, error) {
	if s != "" {
		return s, nil
	}
	if p := firstNonEmpty(c.pane, os.Getenv("TMUX_PANE")); p != "" {
		out, err := c.w.T.Run(c.ctx, tmux.Cmd{"display-message", "-p", "-t", p, "#{session_name}"})
		if err == nil && strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return workspace.SessionName(cwd), nil
}

// create opens the worktree from the pane's project (or this folder).
func (c *worktreeCmd) create(branch string, agent session.AgentID) error {
	dir := ""
	if p := firstNonEmpty(c.pane, os.Getenv("TMUX_PANE")); p != "" {
		if pn, err := c.w.PaneTarget(c.ctx, p); err == nil {
			dir = pn.CWD
		}
	}
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		dir = cwd
	}
	if err := c.w.EnsureConf(c.ctx); err != nil {
		return err
	}
	res, path, err := c.w.NewWorktree(c.ctx, dir, branch, agent)
	if errors.Is(err, workspace.ErrNotGit) {
		return c.fail(errors.New(i18n.T("worktree.not_git")))
	}
	if err != nil {
		return c.fail(fmt.Errorf("%s: %w", i18n.T("worktree.failed"), err))
	}
	c.say(i18n.T("worktree.made", branch, path))
	return showSession(c.ctx, c.w, res.Session, c.client)
}

// diff pages what the worktree's branch changed since it left the base.
func (c *worktreeCmd) diff(wt workspace.Worktree) error {
	out, _ := exec.Command("git", "-C", wt.Main, "diff", "--stat", "--patch", "--color=always", wt.Base+"..."+wt.Branch).Output()
	lines := pager.Text(string(out))
	if len(strings.TrimSpace(string(out))) == 0 {
		lines = []string{"  " + i18n.T("worktree.no_diff", wt.Branch, wt.Base)}
	}
	return page(c.e, []string{" " + i18n.T("worktree.diff_title", wt.Branch, wt.Base), ""}, lines, 0)
}

// merge merges the worktree's branch into the base, in the main worktree.
func (c *worktreeCmd) merge(wt workspace.Worktree) error {
	if !gitops.Clean(c.ctx, wt.Path) {
		return c.fail(errors.New(i18n.T("worktree.dirty", wt.Branch)))
	}
	if err := gitops.Merge(c.ctx, wt.Main, wt.Branch); err != nil {
		return c.fail(errors.New(i18n.T("worktree.merge_failed", wt.Branch, wt.Base, firstLine(err.Error()))))
	}
	c.say(i18n.T("worktree.merged", wt.Branch, wt.Base))
	return nil
}

// remove removes the worktree (the branch stays) and closes its project;
// with changes it asks again, with --force.
func (c *worktreeCmd) remove(wt workspace.Worktree, s string, force bool) error {
	err := gitops.RemoveWorktree(c.ctx, wt.Main, wt.Path, force)
	if err != nil && !force && strings.Contains(err.Error(), "--force") {
		return c.w.Show(c.ctx, c.w.WorktreeConfirm(wt, "remove-force", s, c.client, c.pane), c.client, c.pane)
	}
	if err != nil {
		return c.fail(err)
	}
	_ = c.w.CloseProject(c.ctx, s)
	c.say(i18n.T("worktree.removed", wt.Branch))
	return nil
}
