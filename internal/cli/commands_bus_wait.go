package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/bus"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

func init() {
	register(&command{
		Name:    "wait",
		Args:    "<target> [--timeout 600] [--idle 5s] [--print]",
		Summary: "cmd.wait",
		Run:     runWait,
	})
}

// runWait waits until another pane's screen is idle: exit 0, or 124 on
// timeout. --print prints the final screen.
func runWait(e *Env, args []string) error {
	fs := newFlags(e, "wait")
	timeout := fs.Int("timeout", 600, "seconds before giving up (exit 124)")
	idle := fs.Duration("idle", 0, "how long the screen must stay unchanged (default: bus.idle_after)")
	printF := fs.Bool("print", false, "print the final screen")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "wait <target>")}
	}
	b, err := newBus(e)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := b.Resolve(ctx, pos[0], os.Getenv("TMUX_PANE"))
	if err != nil {
		return err
	}
	d := *idle
	if d <= 0 {
		d = b.W.Cfg.Bus.IdleAfter.Duration
	}
	screen, err := b.Wait(ctx, p.ID, d, time.Duration(*timeout)*time.Second)
	if *printF {
		fmt.Fprint(e.Stdout, bus.LastLines(bus.StripANSI(screen), 1<<20))
	}
	if errors.Is(err, bus.ErrTimeout) {
		return &ExitCodeError{Code: ExitTimeout}
	}
	return err
}
