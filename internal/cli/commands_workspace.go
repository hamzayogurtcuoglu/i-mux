package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "attach",
		Args:    "[session]",
		Summary: "cmd.attach",
		Run:     runAttach,
	})
	register(&command{
		Name:    "ls",
		Args:    "[--names]",
		Summary: "cmd.ls",
		Run:     runLs,
	})
	paneHandoff = handoffForPane
}

// handoffForPane builds the handoff of a pane's current conversation
// (`imux handoff --pane %N --print`, the menu's preview popup).
func handoffForPane(e *Env, cfg *config.Config, pane string, to session.AgentID, budget int) (string, error) {
	w, err := workspace.New(cfg)
	if err != nil {
		return "", err
	}
	w.Now = now
	ctx := context.Background()
	p, err := w.PaneTarget(ctx, pane)
	if err != nil {
		return "", err
	}
	if to == "" {
		to = defaultTarget(cfg, p.Agent)
	}
	res, err := w.BuildHandoff(ctx, sources.New(cfg), workspace.HandoffReq{Pane: p, To: to, Budget: budget})
	if errors.Is(err, workspace.ErrNoSource) {
		return "", errors.New(i18n.T("handoff.no_session", p.CWD))
	}
	if err != nil {
		return "", err
	}
	for _, wn := range res.Warnings {
		fmt.Fprintf(e.Stderr, "imux: %s\n", i18n.T("cli.warning", wn))
	}
	return res.Doc.Text, nil
}

// attachExec replaces the process with `tmux attach`; tests replace it.
var attachExec = func(w *workspace.WS, name string) error { return w.ExecAttach(name) }

// newWorkspace loads the workspace for an interactive command: it refreshes
// the bin cache (this process has the user's interactive PATH) and makes sure
// tmux.conf is current.
func newWorkspace(e *Env, interactive bool) (*workspace.WS, *config.Config, error) {
	cfg, err := loadConfig(e)
	if err != nil {
		return nil, nil, err
	}
	w, err := workspace.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	w.Now = now
	if interactive {
		housekeeping(cfg)
		if bins, err := agent.RefreshCache(cfg, now()); err == nil {
			w.SetBins(bins)
		}
	}
	return w, cfg, nil
}

// defaultAction handles `imux [agent] [-d]`: outside tmux it attaches to this
// directory's session or creates it; inside the workspace it opens a window.
func defaultAction(e *Env, args []string) error {
	fs := newFlags(e, "")
	detach := fs.Bool("d", false, "create or find the session but do not attach")
	fs.BoolVar(detach, "detach", false, "same as -d")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "[agent] [-d]")}
	}
	var id session.AgentID
	if len(pos) == 1 {
		if id, err = parseAgentFlag("agent", pos[0]); err != nil {
			return err
		}
	}
	w, _, err := newWorkspace(e, true)
	if err != nil {
		return err
	}
	if id != "" && !w.Installed(id) {
		return &workspace.NotInstalledError{Agent: id}
	}
	ctx := context.Background()
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := w.EnsureConf(ctx); err != nil {
		return err
	}
	// In the home folder (a terminal's default) ask which project to open
	// instead of making the home folder one (D22).
	if len(pos) == 0 && !*detach && os.Getenv("TMUX") == "" && isTerminal(os.Stdin) &&
		filepath.Clean(cwd) == filepath.Clean(paths.UserHome()) {
		c, ok, err := pickFolder(ctx, w)
		if err != nil || !ok {
			return err
		}
		if c.Kind == folders.KindRestore {
			return openChoice(ctx, w, c, "")
		}
		cwd = c.Path
	}
	if pane := os.Getenv("TMUX_PANE"); pane != "" && w.T.Inside(os.Getenv("TMUX")) {
		p, err := w.NewWindow(ctx, pane, id, cwd)
		if err != nil {
			return err
		}
		fmt.Fprintln(e.Stderr, i18n.T("start.window", p))
		return nil
	}
	res, err := w.Start(ctx, id, cwd)
	if err != nil {
		return err
	}
	if *detach {
		if res.Created {
			fmt.Fprintln(e.Stdout, i18n.T("start.created", res.Session, res.Pane))
		} else {
			fmt.Fprintln(e.Stdout, i18n.T("start.exists", res.Session))
		}
		return nil
	}
	return attachExec(w, res.Session)
}

func runAttach(e *Env, args []string) error {
	fs := newFlags(e, "attach")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "attach [session]")}
	}
	w, _, err := newWorkspace(e, true)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := w.EnsureConf(ctx); err != nil {
		return err
	}
	var name string
	if len(pos) == 1 {
		name = pos[0]
		ok, err := w.HasSession(ctx, name)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New(i18n.T("attach.no_such", name))
		}
	} else {
		ss, err := w.Sessions(ctx)
		if err != nil {
			return err
		}
		if len(ss) == 0 {
			return errors.New(i18n.T("attach.none"))
		}
		name = ss[0].Name
	}
	workspace.EnsureWatcher(w)
	return attachExec(w, name)
}

func runLs(e *Env, args []string) error {
	fs := newFlags(e, "ls")
	names := fs.Bool("names", false, "print session names only")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ss, err := w.Sessions(context.Background())
	if err != nil {
		return err
	}
	if *names {
		for _, s := range ss {
			fmt.Fprintln(e.Stdout, s.Name)
		}
		return nil
	}
	if len(ss) == 0 {
		fmt.Fprintln(e.Stderr, i18n.T("ls.none"))
		return nil
	}
	tw := tabwriter.NewWriter(e.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T("ls.header"))
	for _, s := range ss {
		st := i18n.T("ls.detached")
		if s.Attached > 0 {
			st = i18n.T("ls.attached")
		}
		for _, p := range s.Panes {
			a := string(p.Agent)
			pst := st
			switch {
			case p.Dead:
				pst = i18n.T("ls.dead")
			case a == "":
				a = "-"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Name, p.ID, a, pst, paths.Shorten(p.CWD))
		}
	}
	return tw.Flush()
}

func init() {
	register(&command{
		Name:    "split",
		Args:    "<agent> [-h|-v] [--pane %N]",
		Summary: "cmd.split",
		Run:     runSplit,
	})
}

// runSplit opens a new pane with agent next to the target pane, in that
// pane's project directory.
func runSplit(e *Env, args []string) error {
	fs := newFlags(e, "split")
	horiz := fs.Bool("h", false, "split side by side")
	vert := fs.Bool("v", false, "split one above the other (default)")
	paneF := fs.String("pane", "", "pane to split (default: this pane, $TMUX_PANE)")
	clientF := fs.String("client", "", "tmux client to show messages on")
	fromF := fs.String("from-session", "", "continue this conversation (<agent>:<id>) in the new pane")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *horiz && *vert {
		return &UsageError{Msg: i18n.T("cli.usage", "split <agent> [-h|-v]")}
	}
	id, err := parseAgentFlag("agent", pos[0])
	if err != nil {
		return err
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	w, _, err := newWorkspace(e, isTerminal(os.Stdin))
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := w.PaneTarget(ctx, pane)
	if err != nil {
		return err
	}
	pane = p.ID // a sidebar's commands act on its target pane
	if *fromF != "" {
		f := sources.New(w.Cfg)
		ref, err := f.Resolve(ctx, "", *fromF, p.CWD)
		if err != nil {
			w.Message(ctx, *clientF, pane, workspace.SwitchError(id, err))
			return err
		}
		res, err := w.Switch(ctx, f, workspace.SwitchReq{Pane: p.ID, To: id, From: &ref, NewPane: true, Client: *clientF})
		if err != nil {
			w.Message(ctx, *clientF, pane, workspace.SwitchError(id, err))
			return err
		}
		fmt.Fprintln(e.Stdout, res.Pane)
		return nil
	}
	np, err := w.Split(ctx, p.ID, id, p.CWD, *horiz)
	if err != nil {
		w.Message(ctx, *clientF, pane, workspace.SwitchError(id, err))
		return err
	}
	fmt.Fprintln(e.Stdout, np)
	return nil
}
