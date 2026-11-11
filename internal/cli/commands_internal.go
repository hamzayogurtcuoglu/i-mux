package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "__exec",
		Args:    "<spec-file>",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runExecSpec,
	})
	register(&command{
		Name:    "__menu",
		Args:    "--pane %N [--client C] [--sessions [--page N] | --session <agent>:<id> | --split h|v | --keys | --limits | --changes | --tab | --project S | --close-project S | --close-tab | --agent | --close-agent]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runMenu,
	})
	register(&command{
		Name:    "__click",
		Args:    "<range> --pane %N [--client C]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runClick,
	})
	register(&command{
		Name:    "__pane-died",
		Args:    "<pane> <status>",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runPaneDied,
	})
	register(&command{
		Name:    "pick",
		Args:    "[--pane %N]",
		Summary: "cmd.pick",
		Run:     runPick,
	})
}

// runMenu opens the pane menu, the session picker (--sessions) or the menu
// of one picked conversation (--session) on a client. It is run by key
// bindings, clicks and menu items; errors are shown with display-message.
func runMenu(e *Env, args []string) error {
	fs := newFlags(e, "__menu")
	paneF := fs.String("pane", "", "target pane")
	clientF := fs.String("client", "", "client to show the menu on")
	sessionsF := fs.Bool("sessions", false, "open the session picker")
	pageF := fs.Int("page", 0, "picker page (0-based)")
	pinF := fs.Bool("pin", false, "the picker for pinning conversations")
	sessionF := fs.String("session", "", "menu for one conversation (<agent>:<id>)")
	splitF := fs.String("split", "", "ask which agent a new pane starts with: h (right) or v (down)")
	newF := fs.Bool("new", false, "ask which agent a new pane starts with, then where")
	placeF := fs.String("place", "", "ask where agent (or shell) opens: right, left, down, up or a new tab")
	keysF := fs.Bool("keys", false, "show the keys")
	limitsF := fs.Bool("limits", false, "open the limits popup")
	changesF := fs.Bool("changes", false, "menu of the changed files")
	tabF := fs.Bool("tab", false, "ask which agent a new tab starts with")
	projectF := fs.String("project", "", "menu of a project (session)")
	closeProjectF := fs.String("close-project", "", "ask before closing a project (session)")
	closeTabF := fs.Bool("close-tab", false, "ask before closing the pane's tab")
	agentF := fs.Bool("agent", false, "menu of the pane's agent (go, rename, close)")
	closeAgentF := fs.Bool("close-agent", false, "ask before closing the pane's agent")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := w.PaneTarget(ctx, pane)
	if err != nil {
		return err
	}
	pane = p.ID // a sidebar's commands act on its target pane
	client := *clientF
	var m *workspace.Menu
	switch {
	case *limitsF:
		return w.ShowLimits(client)
	case *keysF:
		m = w.KeysMenu()
	case *tabF:
		m = w.TabMenu(p, client)
	case *projectF != "":
		if m, err = w.ProjectMenu(ctx, *projectF, client, pane); err != nil {
			w.Message(ctx, client, pane, err.Error())
			return err
		}
	case *closeProjectF != "":
		if m, err = w.CloseProjectMenu(ctx, *closeProjectF, client, pane); err != nil {
			w.Message(ctx, client, pane, err.Error())
			return err
		}
	case *agentF:
		if m, err = w.AgentMenu(ctx, p, client); err != nil {
			w.Message(ctx, client, pane, err.Error())
			return err
		}
	case *closeAgentF:
		if m, err = w.CloseAgentMenu(ctx, p, client); err != nil {
			w.Message(ctx, client, pane, err.Error())
			return err
		}
	case *closeTabF:
		if m, err = w.CloseTabMenu(ctx, p, client); err != nil {
			w.Message(ctx, client, pane, err.Error())
			return err
		}
	case *changesF:
		if m = w.ChangesMenu(ctx, p, client); m == nil {
			w.Message(ctx, client, pane, i18n.T("menu.no_changes"))
			return nil
		}
	case *splitF != "":
		m = w.SplitMenu(p, client, *splitF == "h")
	case *newF:
		m = w.NewAgentMenu(p, client)
	case *placeF != "":
		m = w.PlaceMenu(p, client, *placeF)
	case *sessionsF:
		if *pinF {
			m = w.PinMenu(ctx, sources.New(cfg), p, client, *pageF)
		} else {
			m = w.PickerMenu(ctx, sources.New(cfg), p, client, *pageF)
		}
		if m == nil {
			w.Message(ctx, client, pane, i18n.T("picker.none", paths.Shorten(p.CWD)))
			return nil
		}
	case *sessionF != "":
		f := sources.New(cfg)
		ref, err := f.Resolve(ctx, "", *sessionF, p.CWD)
		if err != nil {
			w.Message(ctx, client, pane, err.Error())
			return err
		}
		m = w.SessionMenu(ref, p, client)
	default:
		m = w.PaneMenu(p, client)
	}
	if err := w.Show(ctx, m, client, pane); err != nil {
		w.Message(ctx, client, pane, err.Error())
		return err
	}
	return nil
}

// runClick handles a click on an agent-bar region (range=user|imux:<x>).
func runClick(e *Env, args []string) error {
	fs := newFlags(e, "__click")
	paneF := fs.String("pane", "", "active pane of the clicked client")
	clientF := fs.String("client", "", "the clicked client")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__click <range> --pane %N")}
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := w.PaneTarget(ctx, pane)
	if err != nil {
		return err
	}
	return w.Click(ctx, sources.New(cfg), pos[0], p, *clientF)
}

// runPaneDied is the pane-died hook: fallback, shell or close (§9.5).
func runPaneDied(e *Env, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return &UsageError{Msg: i18n.T("cli.usage", "__pane-died <pane> <status>")}
	}
	status := -1
	if len(args) == 2 {
		if n, err := strconv.Atoi(args[1]); err == nil {
			status = n
		}
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	return w.PaneDied(context.Background(), args[0], status)
}

// runPick opens the session picker on the client attached to the pane's
// session (`imux pick`, the same menu as ⧉ Sessions and prefix S).
func runPick(e *Env, args []string) error {
	fs := newFlags(e, "pick")
	paneF := fs.String("pane", "", "pane to continue a conversation in (default: this pane)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := w.PaneTarget(ctx, pane)
	if err != nil {
		return err
	}
	client := w.ClientFor(ctx, p)
	if client == "" {
		return errors.New(i18n.T("pick.no_client"))
	}
	return runMenu(e, []string{"--sessions", "--pane", p.ID, "--client", client})
}

// runExecSpec reads a launch spec (deleting it), fills in the pane from
// TMUX_PANE when the launcher did not know it yet (new-session), and replaces
// the process with the agent. It returns only on failure; the message stays
// on the dead pane's screen.
func runExecSpec(e *Env, args []string) error {
	if len(args) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__exec <spec-file>")}
	}
	s, err := launch.Read(args[0], true)
	if err != nil {
		return err
	}
	if s.Pane == "" {
		s.Pane = tmux.EnvPane()
	}
	if err := launchExec(s); err != nil {
		return fmt.Errorf("%s: %w", s.Agent, err)
	}
	return nil
}
