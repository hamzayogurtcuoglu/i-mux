package cli

import (
	"context"
	"os"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "sidebar",
		Args:    "[on|off|toggle] [--pane %N]",
		Summary: "cmd.sidebar",
		Run:     runSidebarToggle,
	})
	register(&command{
		Name:    "__sidebar",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runSidebar,
	})
	register(&command{
		Name:    "__sidebar-refresh",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runSidebarRefresh,
	})
}

// runSidebar is the sidebar process in its pane (§9.3 "Sidebar").
func runSidebar(e *Env, args []string) error {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return &UsageError{Msg: i18n.T("cli.no_pane")}
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	self, err := workspace.Self()
	if err != nil {
		return err
	}
	src := &sidebar.Source{T: tmux.New(paths.TmuxConf()), Cfg: cfg, Now: time.Now}
	return sidebar.Run(context.Background(), sidebar.Options{
		Source: src, Self: self, Pane: pane, Width: cfg.Tmux.SidebarWidth, In: os.Stdin, Out: os.Stdout,
	})
}

// runSidebarRefresh signals every sidebar to redraw (tmux hooks).
func runSidebarRefresh(e *Env, args []string) error {
	sidebar.RefreshAll(context.Background(), tmux.New(paths.TmuxConf()))
	return nil
}

// runSidebarToggle shows or hides the sidebar of the pane's window.
func runSidebarToggle(e *Env, args []string) error {
	fs := newFlags(e, "sidebar")
	paneF := fs.String("pane", "", "a pane of the window (default: this pane)")
	clientF := fs.String("client", "", "client for the message")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	mode := "toggle"
	if len(pos) == 1 {
		mode = pos[0]
	}
	if len(pos) > 1 || (mode != "on" && mode != "off" && mode != "toggle") {
		return &UsageError{Msg: i18n.T("cli.usage", "sidebar [on|off|toggle]")}
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
	shown, err := w.ToggleSidebar(ctx, pane, mode)
	if err != nil {
		w.Message(ctx, *clientF, pane, err.Error())
		return err
	}
	msg := i18n.T("sidebar.toggled_off")
	if shown {
		msg = i18n.T("sidebar.toggled_on")
	}
	if *clientF != "" {
		w.Message(ctx, *clientF, pane, msg)
	}
	return nil
}
