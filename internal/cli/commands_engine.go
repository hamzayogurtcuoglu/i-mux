package cli

import (
	"context"
	"fmt"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__engine",
		Args:   "switch <tmux|core> [--yes] [--pane P] [--client C]",
		Hidden: true,
		Run:    runEngineSwitch,
	})
}

// runEngineSwitch switches the engine backend (D83).
func runEngineSwitch(e *Env, args []string) error {
	fs := newFlags(e, "__engine")
	paneF := fs.String("pane", "", "pane the menu was shown for")
	clientF := fs.String("client", "", "client that showed it")
	yes := fs.Bool("yes", false, "close the running sessions first")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 || pos[0] != "switch" {
		return &UsageError{Msg: i18n.T("cli.usage", "__engine switch <tmux|core> [--yes] [--pane P] [--client C]")}
	}
	return engineSwitch(e, pos[1], *paneF, *clientF, *yes)
}

// engineSwitch changes [engine] backend (D83). While sessions run on the
// current engine the switch is refused: with a pane and a client (the
// Settings row) as a menu that offers to close them and switch, without them
// (a direct call) as an error naming what to do. With force the sessions are
// closed first.
func engineSwitch(e *Env, value, pane, client string, force bool) error {
	switch value {
	case "tmux", "core":
	default:
		return fmt.Errorf("engine: invalid value %q", value)
	}
	cfg, _, err := config.Load(paths.Config())
	if err != nil {
		return err
	}
	ctx := context.Background()
	t := tmux.New(paths.TmuxConf())
	if workspace.EngineValue(cfg.Engine.Backend) == value {
		return showSettings(e, pane, client, "") // already there
	}
	if t.Running(ctx) {
		if !force {
			if pane == "" || client == "" {
				return fmt.Errorf("%s", i18n.T("settings.engine_running"))
			}
			w, _, werr := newWorkspace(e, false)
			if werr != nil {
				return werr
			}
			m := &workspace.Menu{
				Title: i18n.T("settings.engine_switch_title"),
				Items: []workspace.MenuItem{
					{Name: i18n.T("settings.engine_switch_text", workspace.EngineLabel(value)), Disabled: true},
					workspace.Separator(),
					{Name: i18n.T("settings.engine_switch_close"), Key: "y", Cmd: tmux.RunShell(w.Self, "__engine", "switch", value, "--yes", "--pane", pane, "--client", client)},
					{Name: i18n.T("click.cancel"), Key: "n", Cmd: tmux.RunShell(w.Self, "__settings", "menu", "engine", "--pane", pane, "--client", client)},
				},
			}
			return w.Show(ctx, m, client, pane)
		}
		if _, err := t.Run(ctx, tmux.Cmd{"kill-server"}); err != nil {
			return err
		}
	}
	if err := setSetting("engine", value); err != nil {
		return err
	}
	if client != "" {
		if w, _, werr := newWorkspace(e, false); werr == nil {
			w.Message(ctx, client, pane, i18n.T("settings.engine_switched", workspace.EngineLabel(value)))
		}
	}
	return nil
}
