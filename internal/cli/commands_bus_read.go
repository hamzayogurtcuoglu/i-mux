package cli

import (
	"context"
	"fmt"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:    "read",
		Args:    "<target> [-n 60]",
		Summary: "cmd.read",
		Run:     runRead,
	})
}

// runRead prints the last lines of another pane (agent bus).
func runRead(e *Env, args []string) error {
	fs := newFlags(e, "read")
	n := fs.Int("n", 60, "number of lines")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "read <target> [-n 60]")}
	}
	b, err := newBus(e)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := b.Resolve(ctx, pos[0], tmux.EnvPane())
	if err != nil {
		return err
	}
	text, err := b.Read(ctx, p.ID, *n)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(e.Stdout, text)
	return err
}
