package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
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
	cmds = append(cmds, optCmds("", te, append(append(Trail{}, l.Before...), te), l.CWD, w.Label(l.Agent))...)
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
		// Opened before: bring its tabs and agents back as they were (D24).
		if snap := LoadSnapshots().Find(cwd); snap != nil && snap.Agents() > 0 {
			if res, err := w.Restore(ctx, sources.New(w.Cfg), snap); err == nil {
				_ = state.Update(func(s *state.State) error { s.AddRecent(cwd, w.Now()); return nil })
				return res, nil
			}
		}
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
func (w *WS) Split(ctx context.Context, pane string, id session.AgentID, cwd string, horizontal, before bool) (string, error) {
	return w.launchNew(ctx, Launch{Agent: id, CWD: cwd}, func(cmd []string) tmux.Cmd {
		return append(splitCmd(pane, cwd, horizontal, before), cmd...)
	})
}

// splitCmd splits pane: side by side or one above the other, the new pane
// after it (right, below) or before it (left, above).
func splitCmd(pane, cwd string, horizontal, before bool) tmux.Cmd {
	c := tmux.Cmd{"split-window", "-v"}
	if horizontal {
		c[1] = "-h"
	}
	if before {
		c = append(c, "-b")
	}
	return append(c, "-t", pane, "-c", cwd, "-P", "-F", "#{pane_id}")
}

// SideBySide is the narrowest pane a new agent opens beside rather than
// below: each half keeps 60 columns.
const SideBySide = 120

// StartPrompt opens agent id with prompt as its first message, named name
// (an agent writing a skill, §9.3 "Skills"): beside p (below it when p is
// narrower than SideBySide), or in p's place when p is the project's "No
// agents" pane. It returns the agent's pane.
func (w *WS) StartPrompt(ctx context.Context, p *Pane, id session.AgentID, prompt, name string) (string, error) {
	l := Launch{Agent: id, CWD: p.CWD, Prompt: prompt}
	pane := p.ID
	if p.IsEmpty() {
		if err := w.respawn(ctx, p, l, nil, ""); err != nil {
			return "", err
		}
	} else {
		dir := "-h"
		if out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", p.ID, "#{pane_width}"}); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil && n < SideBySide {
				dir = "-v"
			}
		}
		np, err := w.launchNew(ctx, l, func(cmd []string) tmux.Cmd {
			return append(tmux.Cmd{"split-window", dir, "-t", p.ID, "-c", p.CWD, "-P", "-F", "#{pane_id}"}, cmd...)
		})
		if err != nil {
			return "", err
		}
		pane = np
	}
	if name != "" {
		_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", pane, sidebar.OptName, name})
	}
	w.RefreshSidebars(ctx)
	if w.pasteMode(l) {
		if err := w.WaitStable(ctx, pane); err != nil {
			return pane, err
		}
		if err := w.Paste(ctx, pane, prompt, true); err != nil {
			return pane, err
		}
	}
	return pane, nil
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
		if p.IsSidebar() || p.IsEmpty() {
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
	// The terminal this client runs in: the sidebars pick their icons for
	// it (termfont). Inside tmux TERM_PROGRAM is tmux's, not the terminal's.
	// Windows Terminal sets no TERM_PROGRAM, only WT_SESSION (passed to WSL).
	prog := os.Getenv("TERM_PROGRAM")
	if prog == "" && os.Getenv("WT_SESSION") != "" {
		prog = "WindowsTerminal"
	}
	if prog != "" && os.Getenv("TMUX") == "" && prog != "tmux" {
		_, _ = w.T.Run(context.Background(), tmux.Cmd{"set-option", "-g", "@imux_terminal", prog})
	}
	if os.Getenv("TMUX") == "" {
		// VS Code's terminal (also over a tunnel): pages open where VS Code
		// runs, through its BROWSER helper (D47); another terminal: not.
		b, ipc := VSCodeBrowser(os.Getenv)
		set := func(opt, v string) tmux.Cmd {
			if v == "" {
				return tmux.Cmd{"set-option", "-gu", opt}
			}
			return tmux.Cmd{"set-option", "-g", opt, v}
		}
		_, _ = w.T.Run(context.Background(), set(OptBrowser, b), set(OptVSCodeIPC, ipc))
	}
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

// Options a VS Code terminal leaves when it attaches (§9.3 "Live preview
// in the browser"): its BROWSER helper, which opens a page where VS Code
// runs (forwarding the port), and the IPC socket the helper needs.
const (
	OptBrowser   = "@imux_browser"
	OptVSCodeIPC = "@imux_vscode_ipc"
)

// VSCodeBrowser is what ExecAttach keeps of a terminal's environment: its
// BROWSER when it is VS Code's helper (browser.sh; browser.cmd on Windows),
// and VSCODE_IPC_HOOK_CLI; "" for other terminals.
func VSCodeBrowser(getenv func(string) string) (helper, ipc string) {
	b, i := getenv("BROWSER"), getenv("VSCODE_IPC_HOOK_CLI")
	h := filepath.ToSlash(b)
	if i == "" || !(strings.HasSuffix(h, "/helpers/browser.sh") || strings.HasSuffix(h, "/helpers/browser.cmd")) {
		return "", ""
	}
	return b, i
}

// execFunc is sysx.Exec; tests replace it.
var execFunc = sysx.Exec

// ShellAgent is the name menus and commands use for a plain terminal.
const ShellAgent = "shell"

// SplitShell opens the user's shell (no agent) next to pane, in cwd.
func (w *WS) SplitShell(ctx context.Context, pane, cwd string, horizontal, before bool) (string, error) {
	out, err := w.T.Run(ctx, splitCmd(pane, cwd, horizontal, before))
	if err != nil {
		return "", err
	}
	w.RefreshSidebars(ctx)
	return strings.TrimSpace(out), nil
}

// NewShellWindow opens a tab with the user's shell after fromPane's tab,
// with a sidebar.
func (w *WS) NewShellWindow(ctx context.Context, fromPane, cwd string) (string, error) {
	win, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", fromPane, "#{window_id}"})
	if err != nil {
		return "", err
	}
	out, err := w.T.Run(ctx, tmux.Cmd{"new-window", "-a", "-t", strings.TrimSpace(win), "-c", cwd, "-P", "-F", "#{pane_id}"})
	if err != nil {
		return "", err
	}
	pane := strings.TrimSpace(out)
	w.maybeSidebar(ctx, pane)
	return pane, nil
}
