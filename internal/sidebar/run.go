package sidebar

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
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
	sel    int // keyboard selection (row index), -1 = none

	clicks    chan Action // click actions, delivered after ClickDelay
	lastClick time.Time
}

// ClickDelay holds a click's action until tmux's double-click window (300 ms)
// has passed. tmux reports a second click within it as DoubleClick1Pane
// later on, and that binding selects the sidebar pane again: it would close
// a menu the first click opened and take the focus back from the agent pane.
// A click within ClickIgnore of the previous one is a double click and is
// dropped, so an action never runs twice.
var (
	ClickDelay  = 350 * time.Millisecond
	ClickIgnore = 400 * time.Millisecond
)

// Run draws the sidebar until its window has no other pane, stdin closes or
// it is told to stop. It sleeps between events: tmux hooks, imux commands and
// the watcher send SIGUSR1 to redraw it.
func Run(ctx context.Context, o Options) error {
	if o.Safety <= 0 {
		o.Safety = 15 * time.Second
	}
	s := &sidebar{o: o, t: o.Source.T, sel: -1, clicks: make(chan Action, 4)}
	restore, err := rawMode(int(o.In.Fd()))
	if err == nil {
		defer restore()
	}
	// Alternate screen, no cursor, SGR mouse reports for button presses.
	_, _ = io.WriteString(o.Out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(o.Out, "\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()

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

	tick := time.NewTicker(o.Safety)
	defer tick.Stop()
	s.keepWidth(ctx)
	if s.redraw(ctx) {
		return nil
	}
	var pending []byte
	for {
		select {
		case <-ctx.Done():
			return nil
		case sig := <-sigs:
			switch sig {
			case syscall.SIGUSR1:
			case syscall.SIGWINCH:
				s.keepWidth(ctx)
			default:
				return nil
			}
		case <-tick.C:
		case a := <-s.clicks:
			s.do(ctx, a)
		case b, ok := <-input:
			if !ok {
				return nil
			}
			var evs []Event
			evs, pending = Parse(append(pending, b...), true)
			for _, ev := range evs {
				s.handle(ctx, ev)
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
	m, w, err := s.o.Source.Gather(ctx, s.o.Pane)
	if err != nil {
		return false // keep the last frame; tmux may be busy
	}
	if w.Alone {
		return true
	}
	s.model, s.win = m, w
	s.screen = Render(m, w.Width, w.Height, s.sel)
	_, _ = io.WriteString(s.o.Out, Frame(s.screen, w.Height))
	return false
}

// keepWidth restores the sidebar's width after the window was resized,
// unless the window is too narrow to keep it.
func (s *sidebar) keepWidth(ctx context.Context) {
	w := s.o.Width
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
	if ev.Kind == EvClick {
		row := ev.Y - 1
		if row < 0 || row >= len(s.screen.Lines) {
			return
		}
		now := time.Now()
		if now.Sub(s.lastClick) < ClickIgnore {
			return
		}
		s.lastClick = now
		s.sel = -1
		a := s.screen.Lines[row].Action
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
		if s.sel >= 0 && s.sel < len(s.screen.Lines) {
			s.do(ctx, s.screen.Lines[s.sel].Action)
		}
	case "esc", "q":
		s.sel = -1
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
	case "n":
		s.do(ctx, Action{Kind: ActRename})
	case "o":
		s.do(ctx, Action{Kind: ActOpen})
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
	rows := s.screen.Selectable()
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
		s.prompt(ctx, client, i18n.T("prompt.open"), "~/",
			"set-option -g @imux_open \"%%%\" ; "+tmux.RunShell(s.o.Self, "open", "--from-option", "--client", client))
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
		s.prompt(ctx, client, i18n.T("prompt.rename"), cur,
			"set-option -p -t "+target+" "+OptName+" \"%%%\" ; "+tmux.RunShell(s.o.Self, "__sidebar-refresh"))
	case ActDiff:
		s.diff(a, client)
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

// diff opens a popup with the changes of a file (the file itself when git
// does not track it yet).
func (s *sidebar) diff(a Action, client string) {
	var sh string
	if a.Status == "?" {
		sh = tmux.ShJoin("less", "-R", "--", a.Path)
	} else {
		d := func(args ...string) string {
			return tmux.ShJoin(append([]string{"git", "diff", "--color=always"}, args...)...)
		}
		sh = "{ " + d("HEAD", "--", a.Path) + " 2>/dev/null || " + d("--", a.Path) + "; } | less -R"
	}
	c := tmux.Cmd{"display-popup"}
	if client != "" {
		c = append(c, "-c", client)
	}
	c = append(c, "-E", "-w", "90%", "-h", "90%")
	if s.model.CWD != "" {
		c = append(c, "-d", s.model.CWD)
	}
	_ = s.t.Spawn(append(c, sh))
}

// spawn starts imux with args without waiting for it.
func (s *sidebar) spawn(args []string) {
	cmd := exec.Command(s.o.Self, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}
