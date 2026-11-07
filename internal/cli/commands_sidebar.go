package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func init() {
	register(&command{
		Name:    "rename",
		Args:    "<name> [--pane %N]",
		Summary: "cmd.rename",
		Run:     runRename,
	})
	register(&command{
		Name:    "open",
		Args:    "<folder> [--client C]",
		Summary: "cmd.open",
		Run:     runOpen,
	})
}

// runRename names a pane ("" clears the name); the sidebar and the pane
// border show it.
func runRename(e *Env, args []string) error {
	fs := newFlags(e, "rename")
	paneF := fs.String("pane", "", "pane to name (default: this pane)")
	clientF := fs.String("client", "", "client for the message")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(strings.Join(pos, " "))
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
	c := tmux.Cmd{"set-option", "-p", "-t", p.ID, sidebar.OptName, name}
	if name == "" {
		c = tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, sidebar.OptName}
	}
	if _, err := w.T.Run(ctx, c); err != nil {
		return err
	}
	w.RefreshSidebars(ctx)
	if *clientF != "" {
		w.Message(ctx, *clientF, p.ID, i18n.T("rename.done", name))
	}
	return nil
}

// runOpen opens a folder as a project (its own session, with the default
// agent and a sidebar) and switches the client to it. --from-option takes
// the folder from @imux_open, where the sidebar's prompt put it (the answer
// never goes through a shell).
func runOpen(e *Env, args []string) error {
	fs := newFlags(e, "open")
	clientF := fs.String("client", "", "client to switch to the project")
	fromOpt := fs.Bool("from-option", false, "read the folder from the @imux_open option")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	dir := strings.Join(pos, " ")
	if *fromOpt {
		out, _ := w.T.Run(ctx, tmux.Cmd{"show-options", "-gqv", "@imux_open"})
		_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-gu", "@imux_open"})
		dir = strings.TrimSpace(out)
	}
	if dir == "" {
		return &UsageError{Msg: i18n.T("cli.usage", "open <folder>")}
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		msg := i18n.T("open.missing", paths.Shorten(dir))
		if *clientF != "" {
			w.Message(ctx, *clientF, "", msg)
		}
		return errors.New(msg)
	}
	if err := w.EnsureConf(ctx); err != nil {
		return err
	}
	res, err := w.Start(ctx, "", dir)
	if err != nil {
		if *clientF != "" {
			w.Message(ctx, *clientF, "", err.Error())
		}
		return err
	}
	switch {
	case *clientF != "":
		_, err = w.T.Run(ctx, tmux.Cmd{"switch-client", "-c", *clientF, "-t", "=" + res.Session})
		w.Message(ctx, *clientF, "", i18n.T("open.opened", paths.Shorten(dir)))
		return err
	case os.Getenv("TMUX") != "":
		_, err = w.T.Run(ctx, tmux.Cmd{"switch-client", "-t", "=" + res.Session})
		return err
	}
	return attachExec(w, res.Session)
}
