package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/envdialog"
	"github.com/hamzayogurtcuoglu/i-mux/internal/envfile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/popmenu"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

const envUsage = "env [ls | new [<name>] | edit <name> | rm <name> | apply <name> [--pane P] [--yes] | clear [--pane P] [--yes]]"

func init() {
	register(&command{
		Name:    "env",
		Args:    "[ls | new [<name>] | edit <name> | rm <name> | apply <name> [--pane P] [--yes] | clear [--pane P] [--yes]]",
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
	switch {
	case pos[0] == "ls":
		return envList(e)
	case pos[0] == "edit":
		name, err := named()
		if err != nil {
			return err
		}
		if !envfile.Exists(name) {
			return fmt.Errorf("%s: %w", name, fs.ErrNotExist)
		}
		return envEdit(e, name, 0)
	case pos[0] == "new" && len(pos) == 1:
		in, inOK := e.Stdin.(*os.File)
		out, outOK := e.Stdout.(*os.File)
		if !inOK || !outOK || !isTerminal(in) || !isTerminal(out) {
			return usage
		}
		return envNewDialog(e, in, out, *clientF)
	}
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	client := *clientF
	switch pos[0] {
	case "dialog":
		// The Env view's +: the New environment dialog (`imux env new`) in
		// a popup in the middle of the client, New skill's size (T89.7).
		cw, ch := 80, 24
		if client != "" {
			if out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-c", client, "#{client_width}\t#{client_height}"}); err == nil {
				_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &cw, &ch)
			}
		}
		argv := []string{w.Self, "env", "new"}
		if client != "" {
			argv = append(argv, "--client", client)
		}
		return popmenu.Popup(ctx, w.T, client, "", min(cw, dialogW), min(ch, dialogH), "C", "C", argv)
	case "new":
		name, err := named()
		if err != nil {
			return err
		}
		path, err := envfile.Create(name)
		if err != nil {
			return err
		}
		w.RefreshSidebars(ctx)
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

// checkEnvName checks a --env name (D101): "" is none; a name that cannot
// be an environment's is a usage error.
func checkEnvName(name string) error {
	if name != "" && !envfile.ValidName(name) {
		return &UsageError{Msg: i18n.T("env.bad_name", name)}
	}
	return nil
}

// checkEnv is checkEnvName, and a name with no file is an error, so a new
// agent asked to start with it does not start at all (D101).
func checkEnv(name string) error {
	if err := checkEnvName(name); err != nil {
		return err
	}
	if name != "" && !envfile.Exists(name) {
		return errors.New(i18n.T("env.not_found", name))
	}
	return nil
}

// envNewDialog is `imux env new` with no name, on a terminal (in, out: the
// Env view's popup): the New environment dialog asks the name, then the
// new file opens in the editor, on the line under its template (T89.7).
func envNewDialog(e *Env, in, out *os.File, client string) error {
	name, err := envdialog.Run(envdialog.New(envfile.Exists), in, out)
	if err != nil || name == "" {
		return err
	}
	path, err := envfile.Create(name)
	w, _, werr := newWorkspace(e, false)
	if err != nil {
		if werr == nil && client != "" {
			w.Message(context.Background(), client, "", i18n.T("file.failed", err.Error()))
		}
		return err
	}
	if werr == nil {
		w.RefreshSidebars(context.Background()) // the view lists it while it is edited
	}
	b, _ := os.ReadFile(path)
	return envEdit(e, name, strings.Count(string(b), "\n")+1)
}

// envEdit opens environment name in the editor, at line (0: its first),
// then has the sidebars count its variables again.
func envEdit(e *Env, name string, line int) error {
	args := []string{envfile.Path(name)}
	if line > 0 {
		args = append([]string{"--line", strconv.Itoa(line)}, args...)
	}
	err := runEdit(e, args)
	if w, _, werr := newWorkspace(e, false); werr == nil {
		w.RefreshSidebars(context.Background()) // its count may have changed
	}
	return err
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
