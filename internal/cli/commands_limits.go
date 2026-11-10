package cli

import (
	"bytes"
	"context"

	"github.com/hamzayogurtcuoglu/i-mux/internal/pager"
	"github.com/hamzayogurtcuoglu/i-mux/internal/panel"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

func init() {
	register(&command{
		Name:    "limits",
		Args:    "[--json] [--pager]",
		Summary: "cmd.limits",
		Run:     runLimits,
	})
}

func runLimits(e *Env, args []string) error {
	fs := newFlags(e, "limits")
	asJSON := fs.Bool("json", false, "print JSON (schema: docs/LIMITS.md)")
	paged := fs.Bool("pager", false, "show in a pager (used by the tmux popup)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	rep := panel.Collect(context.Background(), cfg, w.State(), func(id session.AgentID) bool { return w.Installed(id) }, now())
	if *asJSON {
		return rep.JSON(e.Stdout)
	}
	if !*paged {
		return rep.Table(e.Stdout)
	}
	var buf bytes.Buffer
	if err := rep.Table(&buf); err != nil {
		return err
	}
	return page(e, nil, pager.Text(buf.String()), 0)
}
