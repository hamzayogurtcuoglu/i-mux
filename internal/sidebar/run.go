package sidebar

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/fileops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/selfexec"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Options configure a sidebar process.
type Options struct {
	Source *Source
	Self   string // the imux binary, for actions
	Pane   string // this sidebar's pane id ($TMUX_PANE)
	Width  int    // columns the sidebar keeps
	In     *os.File
	Out    io.Writer
	// Safety is the redraw interval without events (default 15 s).
	Safety time.Duration
}

// sidebar is a running sidebar's state.
type sidebar struct {
	o      Options
	t      *tmux.Client
	model  Model
	win    Window
	screen Screen
	sel    Item // keyboard selection, NoItem = none
	scroll int  // first row of the explorer or changes list

	clicks     chan Action // click actions, delivered after ClickDelay
	lastClick  time.Time
	lastClickY int // row of the last click

	hx, hy     int // the pointer (1-based; hx 0: not over the sidebar)
	px, py     int // the motion report before it
	moves      int // motion reports so far (the way-out check's turn)
	idle       chan int
	selected   string // the explorer's selected path (relative)
	menuX      int    // where the last right-click was (the menu opens there)
	menuY      int
	revealKey  string // i18n key of "Reveal in Finder" & co., "" for none
	lastReveal string // OptReveal as last seen
	revealSeen bool
	revealRow  string // a revealed path to scroll to
	lastFrame  string // the frame last written
}

// ClickDelay holds a click's action until tmux's double-click window (300 ms)
// has passed. tmux reports a second click within it as DoubleClick1Pane
// later on, and that binding selects the sidebar pane again: it would close
// a menu the first click opened and take the focus back from the agent pane.
// A click on the same row within ClickIgnore of the previous one is a double
// click and is dropped, so an action never runs twice; a quick click on
// another row (a file right after the view's icon) still counts.
var (
	ClickDelay  = 350 * time.Millisecond
	ClickIgnore = 400 * time.Millisecond
	SpinEvery   = 120 * time.Millisecond
)

// Run draws the sidebar until its window has no other pane, stdin closes or
// it is told to stop. It sleeps between events: tmux hooks, imux commands and
// the watcher send SIGUSR1 to redraw it.
func Run(ctx context.Context, o Options) error {
	if o.Safety <= 0 {
		o.Safety = 15 * time.Second
	}
	s := &sidebar{o: o, t: o.Source.T, sel: NoItem, clicks: make(chan Action, 4), idle: make(chan int, 4), revealKey: (&fileops.System{}).Reveal()}
	restore, err := rawMode(int(o.In.Fd()))
	if err == nil {
		defer restore()
	} else {
		restore = nil
	}
	// Alternate screen, no cursor, SGR mouse reports for presses and every
	// motion (the hover, D31).
	_, _ = io.WriteString(o.Out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1003h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(o.Out, "\x1b[?1003l\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()

	_, _ = s.t.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", o.Pane, OptRole, RoleSidebar},
		tmux.Cmd{"set-option", "-p", "-t", o.Pane, OptPID, strconv.Itoa(os.Getpid())},
		tmux.Cmd{"set-option", "-p", "-t", o.Pane, "remain-on-exit", "off"},
		tmux.Cmd{"set-option", "-p", "-t", o.Pane, "@imux_label", "i-mux"})

	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGUSR1, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	defer signal.Stop(sigs)

	input := make(chan []byte, 8)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := o.In.Read(buf)
			if n > 0 {
				b := make([]byte, n)
				copy(b, buf[:n])
				input <- b
			}
			if err != nil {
				close(input)
				return
			}
		}
	}()

	stamp := selfexec.Take()
	upgrade := func() {
		// A new imux binary: restart with it in this same pane.
		if stamp.Changed() {
			if restore != nil {
				restore()
			}
			_ = stamp.Exec(os.Args)
		}
	}
	tick := time.NewTicker(o.Safety)
	defer tick.Stop()
	// The spinner of working agents turns from the cached model: no tmux
	// exec, only while one is shown.
	spin := time.NewTicker(SpinEvery)
	defer spin.Stop()
	if s.redraw(ctx) {
		return nil
	}
	s.keepWidth(ctx)
	var pending []byte
	for {
		select {
		case <-ctx.Done():
			return nil
		case sig := <-sigs:
			switch sig {
			case syscall.SIGUSR1:
				upgrade()
			case syscall.SIGWINCH:
				s.lastFrame = "" // the pane was resized: draw it whole
				s.keepWidth(ctx)
			default:
				return nil
			}
		case <-tick.C:
			upgrade()
		case <-spin.C:
			if s.model.Animated() {
				s.model.Tick++
				s.draw()
			}
			continue
		case n := <-s.idle:
			// No motion since: was the pointer on its way out?
			if n == s.moves && s.hx != 0 && Leaving(s.px, s.py, s.hx, s.hy, s.win.Width, s.win.Height) {
				s.hover(0, 0)
			}
			continue
		case a := <-s.clicks:
			s.do(ctx, a)
			if isMenu(a.Kind) {
				continue // nothing changed yet; a redraw would only draw under the menu
			}
		case b, ok := <-input:
			if !ok {
				return nil
			}
			var evs []Event
			evs, pending = Parse(append(pending, b...), true)
			gather := false
			for _, ev := range evs {
				// The pointer moving and a right-click (its menu) change
				// nothing to gather.
				if ev.Kind != EvMove && ev.Kind != EvRightClick {
					gather = true
				}
				s.handle(ctx, ev)
			}
			if !gather {
				continue // the pointer moved: drawn from the model, no tmux
			}
		}
		if s.redraw(ctx) {
			return nil
		}
	}
}

// redraw gathers and draws; it reports whether the sidebar should exit (its
// window has no other pane).
func (s *sidebar) redraw(ctx context.Context) bool {
	// Settings changed the config: read it again (icons, width ...).
	if s.o.Source.Reload() && s.o.Source.Cfg.Tmux.SidebarWidth > 0 {
		s.o.Width = s.o.Source.Cfg.Tmux.SidebarWidth
	}
	m, w, err := s.o.Source.Gather(ctx, s.o.Pane)
	if err != nil {
		return false // keep the last frame; tmux may be busy
	}
	if w.Alone {
		return true
	}
	// A resize the SIGWINCH did not fix (tmux grows panes in proportion
	// when a bigger client attaches): take the width back.
	if want := s.width(m.Closed); want > 0 && w.Width != want && w.WinW >= want+20 {
		_, _ = s.t.Run(ctx, tmux.Cmd{"resize-pane", "-t", s.o.Pane, "-x", strconv.Itoa(want)})
	}
	if s.reveal(w.Reveal) {
		// Folders opened for a new, renamed or pasted path: list them.
		if m2, w2, err := s.o.Source.Gather(ctx, s.o.Pane); err == nil {
			m, w = m2, w2
		}
	}
	m.Tick, m.Scroll = s.model.Tick, s.scroll
	s.model, s.win = m, w
	s.keepVisible()
	s.draw()
	return false
}

// draw renders the cached model.
func (s *sidebar) draw() {
	s.model.Selected = s.selected
	s.screen = Render(s.model, s.win.Width, s.win.Height, s.sel)
	if it := s.screen.HoverAt(s.hx, s.hy, s.win.Width); it != NoItem && it != s.sel {
		Paint(s.screen, it, hoverBg, s.win.Width)
	}
	f := Frame(s.screen, s.win.Width, s.win.Height)
	if f == s.lastFrame {
		// The same frame again (a refresh after a hook): writing it would
		// make tmux draw the pane again, over an open menu too.
		return
	}
	s.lastFrame = f
	_, _ = io.WriteString(s.o.Out, f)
}

// isMenu reports actions that open a menu.
func isMenu(k ActionKind) bool {
	switch k {
	case ActFileMenu, ActProjectMenu, ActAgentMenu, ActBranchMenu, ActSettings:
		return true
	}
	return false
}

// LeaveAfter is how long the pointer may rest before the way-out check.
var LeaveAfter = 150 * time.Millisecond

// Leaving reports whether a pointer that moved from (px, py) to (x, y) and
// stopped was on its way out of a w×h sidebar: the same step again lands
// outside. tmux reports no motion outside the pane, so a fast exit ends on
// a report inside it.
func Leaving(px, py, x, y, w, h int) bool {
	if px == 0 {
		return false
	}
	nx, ny := x+(x-px), y+(y-py)
	return nx < 1 || nx > w || ny < 1 || ny > h
}

// moved records a motion report and schedules the way-out check.
func (s *sidebar) moved(x, y int) {
	s.px, s.py = s.hx, s.hy
	s.moves++
	n := s.moves
	time.AfterFunc(LeaveAfter, func() {
		select {
		case s.idle <- n:
		default:
		}
	})
	s.hover(x, y)
}

// hover follows the pointer; it redraws only when the item under it changes.
func (s *sidebar) hover(x, y int) {
	before := s.screen.HoverAt(s.hx, s.hy, s.win.Width)
	s.hx, s.hy = x, y
	if s.screen.HoverAt(x, y, s.win.Width) != before {
		s.draw()
	}
}

// width is the sidebar's width: the configured one, or the rail's while the
// panel is closed.
func (s *sidebar) width(closed bool) int {
	if closed {
		return ClosedWidth
	}
	return s.o.Width
}

// keepWidth restores the sidebar's width after the window was resized,
// unless the window is too narrow to keep it.
func (s *sidebar) keepWidth(ctx context.Context) {
	w := s.width(s.model.Closed)
	if w <= 0 {
		return
	}
	out, err := s.t.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", s.o.Pane, "#{pane_width}\t#{window_width}"})
	if err != nil {
		return
	}
	rows := tmux.Rows(out, 2)
	if len(rows) == 0 {
		return
	}
	pw, _ := strconv.Atoi(rows[0][0])
	ww, _ := strconv.Atoi(rows[0][1])
	if pw != w && ww >= w+20 {
		_, _ = s.t.Run(ctx, tmux.Cmd{"resize-pane", "-t", s.o.Pane, "-x", strconv.Itoa(w)})
	}
}

// handle reacts to one key or click.
func (s *sidebar) handle(ctx context.Context, ev Event) {
	if ev.Kind == EvMove {
		s.moved(ev.X, ev.Y)
		return
	}
	if ev.Kind == EvScroll {
		n := len(s.model.Tree)
		if s.model.View == ViewChanges && s.model.SCM != nil {
			n = len(s.model.SCM.Staged) + len(s.model.SCM.Changes) + len(s.model.SCM.History) + 3
		}
		if ev.Key == "up" {
			s.scroll = max(s.scroll-3, 0)
		} else {
			s.scroll = min(s.scroll+3, max(n-1, 0))
		}
		s.model.Scroll = s.scroll
		s.draw()
		return
	}
	if ev.Kind == EvRightClick {
		// A right-click on a project opens its menu (go to it, close it).
		row := ev.Y - 1
		if row < 0 || row >= len(s.screen.Lines) {
			return
		}
		var menu Action
		switch a := s.screen.Lines[row].ActionAt(ev.X - 1); a.Kind {
		case ActOpenFile:
			menu = Action{Kind: ActFileMenu, Path: a.Path, Section: MenuFile}
			s.selected = a.Path
		case ActToggleDir:
			menu = Action{Kind: ActFileMenu, Path: a.Path, Section: MenuFolder}
			s.selected = a.Path
		case ActNone, ActSearch:
			// The explorer's empty space and title: the project folder.
			if s.model.View != ViewExplorer || s.model.Closed || ev.X <= railW {
				return
			}
			menu = Action{Kind: ActFileMenu, Section: MenuRoot}
		case ActProject, ActProjectMenu:
			menu = Action{Kind: ActProjectMenu, Session: a.Session}
		case ActFocus, ActAgentMenu:
			menu = Action{Kind: ActAgentMenu, Pane: a.Pane}
		default:
			return
		}
		s.menuX, s.menuY = ev.X, ev.Y
		if menu.Kind == ActFileMenu {
			s.draw() // the row the menu is about, selected
		}
		if s.t.MenuTakesMouse(ctx) {
			// The menu ignores the button's release (-O): open it now.
			s.do(ctx, menu)
			return
		}
		// Older tmux: wait until the button is released, or the release
		// closes the menu.
		time.AfterFunc(ClickDelay, func() { s.clicks <- menu })
		return
	}
	if ev.Kind == EvClick {
		row := ev.Y - 1
		if row < 0 || row >= len(s.screen.Lines) {
			return
		}
		now := time.Now()
		if ev.Y == s.lastClickY && now.Sub(s.lastClick) < ClickIgnore {
			return
		}
		s.lastClick, s.lastClickY = now, ev.Y
		s.sel = NoItem
		a := s.screen.Lines[row].ActionAt(ev.X - 1)
		if a.Kind == ActOpenFile || a.Kind == ActToggleDir {
			s.selected = a.Path
		}
		time.AfterFunc(ClickDelay, func() { s.clicks <- a })
		return
	}
	switch ev.Key {
	case "up", "k":
		s.move(-1)
	case "down", "j", "tab":
		s.move(1)
	case "backtab":
		s.move(-1)
	case "enter":
		s.do(ctx, s.screen.ActionOf(s.sel))
	case "esc", "q":
		s.sel = NoItem
		s.focusTarget(ctx)
	case "v":
		s.do(ctx, Action{Kind: ActSplitH})
	case "-":
		s.do(ctx, Action{Kind: ActSplitV})
	case "s":
		s.do(ctx, Action{Kind: ActSessions})
	case "l":
		s.do(ctx, Action{Kind: ActLimits})
	case "?":
		s.do(ctx, Action{Kind: ActKeys})
	case ",":
		s.do(ctx, Action{Kind: ActSettings})
	case "n":
		s.do(ctx, Action{Kind: ActRename})
	case "o":
		s.do(ctx, Action{Kind: ActOpen})
	case "f":
		s.do(ctx, Action{Kind: ActChanges})
	case "1":
		s.do(ctx, Action{Kind: ActView, Section: ViewAgents})
	case "2":
		s.do(ctx, Action{Kind: ActView, Section: ViewExplorer})
	case "3", "/":
		s.do(ctx, Action{Kind: ActSearch})
	case "4":
		s.do(ctx, Action{Kind: ActView, Section: ViewChanges})
	default:
		for _, a := range s.model.Agents {
			if a.Key == ev.Key {
				s.do(ctx, Action{Kind: ActSwitch, Agent: a.ID})
				return
			}
		}
	}
}

// move steps the keyboard selection over the rows that have an action.
func (s *sidebar) move(d int) {
	rows := s.screen.Items()
	if len(rows) == 0 {
		return
	}
	at := -1
	for i, r := range rows {
		if r == s.sel {
			at = i
		}
	}
	switch {
	case at < 0 && d > 0:
		at = 0
	case at < 0:
		at = len(rows) - 1
	default:
		at = (at + d + len(rows)) % len(rows)
	}
	s.sel = rows[at]
}

func (s *sidebar) focusTarget(ctx context.Context) {
	if s.model.Target != "" {
		_, _ = s.t.Run(ctx, tmux.Cmd{"select-pane", "-t", s.model.Target})
	}
}

// do runs an action. Commands that need the target pane run as imux
// subcommands (the same code paths as menus and clicks); focus goes back to
// the target pane so typing continues there.
func (s *sidebar) do(ctx context.Context, a Action) {
	target, client := s.model.Target, s.win.Client
	tgt := []string{"--pane", target}
	if client != "" {
		tgt = append(tgt, "--client", client)
	}
	switch a.Kind {
	case ActNone:
		return
	case ActFocus:
		s.focus(ctx, a.Pane, client)
		return
	case ActProject:
		if client != "" {
			_, _ = s.t.Run(ctx, tmux.Cmd{"switch-client", "-c", client, "-t", "=" + a.Session})
		}
		return
	case ActOpen:
		// The folder picker in a popup (D22).
		c := tmux.Cmd{"display-popup"}
		if client != "" {
			c = append(c, "-c", client)
		}
		args := []string{s.o.Self, "__open-picker"}
		if client != "" {
			args = append(args, "--client", client)
		}
		_ = s.t.Spawn(append(c, "-E", "-w", "80%", "-h", "70%", tmux.ShJoin(args...)))
		return
	case ActView:
		// Show another view in this sidebar (@imux_view, per window). The
		// active view's icon closes the panel; any view's icon opens it.
		s.scroll = 0
		if s.win.ID == "" {
			return
		}
		cur := s.model.View
		if cur == "" {
			cur = ViewAgents
		}
		setView := tmux.Cmd{"set-option", "-w", "-t", s.win.ID, OptView, a.Section}
		switch {
		case s.model.Closed:
			s.model.Closed = false
			cmds := []tmux.Cmd{setView, {"set-option", "-w", "-u", "-t", s.win.ID, OptClosed}}
			if w := s.width(false); w > 0 {
				cmds = append(cmds, tmux.Cmd{"resize-pane", "-t", s.o.Pane, "-x", strconv.Itoa(w)})
			}
			_, _ = s.t.Run(ctx, cmds...)
		case a.Section == cur:
			s.model.Closed = true
			_, _ = s.t.Run(ctx,
				tmux.Cmd{"set-option", "-w", "-t", s.win.ID, OptClosed, "1"},
				tmux.Cmd{"resize-pane", "-t", s.o.Pane, "-x", strconv.Itoa(ClosedWidth)})
		default:
			_, _ = s.t.Run(ctx, setView)
		}
		return
	case ActToggleDir:
		if s.o.Source.Expanded == nil {
			s.o.Source.Expanded = map[string]bool{}
		}
		if s.o.Source.Expanded[a.Path] {
			delete(s.o.Source.Expanded, a.Path)
		} else {
			s.o.Source.Expanded[a.Path] = true
		}
		return
	case ActOpenFile:
		s.popup(client, "90%", "90%", "__edit", a.Path)
		return
	case ActFileMenu:
		s.fileMenu(ctx, client, a, s.menuX, s.menuY)
		return
	case ActGit:
		s.spawn(s.gitArgs(a.Section, client, a.Status == "?", a.Path))
		return
	case ActGitConfirm:
		s.confirmDiscard(ctx, client, a)
		return
	case ActCommit:
		s.prompt(ctx, client, i18n.T("prompt.commit"), "",
			"set-option -g @imux_git \"%%%\" ; "+tmux.RunShell(s.o.Self, s.gitArgs("commit", client, false)...)+"")
		return
	case ActPasteImage:
		if s.model.Target != "" {
			args := []string{"__paste-image", "--pane", s.model.Target}
			if client != "" {
				args = append(args, "--client", client)
			}
			s.spawn(args)
		}
		return
	case ActSettings:
		args := []string{"__settings", "menu"}
		if client != "" {
			args = append(args, "--client", client)
		}
		if s.model.Target != "" {
			args = append(args, "--pane", s.model.Target)
		}
		s.spawn(args)
		return
	case ActSuggest:
		args := s.gitArgs("suggest", client, false)
		if s.model.Target != "" {
			args = append(args, "--pane", s.model.Target)
		}
		s.spawn(args)
		return
	case ActBranchMenu:
		s.branchMenu(ctx, client)
		return
	case ActGitPopup:
		args := []string{"__git", a.Section, "--cwd", s.model.CWD}
		if a.Path != "" {
			args = append(args, "--", a.Path)
		}
		s.popup(client, "80%", "70%", args...)
		return
	case ActRefresh:
		return
	case ActSearch:
		s.popup(client, "90%", "90%", "__search")
		return
	case ActFold:
		// Fold or unfold a section in every sidebar (@imux_fold, global).
		var names []string
		for _, n := range []string{SecProjects, SecAgents, SecChanges, SecStaged, SecUnstaged, SecHistory} {
			if f := s.model.Fold[n]; f != (n == a.Section) {
				names = append(names, n)
			}
		}
		_, _ = s.t.Run(ctx, tmux.Cmd{"set-option", "-g", "@imux_fold", strings.Join(names, ",")})
		RefreshAll(ctx, s.t)
		return
	case ActDiff:
		s.diff(a, client)
		return
	case ActAgentMenu:
		args := []string{"__menu", "--agent", "--pane", a.Pane}
		if client != "" {
			args = append(args, "--client", client)
		}
		s.spawn(args)
		return
	case ActProjectMenu:
		args := []string{"__menu", "--project", a.Session}
		if target != "" {
			args = append(args, "--pane", target)
		}
		if client != "" {
			args = append(args, "--client", client)
		}
		s.spawn(args)
		return
	}
	if target == "" {
		return
	}
	s.focusTarget(ctx)
	switch a.Kind {
	case ActSwitch:
		s.spawn(append([]string{"__click", "imux:" + string(a.Agent)}, tgt...))
	case ActSplitH:
		s.spawn(append([]string{"__menu", "--split", "h"}, tgt...))
	case ActSplitV:
		s.spawn(append([]string{"__menu", "--split", "v"}, tgt...))
	case ActNewAgent:
		// Side by side when the pane is wide (a cell is about twice as
		// tall as wide), else one above the other.
		dir := "v"
		if s.model.TargetW >= 2*s.model.TargetH {
			dir = "h"
		}
		s.spawn(append([]string{"__menu", "--split", dir}, tgt...))
	case ActSessions:
		s.spawn(append([]string{"__menu", "--sessions"}, tgt...))
	case ActLimits:
		s.spawn(append([]string{"__menu", "--limits"}, tgt...))
	case ActKeys:
		s.spawn(append([]string{"__menu", "--keys"}, tgt...))
	case ActRename:
		cur := ""
		for _, p := range s.model.Panes {
			if p.ID == target {
				cur = p.Name
			}
		}
		_, _ = s.t.Run(ctx, RenameCmds(s.o.Self, target, client, cur, i18n.T("prompt.rename"))...)
	case ActChanges:
		s.spawn(append([]string{"__menu", "--changes"}, tgt...))
	}
}

// focus selects pane id, switching the client to its project and window
// when it is elsewhere.
func (s *sidebar) focus(ctx context.Context, id, client string) {
	for _, p := range s.model.Panes {
		if p.ID == id {
			_, _ = s.t.Run(ctx, tmux.Cmd{"select-pane", "-t", id})
			return
		}
	}
	cmds := []tmux.Cmd{{"select-window", "-t", id}, {"select-pane", "-t", id}}
	if client != "" {
		cmds = append([]tmux.Cmd{{"switch-client", "-c", client, "-t", id}}, cmds...)
	}
	_, _ = s.t.Run(ctx, cmds...)
}

// prompt asks on the client's command prompt and runs template with the
// answer. The answer only ever reaches tmux, escaped by %%%: never a shell.
func (s *sidebar) prompt(ctx context.Context, client, label, initial, template string) {
	c := tmux.Cmd{"command-prompt"}
	if client != "" {
		c = append(c, "-t", client)
	}
	c = append(c, "-I", initial, "-p", label, template)
	_, _ = s.t.Run(ctx, c)
}

// diff opens a popup with a changed file's diff.
func (s *sidebar) diff(a Action, client string) {
	c := tmux.Cmd{"display-popup"}
	if client != "" {
		c = append(c, "-c", client)
	}
	c = append(c, "-E", "-w", "95%", "-h", "90%")
	if s.model.CWD != "" {
		c = append(c, "-d", tmux.EscapeFormat(s.model.CWD)) // -d is a format
	}
	sh := DiffShell(s.o.Self, a.Status, a.Path)
	if a.Section == "staged" || a.Section == "worktree" {
		sh = tmux.ShJoin(s.o.Self, "__diff", "--"+a.Section, a.Status, a.Path)
	}
	_ = s.t.Spawn(append(c, sh))
}

// gitArgs is `imux __git op` for the project folder (and paths).
func (s *sidebar) gitArgs(op, client string, untracked bool, paths ...string) []string {
	args := []string{"__git", op, "--cwd", s.model.CWD}
	if client != "" {
		args = append(args, "--client", client)
	}
	if op == "commit" || op == "new-branch" {
		args = append(args, "--from-option")
	}
	if untracked {
		args = append(args, "--untracked")
	}
	if len(paths) > 0 {
		args = append(append(args, "--"), paths...)
	}
	return args
}

// menu shows a display-menu on client.
func (s *sidebar) menu(ctx context.Context, client, title string, items ...string) {
	c := s.t.Menu(ctx)
	if client != "" {
		c = append(c, "-c", client)
	}
	c = append(append(c, "-T", tmux.EscapeFormat(title), "-x", "C", "-y", "C", "--"), items...)
	_ = s.t.Spawn(c)
}

// confirmDiscard asks before throwing changes away (they cannot be had back).
func (s *sidebar) confirmDiscard(ctx context.Context, client string, a Action) {
	title := i18n.T("git.discard_all_title")
	var args []string
	if a.Section == "discard-all" {
		args = s.gitArgs("discard-all", client, false)
	} else {
		title = i18n.T("git.discard_title", a.Path)
		args = s.gitArgs("discard", client, a.Status == "?", a.Path)
	}
	s.menu(ctx, client, title,
		tmux.EscapeFormat(i18n.T("git.discard_yes")), "y", tmux.RunShell(s.o.Self, args...),
		tmux.EscapeFormat(i18n.T("click.cancel")), "n", "")
}

// branchMenu lists the local branches (checkout) and New branch.
func (s *sidebar) branchMenu(ctx context.Context, client string) {
	bs, cur, err := gitops.Branches(ctx, s.model.CWD)
	if err != nil {
		return
	}
	var items []string
	for i, b := range bs {
		if i >= 20 {
			break
		}
		name := "  " + b
		cmd := tmux.RunShell(s.o.Self, append(s.gitArgs("checkout", client, false), b)...)
		if b == cur {
			name, cmd = "✓ "+b, ""
		}
		items = append(items, tmux.EscapeFormat(name), "", cmd)
	}
	newCmd := "command-prompt"
	if client != "" {
		newCmd += " -t " + tmux.Quote(client)
	}
	newCmd += " -p " + tmux.Quote(i18n.T("prompt.branch")) + " " +
		tmux.Quote("set-option -g @imux_git \"%%%\" ; "+tmux.RunShell(s.o.Self, s.gitArgs("new-branch", client, false)...))
	items = append(items, "", tmux.EscapeFormat("+ "+i18n.T("git.new_branch")), "n", newCmd)
	s.menu(ctx, client, i18n.T("git.branches"), items...)
}

// popup runs an imux command in a popup in the project folder.
func (s *sidebar) popup(client, w, h string, args ...string) {
	c := tmux.Cmd{"display-popup"}
	if client != "" {
		c = append(c, "-c", client)
	}
	c = append(c, "-E", "-w", w, "-h", h)
	if s.model.CWD != "" {
		c = append(c, "-d", tmux.EscapeFormat(s.model.CWD)) // -d is a format
	}
	_ = s.t.Spawn(append(c, tmux.ShJoin(append([]string{s.o.Self}, args...)...)))
}

// DiffShell is the sh command that shows a changed file's diff: the whole
// file with the changes highlighted (imux __diff, D26).
func DiffShell(self, status, path string) string {
	return tmux.ShJoin(self, "__diff", status, path)
}

// spawn starts imux with args without waiting for it.
func (s *sidebar) spawn(args []string) {
	cmd := exec.Command(s.o.Self, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}
