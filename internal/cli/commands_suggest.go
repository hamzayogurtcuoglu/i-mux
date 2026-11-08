package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/commitmsg"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// noticeCommitSuggest records that the user let the changes go to an agent
// for commit messages (D29).
const noticeCommitSuggest = "commit_suggest_ok"

// suggestCommit asks an agent for a commit message and offers it in a menu
// (§9.3 "Commit message suggestions"). Nothing is committed here.
func suggestCommit(ctx context.Context, w *workspace.WS, cfg *config.Config, cwd, client, pane string, yes bool, say func(string)) error {
	paneAgent := ""
	if pane != "" {
		if p, err := w.Pane(ctx, pane); err == nil {
			paneAgent = string(p.Agent)
		}
	}
	id, model, err := commitmsg.Pick(cfg, paneAgent, w.Installed)
	if err != nil {
		say(i18n.T("git.suggest_failed", "i-mux", err.Error()))
		return err
	}
	name := session.DisplayName(id)
	again := []string{"__git", "suggest", "--cwd", cwd, "--yes"}
	if client != "" {
		again = append(again, "--client", client)
	}
	if pane != "" {
		again = append(again, "--pane", pane)
	}
	if yes {
		_ = state.Update(func(s *state.State) error { s.SetNotice(noticeCommitSuggest); return nil })
	} else if !w.State().Notices[noticeCommitSuggest] {
		m := &workspace.Menu{
			Title: i18n.T("git.suggest_ask_title", name),
			Items: []workspace.MenuItem{
				{Name: i18n.T("git.suggest_ask_text"), Disabled: true},
				workspace.Separator(),
				{Name: i18n.T("git.suggest_yes"), Key: "y", Cmd: tmux.RunShell(w.Self, again...)},
				{Name: i18n.T("click.cancel"), Key: "n"},
			},
		}
		return w.Show(ctx, m, client, pane)
	}

	changes, err := commitmsg.Collect(ctx, cwd)
	if err != nil {
		say(i18n.T("git.failed", firstLine(err.Error())))
		return err
	}
	if changes.Empty() {
		say(i18n.T("git.suggest_nothing"))
		return nil
	}
	progress := tmux.Cmd{"display-message", "-d", "0"}
	if client != "" {
		progress = append(progress, "-c", client)
	}
	_, _ = w.T.Run(ctx, append(progress, tmux.EscapeMessage(i18n.T("git.suggest_writing", name))))

	dir, err := os.MkdirTemp("", "imux-commit-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bin := w.Bin(id)
	run, err := commitmsg.Command(id, bin.Path, model, dir, commitmsg.Instructions, commitmsg.Body(changes))
	if err != nil {
		say(i18n.T("git.suggest_failed", name, err.Error()))
		return err
	}
	msg, err := commitmsg.Execute(ctx, run, dir, bin.PathEnv)
	if err != nil {
		say(i18n.T("git.suggest_failed", name, firstLine(err.Error())))
		return err
	}
	if _, err := w.T.Run(ctx, tmux.Cmd{"set-option", "-g", "@imux_git", msg}); err != nil {
		return err
	}
	commit := []string{"__git", "commit", "--cwd", cwd, "--from-option"}
	edit := []string{"__git", "edit-message", "--cwd", cwd}
	if client != "" {
		commit = append(commit, "--client", client)
		edit = append(edit, "--client", client)
	}
	popup := "display-popup -E -w 80% -h 60% -d " + tmux.Quote(cwd) + " " + tmux.Quote(tmux.ShJoin(append([]string{w.Self}, edit...)...))
	m := &workspace.Menu{
		Title: i18n.T("git.suggest_title", name),
		Items: []workspace.MenuItem{
			{Name: msg, Disabled: true},
			workspace.Separator(),
			{Name: i18n.T("git.suggest_commit"), Key: "c", Cmd: tmux.RunShell(w.Self, commit...)},
			{Name: i18n.T("git.suggest_edit"), Key: "e", Cmd: popup},
			{Name: i18n.T("git.suggest_again"), Key: "r", Cmd: tmux.RunShell(w.Self, again...)},
			{Name: i18n.T("click.cancel"), Key: "q", Cmd: "set-option -gu @imux_git"},
		},
	}
	return w.Show(ctx, m, client, pane)
}

// editMessage opens the suggested message (@imux_git) in the editor and
// commits what is saved; comment lines go, an empty message cancels. It runs
// in a popup.
func editMessage(ctx context.Context, e *Env, w *workspace.WS, cfg *config.Config, cwd string, say func(string)) error {
	out, _ := w.T.Run(ctx, tmux.Cmd{"show-options", "-gqv", "@imux_git"})
	_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-gu", "@imux_git"})
	f, err := os.CreateTemp("", "imux-COMMIT_EDITMSG-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	fmt.Fprintf(f, "%s\n\n%s\n", strings.TrimSpace(out), i18n.T("git.edit_help"))
	f.Close()
	argv, env := editorCmd(cfg, f.Name(), 1)
	if argv == nil {
		say(i18n.T("git.no_editor"))
		return nil
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = filepath.Clean(cwd)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, e.Stdout, e.Stderr
	if err := cmd.Run(); err != nil {
		say(i18n.T("git.failed", firstLine(err.Error())))
		return err
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		return err
	}
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		if l := sc.Text(); !strings.HasPrefix(l, "#") {
			lines = append(lines, strings.TrimRight(l, " \t"))
		}
	}
	msg := strings.TrimSpace(strings.Join(lines, "\n"))
	if msg == "" {
		say(i18n.T("git.commit_cancelled"))
		return nil
	}
	staged, err := gitops.Commit(ctx, cwd, msg)
	if err != nil {
		say(i18n.T("git.failed", firstLine(err.Error())))
		return err
	}
	subject, _, _ := strings.Cut(msg, "\n")
	if staged {
		say(i18n.T("git.committed_all", subject))
	} else {
		say(i18n.T("git.committed", subject))
	}
	return nil
}
