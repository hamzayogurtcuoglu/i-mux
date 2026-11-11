//go:build windows

package tmux

// psmux adapter.
//
// psmux runs every session in a server process of its own (one per session,
// in the -L namespace), where tmux has one server for all of them. imux was
// written for tmux's model: pane ids (%3) and window ids (@2) are unique on
// the server, global options and key bindings are shared by all sessions,
// and one batch of commands may touch several sessions. This file keeps that
// model for the rest of imux:
//
//   - Ids are made unique by naming their session: every #{pane_id} and
//     #{window_id} imux asks for is expanded as "%3~proj-1a2b", and every
//     #{client_name} as "<name>~<session>". imux stores and passes these
//     values around unchanged; nothing else in imux looks inside them.
//   - A command whose target (-t, -s, or -c where it names a client) carries
//     such an id goes to that session's server, with the bare id: psmux
//     routes a command by the control port in $TMUX, so the child gets a
//     $TMUX that names the session's port.
//   - Global settings (set -g / -s, set-hook -g, bind, unbind, source-file,
//     setenv -g) go to every session's server, and the set -g / -s values are
//     replayed into each session created later.
//   - list-clients asks every server; psmux itself merges list-sessions and
//     the -a lists.
//   - A command after split-window / new-window / new-session in the same
//     batch, without a target of its own, acts on the new pane, as on tmux.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/pwshshim"
)

// IDSep joins a psmux id to its session in imux's ids.
const IDSep = "~"

// psmuxMode reports whether c talks to psmux (always, unless IMUX_TMUX_BIN
// names another multiplexer).
func (c *Client) psmuxMode() bool {
	b := strings.ToLower(filepath.Base(c.Bin))
	return strings.HasPrefix(b, "psmux") || strings.HasPrefix(b, "pmux") || os.Getenv("IMUX_PSMUX") == "1"
}

var formatIDs = strings.NewReplacer(
	"#{pane_id}", "#{pane_id}"+IDSep+"#{session_name}",
	"#{window_id}", "#{window_id}"+IDSep+"#{session_name}",
	"#{client_name}", "#{client_name}"+IDSep+"#{client_session}",
	"#D", "#{pane_id}"+IDSep+"#{session_name}",
)

// GlobalizeFormat rewrites a format (or a whole config) so the pane, window
// and client ids it expands name their session.
func GlobalizeFormat(s string) string {
	if !strings.Contains(s, "#") {
		return s
	}
	return formatIDs.Replace(s)
}

// EnvPane is the pane imux runs in ($TMUX_PANE), as an imux id.
func EnvPane() string {
	p := os.Getenv("TMUX_PANE")
	if p == "" {
		return ""
	}
	if s := envSession(); s != "" {
		return p + IDSep + s
	}
	return p
}

// envSession is the session of the pane imux runs in.
func envSession() string {
	s := os.Getenv("PSMUX_SESSION")
	if s == "" {
		s = os.Getenv("PSMUX_TARGET_SESSION")
	}
	if _, after, ok := strings.Cut(s, "__"); ok {
		return after
	}
	return s
}

// SplitID returns the psmux id and its session of an imux id ("%3~s" gives
// "%3", "s"); an id without a session is returned as it is.
func SplitID(id string) (local, session string) {
	if i := strings.LastIndex(id, IDSep); i > 0 {
		return id[:i], id[i+len(IDSep):]
	}
	return id, ""
}

// --- the psmux data directory -------------------------------------------------

func psmuxDir() string {
	if d := os.Getenv("PSMUX_DATA_DIR"); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".psmux")
}

// sessionPort returns the control port of session's server ("" when it does
// not run).
func (c *Client) sessionPort(session string) string {
	b, err := os.ReadFile(filepath.Join(psmuxDir(), c.Socket+"__"+session+".port"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// liveSessions lists the sessions of this namespace whose server runs.
func (c *Client) liveSessions() []string {
	ents, err := os.ReadDir(psmuxDir())
	if err != nil {
		return nil
	}
	prefix := c.Socket + "__"
	var out []string
	for _, e := range ents {
		n := e.Name()
		if !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".port") {
			continue
		}
		s := strings.TrimSuffix(strings.TrimPrefix(n, prefix), ".port")
		if s == "" || strings.HasPrefix(s, "__") || strings.Contains(s, "__warm__") {
			continue
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// --- globals replayed into new sessions ---------------------------------------

var globalsMu sync.Mutex

func (c *Client) globalsPath() string {
	return filepath.Join(psmuxDir(), c.Socket+".imux-globals.json")
}

// rememberGlobal records a `set -g/-s` so sessions created later get it too.
func (c *Client) rememberGlobal(cmd Cmd) {
	key := globalKey(cmd)
	if key == "" {
		return
	}
	globalsMu.Lock()
	defer globalsMu.Unlock()
	m := c.readGlobals()
	if hasFlag(cmd, 'u') {
		delete(m, key)
	} else {
		m[key] = cmd
	}
	if b, err := json.Marshal(m); err == nil {
		_ = os.WriteFile(c.globalsPath(), b, 0o600)
	}
}

func (c *Client) readGlobals() map[string]Cmd {
	m := map[string]Cmd{}
	if b, err := os.ReadFile(c.globalsPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// globalKey names the option a set -g / -s writes ("" for -a appends, which
// would grow on every replay).
func globalKey(cmd Cmd) string {
	if hasFlag(cmd, 'a') {
		return ""
	}
	var flags string
	for _, a := range cmd[1:] {
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			flags += strings.Trim(a, "-")
			continue
		}
		return strings.Map(func(r rune) rune {
			if strings.ContainsRune("gsw", r) {
				return r
			}
			return -1
		}, flags) + " " + a
	}
	return ""
}

// --- running --------------------------------------------------------------------

// EnvRunner is a Runner that can also set the child's environment.
type EnvRunner interface {
	RunEnv(ctx context.Context, bin string, args, env []string) (stdout, stderr []byte, err error)
}

func (c *Client) runPsmux(ctx context.Context, cmds []Cmd) (string, error) {
	var out strings.Builder
	var newPane, newSession string
	for _, g := range plan(cmds) {
		switch {
		case g.broadcast:
			for _, s := range c.liveSessions() {
				if _, err := c.execRoute(ctx, s, g.cmds); err != nil && !errors.Is(err, ErrNoPane) {
					return out.String(), err
				}
			}
			for _, cmd := range g.cmds {
				if cmd[0] == "set-option" || cmd[0] == "set" || cmd[0] == "set-window-option" || cmd[0] == "setw" {
					c.rememberGlobal(cmd)
				}
			}
		case g.gather:
			live := c.liveSessions()
			if len(live) == 0 {
				return out.String(), &Error{Args: g.cmds[0], Kind: ErrNoServer}
			}
			for _, s := range live {
				o, _ := c.execRoute(ctx, s, g.cmds)
				out.WriteString(o)
			}
		default:
			cmds := g.cmds
			// Followers of a new pane act on it, as on tmux.
			if newPane != "" && !g.spans && (g.route == "" || g.route == newSession) {
				cmds = withTarget(cmds, newPane)
				g.route = newSession
			}
			var before *splitBefore
			if g.creates {
				if c.legacyPsmux() {
					cmds[0], before = takeBefore(cmds[0])
				}
				cmds = askNewPane(cmds)
			}
			cmds = c.targetedResizes(ctx, g.route, cmds)
			o, err := c.execRoute(ctx, g.route, cmds)
			if g.creates {
				var id string
				id, o = takeNewPane(o)
				newPane, newSession = SplitID(id)
				if (cmds[0][0] == "new-session" || cmds[0][0] == "new") && err == nil {
					// psmux may hand the session a standby server that
					// read the config before imux last wrote it.
					if c.Config != "" {
						_, _ = c.execRoute(ctx, newSession, []Cmd{{"source-file", c.Config}})
					}
					c.replayGlobals(ctx, newSession)
				}
				if before != nil && err == nil && newPane != "" {
					c.moveBefore(ctx, newSession, newPane, before)
				}
			}
			out.WriteString(o)
			if err != nil {
				return out.String(), err
			}
			if ps := labelPanes(cmds); len(ps) > 0 {
				c.refreshTitles(ctx, g.route, ps)
			}
		}
	}
	return out.String(), nil
}

// labelOpts are the pane options imux's pane-border-format shows.
func (c *Client) replayGlobals(ctx context.Context, session string) {
	globalsMu.Lock()
	m := c.readGlobals()
	globalsMu.Unlock()
	if len(m) == 0 || session == "" {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cmds := make([]Cmd, 0, len(m))
	for _, k := range keys {
		cmds = append(cmds, m[k])
	}
	_, _ = c.execRoute(ctx, session, cmds)
}

// execRoute runs cmds in one psmux invocation on session's server.
func (c *Client) execRoute(ctx context.Context, session string, cmds []Cmd) (string, error) {
	args := c.Argv(cmds...)
	c.record(args)
	to := c.Timeout
	if to <= 0 {
		to = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	env := routeEnv(c, session, cmds)
	var out, errb []byte
	var err error
	if r, ok := c.Runner.(EnvRunner); ok {
		out, errb, err = r.RunEnv(ctx, c.Bin, args, env)
	} else {
		r := c.Runner
		if r == nil {
			r = ExecRunner{}
		}
		out, errb, err = r.Run(ctx, c.Bin, args)
	}
	o := strings.ReplaceAll(string(out), "\r\n", "\n")
	if err != nil {
		if notFound(err) {
			return "", &Error{Args: args, Kind: ErrNotFound, Err: err}
		}
		return o, &Error{Args: args, Stderr: string(errb), Kind: c.classifyPsmux(cmds, string(errb)), Err: err}
	}
	return o, nil
}

// routeEnv is the environment that sends a psmux command to session's server.
func routeEnv(c *Client, session string, cmds []Cmd) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "TMUX", "TMUX_PANE", "PSMUX_TARGET_SESSION", "PSMUX_SESSION", "PSMUX_TARGET_FULL":
			continue
		}
		env = append(env, kv)
	}
	if d := pwshshim.Dir(); d != "" {
		env = withPathFirst(env, d)
	}
	// psmux's standby ("warm") servers are started from a server's own
	// console; a session that takes one over can then start no run-shell
	// command (Windows reports an invalid handle), so every key binding of
	// imux fails there. Each new session gets a server of its own instead.
	env = append(env, "PSMUX_NO_WARM=1")
	if session == "" || len(cmds) == 0 {
		return env
	}
	switch cmds[0][0] {
	case "new-session", "new", "attach-session", "attach", "has-session", "has", "kill-session":
		return env
	}
	if port := c.sessionPort(session); port != "" {
		env = append(env, "TMUX="+filepath.Join(os.TempDir(), "psmux", c.Socket)+","+port+",0")
	}
	return env
}

// withPathFirst puts dir at the front of env's PATH.
func withPathFirst(env []string, dir string) []string {
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "PATH") {
			found = true
			kv = k + "=" + dir + string(os.PathListSeparator) + v
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, "PATH="+dir)
	}
	return out
}

// classifyPsmux maps psmux's messages to imux's typed errors.
func (c *Client) classifyPsmux(cmds []Cmd, stderr string) error {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "can't find window: %"), strings.Contains(s, "can't find pane"):
		return ErrNoPane
	case strings.Contains(s, "can't find window"):
		return ErrNoPane
	case strings.Contains(s, "no server running"), strings.TrimSpace(s) == "" && len(cmds) > 0 && (cmds[0][0] == "has-session" || cmds[0][0] == "has"):
		if len(c.liveSessions()) == 0 {
			return ErrNoServer
		}
		return ErrNoSession
	case strings.Contains(s, "can't find session"):
		return ErrNoSession
	}
	return classify(stderr)
}

// RunEnv implements EnvRunner.
func (ExecRunner) RunEnv(ctx context.Context, bin string, args, env []string) ([]byte, []byte, error) {
	cmd := execCommand(ctx, bin, args)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = env
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

// psmuxArgs rewrites display-menu and display-popup into the forms psmux
// reads. Its display-menu knows -T, -x and -y (a number) only, takes every
// other word not starting with - as a menu item (so a flag's value became an
// item) and has no disabled items; its display-popup takes -c as the start
// directory, not the client. The client and pane were used for routing.
func (c *Client) spawnPsmux(extra []string, cmds []Cmd) error {
	gs := plan(cmds)
	if len(gs) == 0 {
		return nil
	}
	last := gs[len(gs)-1]
	if len(gs) > 1 {
		var before []Cmd
		for _, g := range gs[:len(gs)-1] {
			before = append(before, g.cmds...)
		}
		if _, err := c.runPsmux(context.Background(), before); err != nil {
			return err
		}
	}
	args := c.Argv(last.cmds...)
	c.record(args)
	cmd := exec.Command(c.Bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = append(routeEnv(c, last.route, last.cmds), localPaneEnv(extra)...)
	if err := cmd.Start(); err != nil {
		if notFound(err) {
			return &Error{Args: args, Kind: ErrNotFound, Err: err}
		}
		return &Error{Args: args, Err: err}
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// localPaneEnv is SpawnEnv's environment for the psmux client: TMUX_PANE
// names the pane as its own server does (%3, not imux's %3~session).
func localPaneEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "TMUX_PANE="); ok {
			local, _ := SplitID(v)
			kv = "TMUX_PANE=" + local
		}
		out = append(out, kv)
	}
	return out
}

// execCommand is a psmux child without a console window of its own.
func execCommand(ctx context.Context, bin string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}
