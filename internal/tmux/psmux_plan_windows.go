//go:build windows

package tmux

// Planning a batch for psmux: which server each command goes to, the forms
// of its flags and targets psmux reads, and the commands psmux has no form
// of its own for (new panes acted on by the commands after them).

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// --- planning a batch -----------------------------------------------------------

type group struct {
	route     string // session whose server runs the commands; "" = psmux's choice
	cmds      []Cmd
	broadcast bool // every live session
	gather    bool // every live session, outputs joined
	creates   bool // a pane-creating command that untargeted followers act on
	spans     bool // lists every session: never routed to one server
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
		if spansSessions(cmd) {
			// psmux merges every server's answer when the command is
			// not routed to one of them.
			gs = append(gs, group{cmds: []Cmd{cmd}, spans: true})
			cur = -1
			continue
		}
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

// spansSessions reports whether cmd lists every session (list-sessions, and
// list-panes / list-windows -a).
func spansSessions(cmd Cmd) bool {
	switch cmd[0] {
	case "list-sessions", "ls":
		return true
	case "list-panes", "lsp", "list-windows", "lsw":
		return hasFlag(cmd, 'a')
	}
	return false
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
		c = menuItemCommand(c)
		disabled := strings.HasPrefix(name, "-")
		name = strings.ReplaceAll(strings.TrimPrefix(name, "-"), "##", "#")
		if disabled {
			key, c = "", ""
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

// menuItemCommand makes a menu item's tmux commands that name imux ids
// (%3~session) run through imux (imux __tmux), which routes them: psmux runs
// a menu item itself and knows neither the ids nor the other sessions'
// servers. Shell commands (run-shell, popups) pass ids to imux already.
func menuItemCommand(c string) string {
	if !strings.Contains(c, IDSep) {
		return c
	}
	first, _, _ := strings.Cut(strings.TrimSpace(c), " ")
	switch first {
	case "run-shell", "run", "display-popup", "popup":
		return c
	}
	self, err := os.Executable()
	if err != nil {
		return c
	}
	return "run-shell -b " + Quote(UICommand(self, "__tmux", ShQuote(c)))
}
