package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/picker"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
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
		Name:    "close",
		Args:    "[--session S | --pane %N] [--client C]",
		Summary: "cmd.close",
		Run:     runClose,
	})
	register(&command{
		Name:   "__close-tab",
		Args:   "--pane %N [--client C]",
		Hidden: true,
		Run:    runCloseTab,
	})
	register(&command{
		Name:   "__open-picker",
		Args:   "[--client C]",
		Hidden: true,
		Run:    runOpenPicker,
	})
	register(&command{
		Name:    "tab",
		Args:    "<agent> [--pane %N] [--client C]",
		Summary: "cmd.tab",
		Run:     runTab,
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

// runTab opens a new tab (tmux window) in the pane's project with an agent,
// right after the pane's tab, and shows it (§9.3 "Tabs").
func runTab(e *Env, args []string) error {
	fs := newFlags(e, "tab")
	paneF := fs.String("pane", "", "a pane of the project (default: this pane)")
	clientF := fs.String("client", "", "client for messages")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "tab <agent>")}
	}
	id, err := parseAgentFlag("agent", pos[0])
	if err != nil {
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
	if !w.Installed(id) {
		err := &workspace.NotInstalledError{Agent: id}
		w.Message(ctx, *clientF, p.ID, err.Error())
		return err
	}
	np, err := w.NewWindow(ctx, p.ID, id, p.CWD)
	if err != nil {
		w.Message(ctx, *clientF, p.ID, err.Error())
		return err
	}
	fmt.Fprintln(e.Stdout, np)
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
	if dir == "" && !*fromOpt && isTerminal(os.Stdin) {
		// No folder given: pick one.
		if dir, err = pickFolder(ctx, w); err != nil || dir == "" {
			return err
		}
	}
	if dir == "" {
		return &UsageError{Msg: i18n.T("cli.usage", "open <folder>")}
	}
	return openFolder(ctx, w, dir, *clientF)
}

// runClose closes a project: its session, after moving the clients that
// show it to another project (§9.3 "Open and close projects").
func runClose(e *Env, args []string) error {
	fs := newFlags(e, "close")
	sessionF := fs.String("session", "", "the project's session (default: this pane's, or this folder's)")
	paneF := fs.String("pane", "", "a pane of the project")
	clientF := fs.String("client", "", "client for messages")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	s := *sessionF
	if s == "" {
		pane := *paneF
		if pane == "" {
			pane = os.Getenv("TMUX_PANE")
		}
		if pane != "" {
			out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", pane, "#{session_name}"})
			if err != nil {
				return err
			}
			s = strings.TrimSpace(out)
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			s = workspace.SessionName(cwd)
		}
	}
	pi, err := w.Project(ctx, s)
	if err != nil {
		return err
	}
	if err := w.CloseProject(ctx, s); err != nil {
		if *clientF != "" {
			w.Message(ctx, *clientF, "", err.Error())
		}
		return err
	}
	if *clientF != "" {
		w.Message(ctx, *clientF, "", i18n.T("close.done", pi.Name))
	} else {
		fmt.Fprintln(e.Stderr, i18n.T("close.done", pi.Name))
	}
	return nil
}

// runCloseTab closes the tab of --pane (the × of the tab bar, confirmed).
func runCloseTab(e *Env, args []string) error {
	fs := newFlags(e, "__close-tab")
	paneF := fs.String("pane", "", "a pane of the tab")
	clientF := fs.String("client", "", "client for messages")
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
	if err := w.CloseTab(ctx, p); err != nil {
		w.Message(ctx, *clientF, "", err.Error())
		return err
	}
	return nil
}

// runOpenPicker is the folder picker in a popup (the sidebar's + and o):
// the chosen folder opens as a project on the client.
func runOpenPicker(e *Env, args []string) error {
	fs := newFlags(e, "__open-picker")
	clientF := fs.String("client", "", "client to switch to the project")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	dir, err := pickFolder(ctx, w)
	if err != nil || dir == "" {
		return err
	}
	return openFolder(ctx, w, dir, *clientF)
}

// pickFolder runs the folder picker on this terminal: open, recent and git
// folders; the git scan runs while the list is shown (§9.3).
func pickFolder(ctx context.Context, w *workspace.WS) (string, error) {
	home := paths.UserHome()
	var open []string
	if out, err := w.T.Run(ctx, tmux.Cmd{"list-sessions", "-F", "#{session_path}"}); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if l = strings.TrimSpace(l); l != "" && l != home {
				open = append(open, l)
			}
		}
	}
	var recent []string
	if st, _ := state.Read(); st != nil {
		for _, r := range st.Recent {
			if r.Path != home {
				recent = append(recent, r.Path)
			}
		}
	}
	cached, fresh := folders.Cached(paths.FoldersCache(), now())
	var more chan []folders.Cand
	if !fresh {
		more = make(chan []folders.Cand, 1)
		go func() {
			repos := folders.Scan(home, folders.ScanOptions{})
			_ = folders.Store(paths.FoldersCache(), repos, now())
			more <- folders.Merge(open, recent, repos)
		}()
	}
	return picker.Run(picker.Options{
		In: os.Stdin, Out: os.Stdout, Home: home,
		Cands: folders.Merge(open, recent, cached), More: more,
	})
}

// openFolder opens dir as a project (its session, with the default agent
// and a sidebar) and shows it: on client, in this tmux client, or by
// attaching this terminal.
func openFolder(ctx context.Context, w *workspace.WS, dir, client string) error {
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		dir = filepath.Join(paths.UserHome(), strings.TrimPrefix(dir, "~"))
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		msg := i18n.T("open.missing", paths.Shorten(dir))
		if client != "" {
			w.Message(ctx, client, "", msg)
		}
		return errors.New(msg)
	}
	if err := w.EnsureConf(ctx); err != nil {
		return err
	}
	res, err := w.Start(ctx, "", dir)
	if err != nil {
		if client != "" {
			w.Message(ctx, client, "", err.Error())
		}
		return err
	}
	switch {
	case client != "":
		_, err = w.T.Run(ctx, tmux.Cmd{"switch-client", "-c", client, "-t", "=" + res.Session})
		w.Message(ctx, client, "", i18n.T("open.opened", paths.Shorten(dir)))
		return err
	case os.Getenv("TMUX") != "":
		_, err = w.T.Run(ctx, tmux.Cmd{"switch-client", "-t", "=" + res.Session})
		return err
	}
	return attachExec(w, res.Session)
}
