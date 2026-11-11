package cli

import (
	"context"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:   "__tmux",
		Args:   "<tmux commands>",
		Hidden: true,
		Run:    runTmux,
	})
}

// runTmux runs a tmux command string through imux's client. Where the
// multiplexer cannot run imux's ids itself (psmux: a menu item that names a
// pane as %3~session), the item runs this instead and the client routes it.
func runTmux(e *Env, args []string) error {
	if len(args) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__tmux '<commands>'")}
	}
	cmds, err := tmux.ParseCommandLine(args[0])
	if err != nil || len(cmds) == 0 {
		return err
	}
	_, err = tmux.New(paths.TmuxConf()).Run(context.Background(), cmds...)
	return err
}
