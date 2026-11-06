package cli

import (
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
