package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/bus"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:    "send",
		Args:    "<target> <message|-> [--wait-idle] [--no-enter]",
		Summary: "cmd.send",
		Run:     runSend,
	})
	register(&command{
		Name:    "__deliver",
		Args:    "<file> --pane %N [--no-enter] | <file> --discard",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runDeliver,
	})
}

// runSend delivers a message to another agent's pane (agent bus). The
// message "-" is read from stdin.
func runSend(e *Env, args []string) error {
	fs := newFlags(e, "send")
	waitIdle := fs.Bool("wait-idle", false, "wait until the target is idle before sending")
	noEnter := fs.Bool("no-enter", false, "paste without pressing Enter")
	timeout := fs.Int("timeout", 600, "seconds to wait with --wait-idle")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return &UsageError{Msg: i18n.T("cli.usage", "send <target> <message|->")}
	}
	msg := pos[1]
	if msg == "-" {
		b, err := io.ReadAll(io.LimitReader(e.Stdin, 1<<20))
		if err != nil {
			return err
		}
		msg = string(b)
	}
	b, err := newBus(e)
	if err != nil {
		return err
	}
	res, err := b.Send(context.Background(), bus.SendReq{
		From: tmux.EnvPane(), FromAgent: session.AgentID(os.Getenv("IMUX_AGENT")),
		Target: pos[0], Msg: msg, WaitIdle: *waitIdle, NoEnter: *noEnter, Timeout: time.Duration(*timeout) * time.Second,
	})
	if errors.Is(err, bus.ErrTimeout) {
		return &ExitCodeError{Code: ExitTimeout, Msg: i18n.T("bus.wait_timeout", pos[0])}
	}
	if err != nil {
		return err
	}
	if res.Pending != "" {
		fmt.Fprintln(e.Stdout, i18n.T("bus.pending", res.Pane))
		return nil
	}
	fmt.Fprintln(e.Stdout, i18n.T("bus.sent", res.Pane))
	return nil
}

// runDeliver delivers (or discards) a message the user confirmed in the
// bus.confirm menu.
func runDeliver(e *Env, args []string) error {
	fs := newFlags(e, "__deliver")
	pane := fs.String("pane", "", "target pane")
	noEnter := fs.Bool("no-enter", false, "do not press Enter")
	discard := fs.Bool("discard", false, "drop the message")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *pane == "" && !*discard {
		return &UsageError{Msg: i18n.T("cli.usage", "__deliver <file> --pane %N")}
	}
	b, err := newBus(e)
	if err != nil {
		return err
	}
	return b.Deliver(context.Background(), pos[0], *pane, !*noEnter, *discard)
}
