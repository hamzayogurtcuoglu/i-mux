package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/picker"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
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
		Name:   "__close-pane",
		Args:   "--pane %N [--client C]",
		Hidden: true,
		Run:    runClosePane,
	})
	register(&command{
		Name:   "__open-picker",
		Args:   "[--client C]",
		Hidden: true,
		Run:    runOpenPicker,
	})
	register(&command{
		Name:    "tab",
		Args:    "<agent|shell> [--pane %N] [--client C]",
		Summary: "cmd.tab",
		Run:     runTab,
	})
	register(&command{
		Name:    "open",
		Args:    "[<folder>] [-d] [--client C]",
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
		return &UsageError{Msg: i18n.T("cli.usage", "tab <agent|shell>")}
	}
	var id session.AgentID
	shell := pos[0] == workspace.ShellAgent
	if !shell {
		if id, err = parseAgentFlag("agent", pos[0]); err != nil {
			return err
		}
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
	if shell {
		np, err := w.NewShellWindow(ctx, p.ID, p.CWD)
		if err != nil {
			w.Message(ctx, *clientF, p.ID, err.Error())
			return err
		}
		fmt.Fprintln(e.Stdout, np)
		return nil
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
	detach := fs.Bool("d", false, "open the project but do not show it")
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
		c, ok, err := pickFolder(ctx, w)
		if err != nil || !ok {
			return err
		}
		return openChoice(ctx, w, c, *clientF)
	}
	if dir == "" {
		return &UsageError{Msg: i18n.T("cli.usage", "open <folder>")}
	}
	if *detach {
		return openDetached(ctx, w, dir, e)
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

// runClosePane closes the agent of --pane (the agent menu, confirmed).
func runClosePane(e *Env, args []string) error {
	fs := newFlags(e, "__close-pane")
	paneF := fs.String("pane", "", "the agent's pane")
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
	if err := w.CloseAgent(ctx, &workspace.Pane{ID: pane}); err != nil {
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
	c, ok, err := pickFolder(ctx, w)
	if err != nil || !ok {
		return err
	}
	return openChoice(ctx, w, c, *clientF)
}

// pickFolder runs the folder picker on this terminal: the restore row,
// open and recent projects (with their saved tabs), then git folders; the
// git scan runs while the list is shown (§9.3).
func pickFolder(ctx context.Context, w *workspace.WS) (folders.Cand, bool, error) {
	home := paths.UserHome()
	t := now()
	openSet := map[string]bool{}
	var openC []folders.Cand
	if out, err := w.T.Run(ctx, tmux.Cmd{"list-sessions", "-F", "#{session_path}"}); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if l = strings.TrimSpace(l); l != "" && l != home && !openSet[l] {
				openSet[l] = true
				openC = append(openC, folders.Cand{Path: l, Kind: folders.KindOpen})
			}
		}
	}
	snaps := workspace.LoadSnapshots()
	type rec struct {
		c  folders.Cand
		at int64
	}
	var recs []rec
	for _, p := range snaps.Projects {
		if openSet[p.Dir] || p.Dir == home {
			continue
		}
		d := ageText(t, time.Unix(p.At, 0))
		if p.Agents() > 0 {
			d = projectSummary(p) + " · " + d
		}
		recs = append(recs, rec{folders.Cand{Path: p.Dir, Kind: folders.KindRecent, Detail: d}, p.At})
	}
	if st, _ := state.Read(); st != nil {
		for _, r := range st.Recent {
			if !openSet[r.Path] && r.Path != home && snaps.Find(r.Path) == nil {
				recs = append(recs, rec{folders.Cand{Path: r.Path, Kind: folders.KindRecent, Detail: ageText(t, time.Unix(r.At, 0))}, r.At})
			}
		}
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].at > recs[j].at })
	recentC := make([]folders.Cand, len(recs))
	for i, r := range recs {
		recentC[i] = r.c
	}
	var restore *folders.Cand
	if n := len(snaps.Interrupted(w.ServerID(ctx), openSet)); n > 0 {
		restore = &folders.Cand{Kind: folders.KindRestore, Label: i18n.T("folders.restore"), Detail: i18n.T("folders.restore_n", n)}
	}
	asGit := func(ps []string) []folders.Cand {
		out := make([]folders.Cand, len(ps))
		for i, p := range ps {
			out[i] = folders.Cand{Path: p, Kind: folders.KindGit}
		}
		return out
	}
	cached, fresh := folders.Cached(paths.FoldersCache(), t)
	var more chan []folders.Cand
	if !fresh {
		more = make(chan []folders.Cand, 1)
		go func() {
			repos := folders.Scan(home, folders.ScanOptions{})
			_ = folders.Store(paths.FoldersCache(), repos, now())
			more <- folders.MergeCands(openC, recentC, asGit(repos))
		}()
	}
	return picker.Run(picker.Options{
		In: os.Stdin, Out: os.Stdout, Home: home, Restore: restore,
		Cands: folders.MergeCands(openC, recentC, asGit(cached)), More: more,
	})
}

// projectSummary is "3 tabs · 4 agents".
func projectSummary(p workspace.ProjectSnap) string {
	plural := func(n int, one, many string) string {
		if n == 1 {
			return i18n.T(one)
		}
		return i18n.T(many, n)
	}
	return p.Summary(func(n int) string { return plural(n, "folders.tab", "folders.tabs") },
		func(n int) string { return plural(n, "folders.agent", "folders.agents") })
}

// ageText is how long ago t was: "<1m", "9m", "2h", "3d".
func ageText(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	}
	return strconv.Itoa(int(d/(24*time.Hour))) + "d"
}

// openChoice opens what the picker returned: a folder, or every project of
// the last session.
func openChoice(ctx context.Context, w *workspace.WS, c folders.Cand, client string) error {
	if c.Kind != folders.KindRestore {
		return openFolder(ctx, w, c.Path, client)
	}
	open := map[string]bool{}
	if out, err := w.T.Run(ctx, tmux.Cmd{"list-sessions", "-F", "#{session_path}"}); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			open[strings.TrimSpace(l)] = true
		}
	}
	snaps := workspace.LoadSnapshots()
	f := sources.New(w.Cfg)
	if err := w.EnsureConf(ctx); err != nil {
		return err
	}
	first := ""
	for _, p := range snaps.Interrupted(w.ServerID(ctx), open) {
		res, err := w.Restore(ctx, f, &p)
		if err != nil {
			continue
		}
		if first == "" {
			first = res.Session
		}
	}
	if first == "" {
		return errors.New(i18n.T("folders.restore_failed"))
	}
	return showSession(ctx, w, first, client)
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
	if client != "" {
		w.Message(ctx, client, "", i18n.T("open.opened", paths.Shorten(dir)))
	}
	return showSession(ctx, w, res.Session, client)
}

// openDetached opens dir as a project without showing it (imux open -d).
func openDetached(ctx context.Context, w *workspace.WS, dir string, e *Env) error {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return errors.New(i18n.T("open.missing", paths.Shorten(dir)))
	}
	if err := w.EnsureConf(ctx); err != nil {
		return err
	}
	res, err := w.Start(ctx, "", dir)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, res.Session)
	return nil
}

// showSession shows session s: on client, in this tmux client, or by
// attaching this terminal.
func showSession(ctx context.Context, w *workspace.WS, s, client string) error {
	var err error
	switch {
	case client != "":
		_, err = w.T.Run(ctx, tmux.Cmd{"switch-client", "-c", client, "-t", "=" + s})
		return err
	case os.Getenv("TMUX") != "":
		_, err = w.T.Run(ctx, tmux.Cmd{"switch-client", "-t", "=" + s})
		return err
	}
	return attachExec(w, s)
}
