package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/hamzayogurtcuoglu/i-mux/internal/bus"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:    "panes",
		Args:    "[--json]",
		Summary: "cmd.panes",
		Run:     runPanes,
	})
}

// newBus returns a bus for a non-interactive command (called by agents).
func newBus(e *Env) (*bus.Bus, error) {
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return nil, err
	}
	return bus.New(w), nil
}

func runPanes(e *Env, args []string) error {
	fs := newFlags(e, "panes")
	asJSON := fs.Bool("json", false, "print JSON")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	b, err := newBus(e)
	if err != nil {
		return err
	}
	ps, err := b.Panes(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		if ps == nil {
			ps = []bus.Info{}
		}
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ps)
	}
	tw := tabwriter.NewWriter(e.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T("panes.header"))
	self := tmux.EnvPane()
	for _, p := range ps {
		id := p.Pane
		if id == self {
			id += "*"
		}
		a := string(p.Agent)
		if a == "" {
			a = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%ds\t%s\n", id, a, p.Session, p.State, p.IdleSeconds, paths.Shorten(p.CWD))
	}
	return tw.Flush()
}
