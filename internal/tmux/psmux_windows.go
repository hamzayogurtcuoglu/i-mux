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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
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

// --- planning a batch -----------------------------------------------------------

type group struct {
	route     string // session whose server runs the commands; "" = psmux's choice
	cmds      []Cmd
	broadcast bool // every live session
	gather    bool // every live session, outputs joined
	creates   bool // a pane-creating command that untargeted followers act on
}

// targetFlags lists, per command, the flags whose value is a target.
func targetFlags(name string) string {
	switch name {
	case "display-menu", "menu", "display-popup", "popup", "display-message", "display",
		"switch-client", "switchc", "refresh-client", "refresh", "detach-client", "detach",
		"show-messages", "showmsgs", "command-prompt", "confirm-before", "confirm",
		"choose-tree", "choose-client", "choose-buffer", "list-keys":
		return "tsc"
	}
	return "ts"
}

var creators = map[string]bool{"split-window": true, "splitw": true, "new-window": true, "neww": true, "new-session": true, "new": true}

func isBroadcast(cmd Cmd) bool {
	switch cmd[0] {
	case "set-option", "set", "set-window-option", "setw":
		return !hasValueFlag(cmd, "-t") && (hasFlag(cmd, 'g') || hasFlag(cmd, 's'))
	case "set-hook":
		return hasFlag(cmd, 'g')
	case "bind-key", "bind", "unbind-key", "unbind", "source-file", "source":
		return true
	case "set-environment", "setenv":
		return hasFlag(cmd, 'g')
	}
	return false
}

// hasFlag reports whether a single-letter flag appears among cmd's leading
// flag words (combined forms such as -gq count).
func hasFlag(cmd Cmd, f byte) bool {
	for i := 1; i < len(cmd); i++ {
		a := cmd[i]
		if a == "--" || len(a) < 2 || a[0] != '-' {
			return false
		}
		if len(a) == 2 && valueFlag(cmd[0], a[1]) {
			if a[1] == f {
				return true
			}
			i++ // the flag's value
			continue
		}
		if strings.IndexByte(a[1:], f) >= 0 {
			return true
		}
	}
	return false
}

// valueFlags lists, per command, the flags that take a value.
var valueFlags = map[string]string{
	"split-window": "ltcFepS", "splitw": "ltcFepS",
	"new-window": "ntcFeS", "neww": "ntcFeS",
	"new-session": "sncexyFtf", "new": "sncexyFtf",
	"respawn-pane": "tce", "respawnp": "tce",
	"respawn-window": "tce", "respawnw": "tce",
	"display-popup": "wxyhdctTsSbe", "popup": "wxyhdctTsSbe",
	"display-menu": "tcTxyHsSbC", "menu": "tcTxyHsSbC",
	"display-message": "tcd", "display": "tcd",
	"run-shell": "td", "run": "td",
	"command-prompt": "tIpT", "confirm-before": "tpc", "confirm": "tpc",
	"set-option": "t", "set": "t", "set-window-option": "t", "setw": "t",
	"show-options": "t", "show": "t", "set-hook": "t", "show-hooks": "t",
	"bind-key": "TN", "bind": "TN", "unbind-key": "T", "unbind": "T",
	"list-panes": "tF", "lsp": "tF", "list-windows": "tF", "lsw": "tF",
	"list-sessions": "Ff", "ls": "Ff", "list-clients": "tF", "lsc": "tF",
	"capture-pane": "tSEb", "capturep": "tSEb",
	"resize-pane": "txy", "resizep": "txy",
	"swap-pane": "st", "swapp": "st", "join-pane": "stl", "joinp": "stl",
	"move-pane": "stl", "break-pane": "stnF", "breakp": "stnF",
	"switch-client": "tcT", "switchc": "tcT",
	"send-keys": "tN", "send": "tN", "select-pane": "tT", "selectp": "tT",
	"kill-pane": "t", "killp": "t", "kill-window": "t", "killw": "t",
	"kill-session": "t", "has-session": "t", "has": "t",
	"attach-session": "tc", "attach": "tc", "select-window": "t", "selectw": "t",
	"rename-window": "t", "renamew": "t", "rename-session": "t", "rename": "t",
	"set-environment": "t", "setenv": "t", "show-environment": "t", "showenv": "t",
	"pipe-pane": "t", "pipep": "t", "refresh-client": "tcCBAl", "refresh": "tcCBAl",
	"source-file": "t", "source": "t", "if-shell": "t", "if": "t",
	"select-layout": "t", "selectl": "t", "wait-for": "", "detach-client": "tsE",
}

// valueFlag reports whether flag takes a value for the command.
func valueFlag(name string, f byte) bool {
	fl, ok := valueFlags[name]
	if !ok {
		fl = "tsFc"
	}
	return strings.IndexByte(fl, f) >= 0
}

// argvForm makes the trailing words of a pane-creating command (the program
// and its arguments) psmux's argv form (-- prog args): psmux would otherwise
// join them into one command line for the shell.
func argvForm(cmd Cmd) Cmd {
	switch cmd[0] {
	case "split-window", "splitw", "new-window", "neww", "new-session", "new", "respawn-pane", "respawnp", "respawn-window", "respawnw":
	default:
		return cmd
	}
	i := 1
	for i < len(cmd) {
		a := cmd[i]
		if a == "--" {
			return cmd
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		if len(a) == 2 && valueFlag(cmd[0], a[1]) {
			i += 2
			continue
		}
		i++
	}
	if len(cmd)-i < 2 {
		return cmd // no program, or one command line for the shell
	}
	out := append(Cmd(nil), cmd[:i]...)
	out = append(out, "--")
	return append(out, cmd[i:]...)
}

func hasValueFlag(cmd Cmd, flag string) bool {
	for i := 1; i+1 < len(cmd); i++ {
		if cmd[i] == flag {
			return true
		}
	}
	return false
}

// localTarget rewrites one target value to psmux's form and returns the
// session it names ("" when it names none).
func localTarget(v string) (string, string) {
	if !strings.Contains(v, IDSep) {
		// =name: or name:... names a session
		s := strings.TrimPrefix(v, "=")
		if i := strings.IndexByte(s, ':'); i > 0 && !strings.HasPrefix(s, "%") && !strings.HasPrefix(s, "@") {
			return v, s[:i]
		}
		return v, ""
	}
	session := ""
	out := idRe.ReplaceAllStringFunc(v, func(m string) string {
		local, s := SplitID(m)
		if session == "" {
			session = s
		}
		return local
	})
	return out, session
}

// idRe matches an imux id inside a target ("%3~s", "@2~s", "name~s").
var idRe = regexp.MustCompile(`[^\s:.,=~]+~[A-Za-z0-9_.\-]+`)

// plan splits a batch into groups that each go to one server.
func plan(cmds []Cmd) []group {
	var gs []group
	cur := -1
	lastRoute := ""
	for _, raw := range cmds {
		cmd := make(Cmd, len(raw))
		for i, a := range raw {
			cmd[i] = GlobalizeFormat(a)
		}
		if len(cmd) == 0 {
			continue
		}
		cmd = argvForm(cmd)
		if isBroadcast(cmd) {
			gs = append(gs, group{cmds: []Cmd{cmd}, broadcast: true})
			cur = -1
			continue
		}
		if cmd[0] == "list-clients" || cmd[0] == "lsc" {
			gs = append(gs, group{cmds: []Cmd{cmd}, gather: true})
			cur = -1
			continue
		}
		route := ""
		flags := targetFlags(cmd[0])
		for i := 1; i+1 < len(cmd); i++ {
			a := cmd[i]
			if a == "--" || len(a) < 2 || a[0] != '-' {
				break
			}
			if len(a) != 2 || !valueFlag(cmd[0], a[1]) {
				continue
			}
			if strings.IndexByte(flags, a[1]) >= 0 {
				local, s := localTarget(cmd[i+1])
				cmd[i+1] = local
				if s != "" && route == "" {
					route = s
				}
			}
			i++
		}
		if cmd[0] == "new-session" || cmd[0] == "new" {
			if s := valueOf(cmd, "-s"); s != "" {
				route = s
			}
		}
		cmd = psmuxArgs(cmd)
		if route == "" {
			route = lastRoute
		}
		if cur < 0 || gs[cur].route != route || gs[cur].creates || creators[cmd[0]] {
			gs = append(gs, group{route: route, creates: creators[cmd[0]]})
			cur = len(gs) - 1
		}
		gs[cur].cmds = append(gs[cur].cmds, cmd)
		lastRoute = route
	}
	return gs
}

func valueOf(cmd Cmd, flag string) string {
	for i := 1; i+1 < len(cmd); i++ {
		if cmd[i] == flag {
			return cmd[i+1]
		}
	}
	return ""
}

// newPaneSep ends the new pane's id that the adapter asks a creating command
// to print before the caller's own format.
const newPaneSep = "\t\timux-new\t" // printable: psmux prints control bytes as octal

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
			if newPane != "" && (g.route == "" || g.route == newSession) {
				cmds = withTarget(cmds, newPane)
				g.route = newSession
			}
			var before *splitBefore
			if g.creates {
				cmds[0], before = takeBefore(cmds[0])
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
var labelOpts = map[string]bool{"@imux_label": true, "@imux_name": true, "@imux_flag": true}

// labelPanes lists the panes whose label options cmds change.
func labelPanes(cmds []Cmd) []string {
	var ps []string
	for _, cmd := range cmds {
		if cmd[0] != "set-option" && cmd[0] != "set" {
			continue
		}
		t := valueOf(cmd, "-t")
		if t == "" || !hasFlag(cmd, 'p') {
			continue
		}
		for _, a := range cmd[1:] {
			if labelOpts[a] {
				ps = append(ps, t)
				break
			}
		}
	}
	return ps
}

// refreshTitles writes imux's border label as the panes' titles:
// " name · label flag ", as pane-border-format does on tmux.
func (c *Client) refreshTitles(ctx context.Context, route string, panes []string) {
	seen := map[string]bool{}
	for _, p := range panes {
		if seen[p] {
			continue
		}
		seen[p] = true
		o, err := c.execRoute(ctx, route, []Cmd{{"display-message", "-p", "-t", p,
			"#{@imux_name}\t#{@imux_label}\t#{@imux_flag}\t#{pane_current_command}"}})
		f := strings.Split(strings.TrimRight(o, "\r\n"), "\t")
		if err != nil || len(f) != 4 {
			continue
		}
		title := f[1]
		if title == "" {
			title = f[3]
		}
		if f[0] != "" {
			title = f[0] + " · " + title
		}
		if f[2] != "" {
			title += " " + f[2]
		}
		_, _ = c.execRoute(ctx, route, []Cmd{{"select-pane", "-t", p, "-T", title}})
	}
}

// splitBefore is a split-window -b: psmux has no -b, so the new pane is
// moved before its target afterwards and given its size again.
type splitBefore struct {
	target     string // the pane split (psmux id; "" = the active one)
	size       string // -l value ("" = none)
	horizontal bool
}

// takeBefore removes -b from a split-window and says what to do after it.
func takeBefore(cmd Cmd) (Cmd, *splitBefore) {
	if cmd[0] != "split-window" && cmd[0] != "splitw" {
		return cmd, nil
	}
	i := indexOf(cmd, "-b")
	if i < 0 || i > flagsEnd(cmd) {
		return cmd, nil
	}
	out := append(append(Cmd(nil), cmd[:i]...), cmd[i+1:]...)
	return out, &splitBefore{target: valueOf(out, "-t"), size: valueOf(out, "-l"), horizontal: indexOf(out, "-h") > 0}
}

// flagsEnd is the index of the first word after cmd's flags.
func flagsEnd(cmd Cmd) int {
	i := 1
	for i < len(cmd) {
		a := cmd[i]
		if a == "--" || len(a) < 2 || a[0] != '-' {
			return i
		}
		if len(a) == 2 && valueFlag(cmd[0], a[1]) {
			i += 2
			continue
		}
		i++
	}
	return i
}

// moveBefore puts a new pane before (left of, or above) the pane it split.
func (c *Client) moveBefore(ctx context.Context, session, pane string, b *splitBefore) {
	target := b.target
	if target == "" {
		out, err := c.execRoute(ctx, session, []Cmd{{"display-message", "-p", "#{pane_id}"}})
		if err != nil {
			return
		}
		target = strings.TrimSpace(out)
	}
	cmds := []Cmd{{"swap-pane", "-d", "-s", pane, "-t", target}}
	if b.size != "" {
		axis := "-y"
		if b.horizontal {
			axis = "-x"
		}
		cmds = append(cmds, Cmd{"resize-pane", "-t", pane, axis, b.size})
	}
	_, _ = c.execRoute(ctx, session, c.targetedResizes(ctx, session, cmds))
}

// targetedResizes makes resize-pane -t P -x/-y act on P: psmux resizes the
// active pane whatever -t says, so P is made active for the resize and the
// active pane is given back afterwards.
func (c *Client) targetedResizes(ctx context.Context, route string, cmds []Cmd) []Cmd {
	var out []Cmd
	for _, cmd := range cmds {
		if (cmd[0] != "resize-pane" && cmd[0] != "resizep") || indexOf(cmd, "-t") < 0 ||
			(indexOf(cmd, "-x") < 0 && indexOf(cmd, "-y") < 0) {
			out = append(out, cmd)
			continue
		}
		t := valueOf(cmd, "-t")
		info, err := c.execRoute(ctx, route, []Cmd{{"display-message", "-p", "-t", t, "#{window_id}\t#{window_width}\t#{window_height}"}})
		f := strings.Split(strings.TrimSpace(info), "\t")
		if err != nil || len(f) != 3 {
			out = append(out, cmd)
			continue
		}
		active, _ := c.execRoute(ctx, route, []Cmd{{"list-panes", "-t", f[0], "-F", "#{?pane_active,#{pane_id},}"}})
		prev := strings.TrimSpace(active)
		out = append(out, Cmd{"select-pane", "-t", t}, percentResize(cmd, f[1], f[2]))
		if prev != "" && prev != t {
			out = append(out, Cmd{"select-pane", "-t", prev})
		}
	}
	return out
}

// percentResize turns resize-pane's -x / -y cells into the share of the
// window psmux takes the number as.
func percentResize(cmd Cmd, w, h string) Cmd {
	out := append(Cmd(nil), cmd...)
	for i := 1; i+1 < len(out); i++ {
		total := ""
		switch out[i] {
		case "-x":
			total = w
		case "-y":
			total = h
		default:
			continue
		}
		cells, err1 := strconv.Atoi(out[i+1])
		t, err2 := strconv.Atoi(total)
		if err1 != nil || err2 != nil || t <= 1 {
			continue
		}
		pct := (cells*100 + (t-1)/2) / (t - 1)
		out[i+1] = strconv.Itoa(max(1, min(99, pct)))
	}
	return out
}

// askNewPane makes the creating command print the new pane's id first.
func askNewPane(cmds []Cmd) []Cmd {
	cmd := append(Cmd(nil), cmds[0]...)
	id := "#{pane_id}" + IDSep + "#{session_name}" + newPaneSep
	switch {
	case indexOf(cmd, "-F") > 0:
		i := indexOf(cmd, "-F")
		cmd[i+1] = id + cmd[i+1]
	case indexOf(cmd, "-P") > 0:
		// tmux's own -P format
		cmd = append(Cmd{cmd[0], "-F", id + "#{session_name}:#{window_index}.#{pane_index}"}, cmd[1:]...)
	default:
		cmd = append(Cmd{cmd[0], "-P", "-F", id + silentMark}, cmd[1:]...)
	}
	return append([]Cmd{cmd}, cmds[1:]...)
}

// silentMark tells takeNewPane that the caller did not ask for -P output.
const silentMark = "\t\timux-silent\t"

// takeNewPane removes the new pane's id from a creating command's output.
func takeNewPane(o string) (string, string) {
	id, rest, ok := strings.Cut(o, newPaneSep)
	if !ok {
		return "", o
	}
	id = strings.TrimSpace(id)
	if strings.HasPrefix(rest, silentMark) {
		rest = strings.TrimPrefix(rest, silentMark)
		rest = strings.TrimPrefix(strings.TrimPrefix(rest, "\r"), "\n")
	}
	return id, rest
}

func indexOf(cmd Cmd, a string) int {
	for i, x := range cmd {
		if x == a {
			return i
		}
	}
	return -1
}

// withTarget gives the commands without a target the new pane.
func withTarget(cmds []Cmd, pane string) []Cmd {
	out := make([]Cmd, len(cmds))
	for i, cmd := range cmds {
		if indexOf(cmd, "-t") > 0 || isBroadcast(cmd) {
			out[i] = cmd
			continue
		}
		out[i] = append(Cmd{cmd[0], "-t", pane}, cmd[1:]...)
	}
	return out
}

// replayGlobals sets the remembered global options in a new session.
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
func psmuxArgs(cmd Cmd) Cmd {
	switch cmd[0] {
	case "display-menu", "menu":
		return psmuxMenu(cmd)
	case "display-popup", "popup":
		return psmuxPopup(cmd)
	}
	return cmd
}

func psmuxMenu(cmd Cmd) Cmd {
	out := Cmd{cmd[0]}
	i := 1
	for ; i < len(cmd); i++ {
		a := cmd[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		if len(a) == 2 && valueFlag(cmd[0], a[1]) && i+1 < len(cmd) {
			v := cmd[i+1]
			i++
			switch a[1] {
			case 'T':
				out = append(out, "-T", strings.ReplaceAll(v, "##", "#"))
			case 'x', 'y':
				if _, err := strconv.Atoi(v); err == nil {
					out = append(out, a, v)
				}
			}
		}
	}
	for i < len(cmd) {
		name := cmd[i]
		if name == "" {
			out = append(out, "")
			i++
			continue
		}
		key, c := "", ""
		if i+1 < len(cmd) {
			key = cmd[i+1]
		}
		if i+2 < len(cmd) {
			c = cmd[i+2]
		}
		disabled := strings.HasPrefix(name, "-")
		name = strings.ReplaceAll(strings.TrimPrefix(name, "-"), "##", "#")
		if disabled {
			key, c = " ", ""
		}
		out = append(out, name, key, c)
		i += 3
	}
	return out
}

func psmuxPopup(cmd Cmd) Cmd {
	out := Cmd{cmd[0]}
	var words []string
	for i := 1; i < len(cmd); i++ {
		a := cmd[i]
		if a == "--" {
			words = append(words, cmd[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			words = append(words, a)
			continue
		}
		if len(a) == 2 && valueFlag(cmd[0], a[1]) && i+1 < len(cmd) {
			v := cmd[i+1]
			i++
			switch a[1] {
			case 'w', 'h', 'd':
				out = append(out, a, v)
			}
			continue
		}
		if a == "-E" || a == "-EE" || a == "-K" {
			out = append(out, "-E")
		}
	}
	return append(out, words...)
}

// spawnPsmux is Spawn for psmux: the last group is started and not waited
// for (a display-menu client blocks until the menu closes).
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
