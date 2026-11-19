package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/envfile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

const envUsage = "env [ls | new <name> | edit <name> | rm <name> | apply <name> [--pane P] [--yes] | clear [--pane P] [--yes]]"

func init() {
	register(&command{
		Name:    "env",
		Args:    "[ls | new <name> | edit <name> | rm <name> | apply <name> [--pane P] [--yes] | clear [--pane P] [--yes]]",
		Summary: "cmd.env",
		Run:     runEnv,
	})
}

// runEnv manages the environments (D84).
func runEnv(e *Env, args []string) error {
	fl := newFlags(e, "env")
	paneF := fl.String("pane", "", "the pane (default: this pane, $TMUX_PANE)")
	clientF := fl.String("client", "", "the client to show it on")
	yes := fl.Bool("yes", false, "apply, clear: restart an agent without asking")
	fromOpt := fl.Bool("from-option", false, "new: the name from the prompt's option")
	pos, err := parseFlags(fl, args)
	if err != nil {
		return err
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", envUsage)}
	if len(pos) == 0 {
		pos = []string{"ls"}
	}
	named := func() (string, error) {
		if len(pos) < 2 {
			return "", usage
		}
		if !envfile.ValidName(pos[1]) {
			return "", &UsageError{Msg: i18n.T("env.bad_name", pos[1])}
		}
		return pos[1], nil
	}
	switch pos[0] {
	case "ls":
		return envList(e)
	case "edit":
		name, err := named()
		if err != nil {
			return err
		}
		if !envfile.Exists(name) {
			return fmt.Errorf("%s: %w", name, fs.ErrNotExist)
		}
		err = runEdit(e, []string{envfile.Path(name)})
		if w, _, werr := newWorkspace(e, false); werr == nil {
			w.RefreshSidebars(context.Background()) // its count may have changed
		}
		return err
	}
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	client := *clientF
	switch pos[0] {
	case "new":
		name := ""
		if *fromOpt {
			// The Env view's + (its prompt's answer).
			opt := func(k string) string {
				out, _ := w.T.Run(ctx, tmux.Cmd{"show-options", "-gqv", k})
				return strings.TrimSpace(out)
			}
			name, client = opt(sidebar.OptEnvAnswer), opt(sidebar.OptEnvClient)
			_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-gu", sidebar.OptEnvAnswer})
			if name == "" {
				return nil // the prompt was left empty
			}
			if !envfile.ValidName(name) {
				w.Message(ctx, client, "", i18n.T("env.bad_name", name))
				return &UsageError{Msg: i18n.T("env.bad_name", name)}
			}
		} else if name, err = named(); err != nil {
			return err
		}
		path, err := envfile.Create(name)
		if err != nil {
			return err
		}
		w.RefreshSidebars(ctx)
		if *fromOpt {
			c := tmux.Cmd{"display-popup"}
			if client != "" {
				c = append(c, "-c", client)
			}
			return w.T.Spawn(append(c, "-E", "-w", "90%", "-h", "90%", tmux.ShJoin(w.Self, "env", "edit", name)))
		}
		fmt.Fprintln(e.Stdout, path)
		return nil
	case "rm":
		name, err := named()
		if err != nil {
			return err
		}
		return w.DeleteEnv(ctx, name)
	case "confirm-rm", "menu":
		name, err := named()
		if err != nil {
			return err
		}
		pane := ""
		if p, err := targetPane(*paneF); err == nil {
			if t, err := w.PaneTarget(ctx, p); err == nil {
				pane = t.ID
			}
		}
		menu := w.EnvRowMenu(ctx, name, pane, client)
		if pos[0] == "confirm-rm" {
			menu = w.EnvDeleteMenu(name, pane, client)
		}
		return w.Show(ctx, menu, client, pane)
	case "apply", "clear":
		name := ""
		if pos[0] == "apply" {
			if name, err = named(); err != nil {
				return err
			}
			if !envfile.Exists(name) {
				return fmt.Errorf("%s: %w", name, fs.ErrNotExist)
			}
		}
		pane, err := targetPane(*paneF)
		if err != nil {
			return err
		}
		p, err := w.PaneTarget(ctx, pane) // a sidebar's commands act on its target pane
		if err != nil {
			return err
		}
		if name == "" && p.Env == "" {
			return nil // nothing to remove
		}
		kind := w.EnvKindOf(ctx, p)
		if kind == workspace.EnvAgent && !*yes {
			// Ask first: the agent restarts (D84).
			return w.Show(ctx, w.EnvMenu(ctx, p, name, client), client, p.ID)
		}
		res, err := w.ApplyEnv(ctx, sources.New(cfg), p.ID, name, client)
		if err != nil {
			msg := err.Error()
			if errors.Is(err, workspace.ErrEnvBusy) {
				msg = i18n.T("env.busy", p.Command)
			}
			w.Message(ctx, client, p.ID, msg)
			return &ExitCodeError{Code: ExitError, Msg: msg}
		}
		agent := ""
		if res.Restart != nil {
			agent = session.ShortName(res.Restart.To)
		}
		fmt.Fprintln(e.Stdout, workspace.EnvMessage(name, agent))
		return nil
	}
	return usage
}

// envList prints the environments: names and keys, never values.
func envList(e *Env) error {
	envs := envfile.List()
	if len(envs) == 0 {
		fmt.Fprintln(e.Stdout, i18n.T("env.none", envfile.Dir()))
		return nil
	}
	for _, en := range envs {
		line := en.Name + "\t" + strings.Join(en.Keys(), " ")
		if len(en.Problems) > 0 {
			line += "\t⚠ " + i18n.T("env.problems", len(en.Problems))
		}
		fmt.Fprintln(e.Stdout, line)
	}
	return nil
}
