package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "switch",
		Args:    "<agent> [--pane %N] [--reason manual|limit] [--no-resume] [--from-session <agent>:<id>]",
		Summary: "cmd.switch",
		Run:     runSwitch,
	})
}

// isTerminal reports whether f is a character device (a terminal).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// targetPane returns --pane or the caller's TMUX_PANE.
func targetPane(flagVal string) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	if p := tmux.EnvPane(); p != "" {
		return p, nil
	}
	return "", &UsageError{Msg: i18n.T("cli.no_pane")}
}

func runSwitch(e *Env, args []string) error {
	fs := newFlags(e, "switch")
	paneF := fs.String("pane", "", "pane to switch (default: this pane, $TMUX_PANE)")
	reasonF := fs.String("reason", "", "manual or limit")
	noResume := fs.Bool("no-resume", false, "never resume the target's earlier session natively")
	fromF := fs.String("from-session", "", "take the context from this conversation (<agent>:<id>)")
	clientF := fs.String("client", "", "tmux client to show messages on")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "switch <agent> [--pane %N]")}
	}
	to, err := parseAgentFlag("agent", pos[0])
	if err != nil {
		return err
	}
	if *reasonF != "" && *reasonF != "manual" && *reasonF != "limit" {
		return &UsageError{Msg: i18n.T("cli.bad_reason", *reasonF)}
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	w, cfg, err := newWorkspace(e, isTerminal(os.Stdin))
	if err != nil {
		return err
	}
	ctx := context.Background()
	if p, err := w.PaneTarget(ctx, pane); err == nil {
		pane = p.ID // a sidebar's commands act on its target pane
	}
	f := sources.New(cfg)
	req := workspace.SwitchReq{Pane: pane, To: to, Reason: *reasonF, NoResume: *noResume, Client: *clientF}
	if *fromF != "" {
		cwd := ""
		if p, err := w.Pane(ctx, pane); err == nil {
			cwd = p.CWD
		}
		ref, err := f.Resolve(ctx, "", *fromF, cwd)
		if err != nil {
			w.Message(ctx, *clientF, pane, workspace.SwitchError(to, err))
			return err
		}
		req.From = &ref
	}
	res, err := w.Switch(ctx, f, req)
	if err != nil {
		w.Message(ctx, *clientF, pane, workspace.SwitchError(to, err))
		if errors.Is(err, workspace.ErrBusy) {
			return &ExitCodeError{Code: ExitError, Msg: i18n.T("switch.busy")}
		}
		return err
	}
	for _, wn := range res.Warnings {
		fmt.Fprintf(e.Stderr, "imux: %s\n", i18n.T("cli.warning", wn))
	}
	switch {
	case res.Resumed && res.Delta:
		fmt.Fprintln(e.Stdout, i18n.T("switch.resumed_delta", session.ShortName(to), clean.Thousands(res.Runes)))
	case res.Resumed:
		fmt.Fprintln(e.Stdout, i18n.T("switch.resumed", session.ShortName(to)))
	case res.Fresh:
		fmt.Fprintln(e.Stdout, i18n.T("switch.fresh", session.ShortName(to)))
	default:
		fmt.Fprintln(e.Stdout, i18n.T("switch.done", session.ShortName(to), clean.Thousands(res.Runes)))
		fmt.Fprintln(e.Stdout, i18n.T("handoff.saved", res.Handoff, clean.Thousands(res.Runes)))
	}
	return nil
}
