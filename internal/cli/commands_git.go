package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/pager"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:   "__git",
		Args:   "<op> [--cwd D] [--client C] [--pane P] [--untracked] [--from-option] [--yes] [path...]",
		Hidden: true,
		Run:    runGit,
	})
}

// runGit runs one source-control action of the sidebar (D27) in --cwd and
// makes the sidebars redraw. Network actions (push, pull, sync) and show run
// in a popup: their output stays until Enter.
func runGit(e *Env, args []string) error {
	fs := newFlags(e, "__git")
	cwd := fs.String("cwd", ".", "the project folder")
	client := fs.String("client", "", "client for messages")
	untracked := fs.Bool("untracked", false, "the paths are untracked (discard deletes them)")
	fromOpt := fs.Bool("from-option", false, "read the message or branch from @imux_git")
	paneF := fs.String("pane", "", "the pane the sidebar follows (suggest: its agent writes the message)")
	yes := fs.Bool("yes", false, "suggest: the user agreed to send the changes")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return &UsageError{Msg: i18n.T("cli.usage", "__git <op> [path...]")}
	}
	op, paths := pos[0], pos[1:]
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	arg := strings.Join(paths, " ")
	if *fromOpt {
		out, _ := w.T.Run(ctx, tmux.Cmd{"show-options", "-gqv", "@imux_git"})
		_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-gu", "@imux_git"})
		arg = strings.TrimSpace(out)
	}
	say := func(msg string) {
		if *client != "" {
			w.Message(ctx, *client, "", msg)
		} else {
			fmt.Fprintln(e.Stderr, msg)
		}
	}
	defer sidebar.RefreshAll(ctx, w.T)
	switch op {
	case "stage":
		err = gitops.Stage(ctx, *cwd, paths...)
	case "unstage":
		err = gitops.Unstage(ctx, *cwd, paths...)
	case "discard":
		err = gitops.Discard(ctx, *cwd, *untracked, paths...)
	case "stage-all":
		err = gitops.StageAll(ctx, *cwd)
	case "unstage-all":
		err = gitops.UnstageAll(ctx, *cwd)
	case "discard-all":
		err = gitops.DiscardAll(ctx, *cwd)
	case "commit":
		if arg == "" {
			return nil // the prompt was left empty
		}
		var staged bool
		if staged, err = gitops.Commit(ctx, *cwd, arg); err == nil {
			msg := i18n.T("git.committed", arg)
			if staged {
				msg = i18n.T("git.committed_all", arg)
			}
			say(msg)
		}
	case "checkout":
		if err = gitops.Checkout(ctx, *cwd, arg); err == nil {
			say(i18n.T("git.switched", arg))
		}
	case "new-branch":
		if arg == "" {
			return nil
		}
		if err = gitops.NewBranch(ctx, *cwd, arg); err == nil {
			say(i18n.T("git.switched", arg))
		}
	case "push", "pull", "sync", "show":
		return gitInPopup(e, *cwd, op, paths)
	case "suggest":
		return suggestCommit(ctx, w, cfg, *cwd, *client, *paneF, *yes, say)
	case "edit-message":
		return editMessage(ctx, e, w, cfg, *cwd, say)
	default:
		return &UsageError{Msg: i18n.T("cli.usage", "__git stage|unstage|discard|stage-all|unstage-all|discard-all|commit|suggest|edit-message|checkout|new-branch|push|pull|sync|show")}
	}
	if err != nil {
		say(i18n.T("git.failed", firstLine(err.Error())))
	}
	return err
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// gitInPopup runs a network command (or shows a commit) with the terminal,
// then waits for Enter.
func gitInPopup(e *Env, cwd, op string, args []string) error {
	run := func(a ...string) error {
		fmt.Fprintf(os.Stdout, "\x1b[1m$ git %s\x1b[0m\n", strings.Join(a, " "))
		cmd := exec.Command("git", append([]string{"-C", cwd}, a...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	var err error
	switch op {
	case "push":
		err = run("push")
	case "pull":
		err = run("pull")
	case "sync":
		if err = run("pull"); err == nil {
			err = run("push")
		}
	case "show":
		if len(args) != 1 {
			return errors.New("show <commit>")
		}
		cmd := exec.Command("git", "-C", cwd, "show", "--stat", "--patch", "--color=always", args[0])
		out, _ := cmd.Output()
		return page(e, nil, pager.Text(string(out)), 0)
	}
	fmt.Println()
	if err != nil {
		fmt.Println("\x1b[31m" + i18n.T("git.popup_failed") + "\x1b[0m")
	} else {
		fmt.Println("\x1b[32m" + i18n.T("git.popup_done") + "\x1b[0m")
	}
	fmt.Print(i18n.T("limits.press_enter"))
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return nil
}
