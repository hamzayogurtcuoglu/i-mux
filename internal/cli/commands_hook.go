package cli

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agenthook"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "__hook",
		Args:    "claude | codex <json>",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runHook,
	})
}

// runHook is an agent's hook (D71): Claude Code's events on stdin, Codex's
// notify JSON as the last argument. It writes the state on the pane that
// runs the agent (IMUX_PANE: only i-mux's runs have it). It prints nothing,
// as a hook's output can reach the agent, and always succeeds: a failing
// hook must not get in the agent's way.
func runHook(e *Env, args []string) error {
	pane := os.Getenv("IMUX_PANE")
	if len(args) == 0 || pane == "" {
		return nil
	}
	var r agenthook.Report
	ok := false
	switch args[0] {
	case "claude":
		in, _ := io.ReadAll(io.LimitReader(e.Stdin, 1<<20))
		r, ok = agenthook.Claude(in)
	case "codex":
		if len(args) > 1 {
			r, ok = agenthook.Codex(args[len(args)-1])
		}
	}
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w := &workspace.WS{Cfg: config.Default(), T: tmux.New(paths.TmuxConf()), Now: now}
	_ = w.ReportHook(ctx, pane, session.AgentID(args[0]), r)
	return nil
}
