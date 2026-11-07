package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// launchNew starts a run in a new pane created by create (new-session,
// new-window or split-window with -P -F '#{pane_id}'), setting the pane
// options in the same exec so a fast-exiting agent cannot race them.
func (w *WS) launchNew(ctx context.Context, l Launch, create func(cmd []string) tmux.Cmd) (string, error) {
	spec, te, err := w.Spec(l)
	if err != nil {
		return "", err
	}
	p, err := writeSpec(spec)
	if err != nil {
		return "", err
	}
	cmds := []tmux.Cmd{create(w.execCmd(p))}
	cmds = append(cmds, optCmds("", te, Trail{te}, l.CWD, w.Label(l.Agent))...)
	out, err := w.T.Run(ctx, cmds...)
	if err != nil {
		os.Remove(p)
		return "", err
	}
	pane := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	if !strings.HasPrefix(pane, "%") {
		return "", fmt.Errorf("tmux: unexpected new pane id %q", pane)
	}
	return pane, nil
}

// HasSession reports whether the named session exists.
func (w *WS) HasSession(ctx context.Context, name string) (bool, error) {
	_, err := w.T.Run(ctx, tmux.Cmd{"has-session", "-t", "=" + name})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, tmux.ErrNoServer) || errors.Is(err, tmux.ErrNoSession) {
		return false, nil
	}
	return false, err
}

// StartResult says what Start did.
type StartResult struct {
	Session string
	Pane    string // the new pane ("" when an existing session was found)
	Created bool
}

// Start creates the project's session with agent id in cwd, or finds the
// existing one. The caller attaches afterwards.
func (w *WS) Start(ctx context.Context, id session.AgentID, cwd string) (*StartResult, error) {
	name := SessionName(cwd)
	ok, err := w.HasSession(ctx, name)
	if err != nil {
		return nil, err
	}
	if ok {
		EnsureWatcher(w)
		_ = state.Update(func(s *state.State) error { s.AddRecent(cwd, w.Now()); return nil })
		return &StartResult{Session: name}, nil
	}
	if id == "" {
		if id, err = w.DefaultAgent(); err != nil {
			return nil, err
		}
	}
	pane, err := w.launchNew(ctx, Launch{Agent: id, CWD: cwd}, func(cmd []string) tmux.Cmd {
		return append(tmux.Cmd{"new-session", "-d", "-s", name, "-c", cwd, "-P", "-F", "#{pane_id}"}, cmd...)
	})
	if err != nil {
		return nil, err
	}
	w.maybeSidebar(ctx, pane)
	EnsureWatcher(w)
	_ = state.Update(func(s *state.State) error { s.AddRecent(cwd, w.Now()); return nil })
	return &StartResult{Session: name, Pane: pane, Created: true}, nil
}

// NewWindow opens a window with agent id next to the caller's pane (imux
// invoked inside the workspace).
func (w *WS) NewWindow(ctx context.Context, fromPane string, id session.AgentID, cwd string) (string, error) {
	if id == "" {
		var err error
		if id, err = w.DefaultAgent(); err != nil {
			return "", err
		}
	}
	win, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", fromPane, "#{window_id}"})
	if err != nil {
		return "", err
	}
	pane, err := w.launchNew(ctx, Launch{Agent: id, CWD: cwd}, func(cmd []string) tmux.Cmd {
		return append(tmux.Cmd{"new-window", "-a", "-t", strings.TrimSpace(win), "-c", cwd, "-P", "-F", "#{pane_id}"}, cmd...)
	})
	if err == nil {
		w.maybeSidebar(ctx, pane)
	}
	return pane, err
}

// Split opens a pane with agent id next to pane, in the same directory.
func (w *WS) Split(ctx context.Context, pane string, id session.AgentID, cwd string, horizontal bool) (string, error) {
	dir := "-v"
	if horizontal {
		dir = "-h"
	}
	return w.launchNew(ctx, Launch{Agent: id, CWD: cwd}, func(cmd []string) tmux.Cmd {
		return append(tmux.Cmd{"split-window", dir, "-t", pane, "-c", cwd, "-P", "-F", "#{pane_id}"}, cmd...)
	})
}

// SessionInfo is one imux tmux session.
type SessionInfo struct {
	Name     string
	Attached int
	Activity int64
	Panes    []*Pane
}

// Sessions lists sessions with their panes, most recently active first. A
// server that is not running yields none.
func (w *WS) Sessions(ctx context.Context) ([]*SessionInfo, error) {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-sessions", "-F", "#{session_name}\t#{session_attached}\t#{session_activity}"})
	if errors.Is(err, tmux.ErrNoServer) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	byName := map[string]*SessionInfo{}
	var list []*SessionInfo
	for _, r := range tmux.Rows(out, 3) {
		s := &SessionInfo{Name: r[0]}
		s.Attached, _ = strconv.Atoi(r[1])
		s.Activity, _ = strconv.ParseInt(r[2], 10, 64)
		byName[s.Name] = s
		list = append(list, s)
	}
	panes, err := w.Panes(ctx)
	if err != nil && !errors.Is(err, tmux.ErrNoServer) {
		return nil, err
	}
	for _, p := range panes {
		if p.IsSidebar() {
			continue // part of the UI, not a pane of the project
		}
		if s := byName[p.Session]; s != nil {
			s.Panes = append(s.Panes, p)
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Activity > list[j].Activity })
	return list, nil
}

// AttachArgv returns the tmux command line that attaches to name.
func (w *WS) AttachArgv(name string) []string {
	return append(append([]string{w.T.Bin}, w.T.BaseArgs()...), "attach-session", "-t", "="+name)
}

// ExecAttach replaces the process with `tmux attach`. TMUX is cleared so that
// attaching from inside another tmux works (nesting is intentional then).
func (w *WS) ExecAttach(name string) error {
	argv := w.AttachArgv(name)
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("%w: %s", ErrTmuxMissing, i18n.T("tmux.missing"))
	}
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "TMUX=") && !strings.HasPrefix(kv, "TMUX_PANE=") {
			env = append(env, kv)
		}
	}
	return execFunc(bin, argv, env)
}

// execFunc is syscall.Exec; tests replace it.
var execFunc = syscall.Exec
