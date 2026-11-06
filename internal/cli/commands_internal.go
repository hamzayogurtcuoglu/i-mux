package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
)

func init() {
	register(&command{
		Name:    "__exec",
		Args:    "<spec-file>",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runExecSpec,
	})
	register(&command{
		Name:    "__menu",
		Args:    "--pane %N [--client C] [--sessions [--page N] | --session <agent>:<id>]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runMenu,
	})
}

// runMenu opens the pane menu (or the session picker) on a client. It is run
// by key bindings and clicks; errors are shown with display-message.
func runMenu(e *Env, args []string) error {
	fs := newFlags(e, "__menu")
	paneF := fs.String("pane", "", "target pane")
	clientF := fs.String("client", "", "client to show the menu on")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := w.Pane(ctx, pane)
	if err != nil {
		return err
	}
	if err := w.Show(ctx, w.PaneMenu(p, *clientF), *clientF, pane); err != nil {
		w.Message(ctx, *clientF, pane, err.Error())
		return err
	}
	return nil
}

// runExecSpec reads a launch spec (deleting it), fills in the pane from
// TMUX_PANE when the launcher did not know it yet (new-session), and replaces
// the process with the agent. It returns only on failure; the message stays
// on the dead pane's screen.
func runExecSpec(e *Env, args []string) error {
	if len(args) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__exec <spec-file>")}
	}
	s, err := launch.Read(args[0], true)
	if err != nil {
		return err
	}
	if s.Pane == "" {
		s.Pane = os.Getenv("TMUX_PANE")
	}
	if err := launchExec(s); err != nil {
		return fmt.Errorf("%s: %w", s.Agent, err)
	}
	return nil
}
