package tmux

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// ClickMinVersion is the oldest tmux with `#[range=user|X]` style ranges and
// the mouse_status_range format (absent from the 3.2a man page; Q26). Below
// it the agent bar is an indicator only and switching stays in the menus.
var ClickMinVersion = [2]int{3, 4}

// MinVersion is the oldest supported tmux (popups, extended keys).
var MinVersion = [2]int{3, 2}

// ConfInput is everything the generated tmux.conf depends on.
type ConfInput struct {
	Self      string         // absolute path of the imux binary
	Cfg       *config.Config // effective configuration
	Version   Version        // tmux version of the server
	Terminal  string         // default-terminal; "" = tmux-256color
	AgentBar  string         // status-format[1]; "" = no agent bar
	LocalConf string         // the user's tmux.local.conf, sourced last
}

// Clickable reports whether the status line (agent bar, tab bar) gets
// click handling.
func (in *ConfInput) Clickable() bool {
	return (in.AgentBar != "" || in.Cfg.Tmux.Tabs) && in.Cfg.Tmux.Mouse && in.Version.AtLeast(ClickMinVersion[0], ClickMinVersion[1])
}

// oldTabClicks reports whether the tab bar's × and + take clicks through
// tmux's status-left and status-right ranges: tmux 3.2 and 3.3 have no user
// ranges, but these two areas make mouse keys of their own
// (MouseUp1StatusLeft, MouseUp1StatusRight), which nothing else in imux's
// status lines uses.
func (in *ConfInput) oldTabClicks() bool {
	return in.Cfg.Tmux.Tabs && in.Cfg.Tmux.Mouse && !in.Version.AtLeast(ClickMinVersion[0], ClickMinVersion[1])
}

// TabPad is the tab bar's start: a blank as wide as the window's sidebar and
// its border, so that the tabs begin above the agent's pane (D52); nothing
// when a pane is zoomed or the window has no sidebar. tmux expands a
// modifier's argument given between "/"s (3.2 too), and redraws the bar
// when a pane is resized or closed.
const TabPad = "#{?window_zoomed_flag,,#{P:#{?#{==:#{@imux_role},sidebar},#{p/#{e|+:#{pane_width},1}/:},}}}"

// OptMoved names the client a sidebar click just showed another project or
// tab (D53): while it is set, MouseDown1Pane drops that client's presses on
// a sidebar. tmux 3.7 and later report a double click's second press as a
// new press once the pane under it changed, so the press that switched
// would act again in the new sidebar.
const OptMoved = "@imux_moved"

// MovedFor is how long OptMoved stays (run-shell -d): a double click's
// second press, and a triple click's third.
const MovedFor = "0.5"

// MovedCmds mark client as moved by a click (OptMoved) and have tmux clear
// the mark after MovedFor; they run with the commands that switch it.
func MovedCmds(client string) []Cmd {
	return []Cmd{
		{"set-option", "-g", OptMoved, client},
		{"run-shell", "-b", "-C", "-d", MovedFor, "set-option -gu " + OptMoved},
	}
}

// TabFormat is the tab bar (status-format[0] with tmux.tabs): a blank as
// wide as the sidebar (TabPad), the session's windows as tabs " <n> <name>
// <state> ", the current one highlighted, each a window range (a click
// selects it), then a "+" (range=user|imux:tab), and on the right ⏻ Exit
// (range=user|imux:quit, D48). With old (tmux before 3.4, no user ranges)
// the × is a status-left range, the Exit a status-right one, and the +
// stays outside any range (MouseUp1StatusDefault); the blank is then the
// current window's range, so a click on it does nothing.
func TabFormat(old bool) string {
	closeRange, tabRange, quitRange := "#[range=user|imux:close]", "#[range=user|imux:tab]", "#[range=user|imux:quit]"
	pad := TabPad
	if old {
		closeRange, tabRange, quitRange = "#[range=left]", "", "#[range=right]"
		pad = "#[range=window|#{window_index}]" + TabPad + "#[norange]"
	}
	// The state glyph always takes its column (a space when idle), so the
	// × and the next tabs do not move when an agent starts or stops.
	tab := " #I #{?@imux_tab,#{@imux_tab},#{?@imux_tab_auto,#{@imux_tab_auto},#W}} #{?@imux_tab_state,#{@imux_tab_state}, }"
	// Every range is followed by a cell outside any range: tmux 3.4 makes a
	// range one cell longer than drawn, so a range right after another loses
	// its first cell (the current tab took the click on its ×). The × takes
	// 3 columns, like the +.
	return "#[align=left]" + pad + " " +
		"#{W:" +
		"#[range=window|#{window_index}]#[fg=colour245]" + tab + " #[norange default] ," +
		"#[range=window|#{window_index}]#[bg=colour236 fg=colour255 bold]" + tab + "#[norange] " +
		closeRange + "#[bg=colour236 fg=colour245 nobold] × #[norange default] " +
		"}" +
		tabRange + "#[fg=colour245] + #[norange default]" +
		"#[align=right]" + quitRange + "#[fg=colour245] " + i18n.T("tabs.exit") + " #[norange default] "
}

// Silent is appended to every shell command imux's bindings, hooks and menu
// items run: tmux shows any output of run-shell (or a non-zero exit) in the
// pane's view mode, so imux reports results with display-message instead.
const Silent = " >/dev/null 2>&1 || true"

// UICommand returns the sh command line that runs imux with args silently.
// args are sh words; formats (#{pane_id}) are expanded by run-shell before
// the shell sees them, and the values imux puts there (pane ids, client
// names, agent-bar ranges, exit statuses, agent:id) need no sh quoting.
func UICommand(self string, args ...string) string {
	return ShQuote(self) + " " + strings.Join(args, " ") + Silent
}

// RunShell returns the tmux command `run-shell -b '<imux> args...'`.
func RunShell(self string, args ...string) string {
	return "run-shell -b " + Quote(UICommand(self, args...))
}

func runShell(self string, args ...string) string { return RunShell(self, args...) }

// SwapKeys are prefix H/J/K/L: swap the active pane with the one on that
// side (`imux __pane swap <dir>`, D67).
var SwapKeys = []struct{ Key, Dir string }{
	{"H", "left"}, {"J", "down"}, {"K", "up"}, {"L", "right"},
}

// DirectKeys are the arrows of the keys without the prefix
// (tmux.direct_pane_keys): Ctrl moves to the pane there, Ctrl+Alt splits
// that way after asking which agent (D67).
var DirectKeys = []struct{ Arrow, Flag, Split string }{
	{"Left", "-L", "h"}, {"Down", "-D", "v"}, {"Up", "-U", "v"}, {"Right", "-R", "h"},
}

// ResizeTable is the key table of the resize mode (prefix r, D67).
const ResizeTable = "imux-resize"

// resizeKeys resize the active pane in the resize mode: h j k l and the
// arrows by 2 cells, Shift by 10.
var resizeKeys = []struct {
	key, flag string
	n         int
}{
	{"h", "-L", 2},
	{"j", "-D", 2},
	{"k", "-U", 2},
	{"l", "-R", 2},
	{"Left", "-L", 2},
	{"Down", "-D", 2},
	{"Up", "-U", 2},
	{"Right", "-R", 2},
	{"H", "-L", 10},
	{"J", "-D", 10},
	{"K", "-U", 10},
	{"L", "-R", 10},
	{"S-Left", "-L", 10},
	{"S-Down", "-D", 10},
	{"S-Up", "-U", 10},
	{"S-Right", "-R", 10},
}

// resizeDone leave the resize mode (any other key leaves it too, and does
// what it does).
var resizeDone = []string{"Escape", "Enter", "q"}

// ResizeMode is the tmux command that enters the resize mode, saying so on
// the status line until the next key.
func ResizeMode() string {
	return "switch-client -T " + ResizeTable + " ; display-message -d 0 " + Quote(EscapeMessage(i18n.T("resize.mode")))
}

// BoundKeys lists the "table key" pairs the config binds, so a reload can
// unbind keys that a changed config no longer uses.
func BoundKeys(in *ConfInput) []string {
	t := in.Cfg.Tmux
	var ks []string
	if t.MenuKeyRoot != "" {
		ks = append(ks, "root "+t.MenuKeyRoot)
	}
	if t.QuickOpenKeyRoot != "" {
		ks = append(ks, "root "+t.QuickOpenKeyRoot)
	}
	if t.MenuKeyPrefix != "" {
		ks = append(ks, "prefix "+t.MenuKeyPrefix)
	}
	if t.PickerKeyPrefix != "" {
		ks = append(ks, "prefix "+t.PickerKeyPrefix)
	}
	keys := []string{t.SidebarKey, t.SplitRightKey, t.SplitDownKey, t.ResizeKey, t.HelpKey}
	if t.SwapPaneKeys {
		for _, sw := range SwapKeys {
			keys = append(keys, sw.Key)
		}
	}
	if t.ResizeKey != "" {
		for _, r := range resizeKeys {
			ks = append(ks, ResizeTable+" "+r.key)
		}
		for _, k := range resizeDone {
			ks = append(ks, ResizeTable+" "+k)
		}
	}
	if t.DirectPaneKeys {
		for _, d := range DirectKeys {
			ks = append(ks, "root C-"+d.Arrow, "root C-M-"+d.Arrow)
		}
	}
	if t.Tabs {
		keys = append(keys, t.NewTabKey, t.RenameTabKey)
	}
	for _, k := range keys {
		if k != "" {
			ks = append(ks, "prefix "+k)
		}
	}
	if in.Clickable() {
		ks = append(ks, "root MouseDown1Status", "root MouseUp1Status")
	}
	if in.oldTabClicks() {
		ks = append(ks, "root MouseUp1StatusLeft", "root MouseUp1StatusRight", "root MouseUp1StatusDefault")
	}
	if t.Mouse {
		ks = append(ks, "root MouseDown3Border", "root MouseDown1Pane", "root SecondClick1Pane", "root DoubleClick1Pane", "root TripleClick1Pane")
	}
	if in.Cfg.Paste.Images {
		ks = append(ks, "root C-v", "root M-v")
	}
	return ks
}

// GenerateConf renders ~/.imux/tmux.conf (§9.2).
func GenerateConf(in *ConfInput) string {
	cfg := in.Cfg
	term := in.Terminal
	if term == "" {
		term = "tmux-256color"
	}
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	pane := "--pane #{pane_id} --client #{client_name}"
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	w("# Generated by imux. Do not edit; put overrides in %s", in.LocalConf)
	w("# tmux %s, agent bar: %s, clickable: %s", in.Version, onOff(in.AgentBar != ""), onOff(in.Clickable()))
	w("")
	w("set -g  default-terminal %s", Quote(term))
	w("%%if \"#{!=:#{@imux_loaded},1}\"")
	w("set -as terminal-features ',*:RGB'")
	w("set -as terminal-features ',*:extkeys'")
	w("%%endif")
	w("set -g  @imux_loaded 1")
	w("# Updates of a pane in the background (a sidebar's spinner) reach the terminal as one")
	w("# synchronized update (DECSET 2026), with the cursor's way back to the active pane: it never")
	w("# shows in the sidebar mid-update. tmux 3.2's own Sync is an older form; terminals without")
	w("# 2026 ignore it. Applies to clients that attach after it is set.")
	w("%%if \"#{!=:#{@imux_sync},1}\"")
	w("set -as terminal-overrides ',*:Sync=\\E[?2026%%?%%p1%%{1}%%-%%tl%%eh%%;'")
	w("%%endif")
	w("set -g  @imux_sync 1")
	w("set -s  extended-keys on")
	w("set -s  escape-time 10")
	w("set -g  history-limit %d", cfg.Tmux.HistoryLimit)
	w("set -g  mouse %s", onOff(cfg.Tmux.Mouse))
	w("set -g  focus-events on")
	w("set -g  pane-border-status top")
	// Windows are named after the active pane's agent ("claude"), not its process ("node").
	w("set -g  automatic-rename-format %s", Quote("#{?@imux_agent,#{@imux_agent},#{pane_current_command}}"))
	// A named pane shows its name first: " backend fix · Claude ".
	w("set -g  pane-border-format %s", Quote(" #{?@imux_name,#{@imux_name} · ,}#{?@imux_label,#{@imux_label},#{pane_current_command}} #{?@imux_flag,#{@imux_flag} ,}"))
	w("set -g  status-right %s", Quote("#{?@imux_status,#{@imux_status} · ,}%H:%M "))
	w("set -g  status-right-length 80")
	w("set -g  status-left-length 32") // session names are <dir>-<hash>; the default 10 cuts them
	w("set -gq @imux_status ''")
	if cfg.Tmux.Tabs {
		w("# Tab bar at the top: the session's windows, and + for a new tab.")
		w("set -g  status-position top")
		w("set -g  status-style %s", Quote("bg=default,fg=colour245"))
		w("set -g  status-format[0] %s", Quote(TabFormat(in.oldTabClicks())))
	} else {
		w("set -g  status-position bottom")
		w("set -gu status-format[0]")
	}
	if in.AgentBar != "" {
		w("set -g  status 2")
		w("set -g  status-format[1] %s", Quote(in.AgentBar))
	} else {
		w("set -g  status on")
	}
	w("")
	w("# Pane menu, session picker.")
	if k := cfg.Tmux.MenuKeyRoot; k != "" {
		w("bind -n %s %s", Quote(k), runShell(in.Self, "__menu", pane))
	}
	if k := cfg.Tmux.MenuKeyPrefix; k != "" {
		w("bind    %s %s", Quote(k), runShell(in.Self, "__menu", pane))
	}
	if k := cfg.Tmux.QuickOpenKeyRoot; k != "" {
		w("# Go to File, the editor's Ctrl+P (§9.3).")
		w("bind -n %s %s", Quote(k), runShell(in.Self, "__goto", pane))
	}
	if k := cfg.Tmux.PickerKeyPrefix; k != "" {
		w("bind    %s %s", Quote(k), runShell(in.Self, "__menu", "--sessions", pane))
	}
	if cfg.Paste.Images {
		w("# Ctrl+V / Alt+V in an agent's pane paste the clipboard's image; elsewhere, or with no image, the key goes on.")
		for _, k := range []string{"C-v", "M-v"} {
			w("bind -n %s if-shell -F %s %s %s", k, Quote("#{@imux_agent}"),
				QuoteDouble(runShell(in.Self, "__paste-image", pane, "--key", k)), Quote("send-keys "+k))
		}
	}
	w("# Sidebar on/off, and splits that ask which agent to start.")
	if k := cfg.Tmux.SidebarKey; k != "" {
		w("bind    %s %s", Quote(k), runShell(in.Self, "sidebar", "toggle", pane))
	}
	if k := cfg.Tmux.SplitRightKey; k != "" {
		w("bind    %s %s", Quote(k), runShell(in.Self, "__menu", "--split", "h", pane))
	}
	if k := cfg.Tmux.SplitDownKey; k != "" {
		w("bind    %s %s", Quote(k), runShell(in.Self, "__menu", "--split", "v", pane))
	}
	w("# Panes from the keyboard (D67): swap with the pane on a side, the resize mode, the help.")
	if cfg.Tmux.SwapPaneKeys {
		for _, sw := range SwapKeys {
			w("bind    %s %s", Quote(sw.Key), runShell(in.Self, "__pane", "swap", sw.Dir, pane))
		}
	}
	if k := cfg.Tmux.ResizeKey; k != "" {
		w("bind    %s { %s }", Quote(k), ResizeMode())
		for _, r := range resizeKeys {
			w("bind -T %s %s { resize-pane %s %d ; %s }", ResizeTable, Quote(r.key), r.flag, r.n, ResizeMode())
		}
		for _, k := range resizeDone {
			w("bind -T %s %s { switch-client -T root ; display-message -d 1 '' }", ResizeTable, Quote(k))
		}
	}
	if k := cfg.Tmux.HelpKey; k != "" {
		w("bind    %s %s", Quote(k), runShell(in.Self, "__menu", "--keys", pane))
	}
	if cfg.Tmux.DirectPaneKeys {
		w("# Without the prefix: Ctrl+arrows move to the pane there, Ctrl+Alt+arrows split that way.")
		for _, d := range DirectKeys {
			w("bind -n C-%s select-pane %s", d.Arrow, d.Flag)
			args := []string{"__menu", "--split", d.Split}
			if d.Arrow == "Left" || d.Arrow == "Up" {
				args = append(args, "--before")
			}
			w("bind -n C-M-%s %s", d.Arrow, runShell(in.Self, append(args, pane)...))
		}
	}
	if cfg.Tmux.Tabs {
		w("# New tab (asks which agent) and the tab's name.")
		if k := cfg.Tmux.NewTabKey; k != "" {
			w("bind    %s %s", Quote(k), runShell(in.Self, "__menu", "--tab", pane))
		}
		if k := cfg.Tmux.RenameTabKey; k != "" {
			w("bind    %s command-prompt -I %s -p %s %s", Quote(k), Quote("#{@imux_tab}"), Quote(i18n.T("prompt.tab")),
				Quote("set-option -w @imux_tab \"%%%\""))
		}
	}
	if in.Clickable() {
		w("# Clicks on imux regions (range=user|imux:<x>) go to imux when the button is released: a menu")
		w("# opened while it is still down would be closed by the release. Other status clicks keep tmux's default.")
		w("bind -n MouseDown1Status if-shell -F %s %s",
			Quote("#{?#{m:imux:*,#{mouse_status_range}},0,1}"),
			QuoteDouble("switch-client -t ="))
		w("bind -n MouseUp1Status if-shell -F %s %s",
			Quote("#{m:imux:*,#{mouse_status_range}}"),
			QuoteDouble(runShell(in.Self, "__click", "#{mouse_status_range}", pane)))
	}
	if in.oldTabClicks() {
		w("# tmux before 3.4: the tab bar's × is its status-left range, ⏻ Exit its status-right range and the +")
		w("# the bar outside any range; clicks go to imux when the button is released.")
		w("bind -n MouseUp1StatusLeft %s", runShell(in.Self, "__click", "imux:close", pane))
		w("bind -n MouseUp1StatusRight %s", runShell(in.Self, "__click", "imux:quit", pane))
		w("bind -n MouseUp1StatusDefault %s", runShell(in.Self, "__click", "imux:tab", pane))
	}
	if cfg.Tmux.Mouse {
		w("# Right-click on a pane border opens that pane's menu (-t = is the pane under the mouse).")
		w("bind -n MouseDown3Border run-shell -t = -b %s", Quote(UICommand(in.Self, "__menu", pane)))
		w("# A sidebar acts on a click at once, so a double or triple click on it must not select it")
		w("# (tmux's own bindings do, 300 ms after a double click: the focus went back to the sidebar),")
		w("# and the second and third press reach a sidebar only if the first did: a click that showed")
		w("# another tab or project must not act again in that one's sidebar. Other panes keep tmux's")
		w("# defaults (tmux's MouseDown1Pane, which also notes the pane, @imux_press). tmux 3.7+ reports")
		w("# a second press in another pane as a new one: while a click that showed this client another")
		w("# tab or project is fresh (%s, D53), its presses on a sidebar are dropped.", OptMoved)
		const sidebar = "#{==:#{@imux_role},sidebar}"
		const mouse = "#{||:#{pane_in_mode},#{mouse_any_flag}}"
		const same = "#{==:#{pane_id},#{@imux_press}}"
		const moved = "#{&&:" + sidebar + ",#{==:#{" + OptMoved + "},#{client_name}}}"
		w("bind -n MouseDown1Pane if -F -t= %s {} { set -gF @imux_press '#{mouse_pane}'; select-pane -t=; send -M }", Quote(moved))
		w("bind -n SecondClick1Pane if -F -t= %s { send -M } {}", Quote("#{||:#{!=:#{@imux_role},sidebar},"+same+"}"))
		for _, c := range []struct{ key, sel, own string }{
			{"DoubleClick1Pane", "select-word", "send -M"},
			{"TripleClick1Pane", "select-line", "if -F -t= " + Quote(same) + " { send -M } {}"},
		} {
			w("bind -n %s if -F -t= %s { %s } { select-pane -t=; if -F %s { send -M } { copy-mode -H; send -X %s; run -d0.3; send -X copy-pipe-and-cancel } }",
				c.key, Quote(sidebar), c.own, Quote(mouse), c.sel)
		}
	}
	w("set -g  @imux_keys %s", Quote(strings.Join(BoundKeys(in), ",")))
	w("")
	w("# Pane lifecycle and watcher.")
	w("set-hook -g pane-died %s", QuoteDouble(runShell(in.Self, "__pane-died", "#{pane_id}", "#{pane_dead_status}")))
	w("set-hook -g client-attached %s", QuoteDouble(runShell(in.Self, "__watch", "ensure")))
	if cfg.Tmux.Sidebar {
		w("# Sidebars redraw when panes change (they sleep otherwise).")
		for _, h := range []string{"pane-focus-in", "after-split-window", "after-kill-pane", "pane-exited", "window-pane-changed"} {
			w("set-hook -g %s %s", h, QuoteDouble(runShell(in.Self, "__sidebar-refresh")))
		}
	}
	w("")
	w("source-file -q %s", Quote(in.LocalConf))
	return b.String()
}

// WriteConf writes content to path if it differs from what is there (by
// content hash) and reports whether it changed.
func WriteConf(path, content string, write func(string, []byte) error) (bool, error) {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil && sha256.Sum256(old) == sha256.Sum256([]byte(content)) {
		return false, nil
	}
	if write == nil {
		write = func(p string, b []byte) error { return os.WriteFile(p, b, 0o600) }
	}
	if err := write(path, []byte(content)); err != nil {
		return false, err
	}
	return true, nil
}

// ParseBoundKeys reads @imux_keys (BoundKeys joined by ","): its table and
// key pairs. A key that is a comma ("prefix ," names a tab) is read as one,
// not as an empty key and an empty entry.
func ParseBoundKeys(v string) [][2]string {
	parts := strings.Split(strings.TrimSpace(v), ",")
	var out [][2]string
	for i := 0; i < len(parts); i++ {
		tk := parts[i]
		if strings.HasSuffix(tk, " ") && i+1 < len(parts) && parts[i+1] == "" {
			tk, i = tk+",", i+1
		}
		table, key, ok := strings.Cut(strings.TrimLeft(tk, " "), " ")
		if !ok || key == "" {
			continue
		}
		out = append(out, [2]string{table, key})
	}
	return out
}

// Reload makes a running server pick up path: keys bound by the previous
// config are unbound first (they may have changed), then the file is
// sourced. A server that is not running is not an error.
func (c *Client) Reload(ctx context.Context, path string) error {
	out, err := c.Run(ctx, Cmd{"show-options", "-gqv", "@imux_keys"})
	if errors.Is(err, ErrNoServer) {
		return nil
	}
	if err != nil {
		return err
	}
	var cmds []Cmd
	for _, tk := range ParseBoundKeys(out) {
		cmds = append(cmds, Cmd{"unbind-key", "-q", "-T", tk[0], tk[1]})
	}
	cmds = append(cmds, Cmd{"source-file", path})
	_, err = c.Run(ctx, cmds...)
	if errors.Is(err, ErrNoServer) {
		return nil
	}
	return err
}

// TerminfoExists reports whether a terminfo entry is installed, looking in
// the usual directories (letter and hex subdirectory layouts).
func TerminfoExists(name string) bool {
	if name == "" {
		return false
	}
	var dirs []string
	if d := os.Getenv("TERMINFO"); d != "" {
		dirs = append(dirs, d)
	}
	if h, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(h, ".terminfo"))
	}
	for _, d := range strings.Split(os.Getenv("TERMINFO_DIRS"), ":") {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	dirs = append(dirs, "/etc/terminfo", "/lib/terminfo", "/usr/share/terminfo", "/usr/lib/terminfo",
		"/opt/homebrew/share/terminfo", "/usr/local/share/terminfo")
	for _, d := range dirs {
		for _, sub := range []string{name[:1], fmt.Sprintf("%x", name[0])} {
			if fi, err := os.Stat(filepath.Join(d, sub, name)); err == nil && !fi.IsDir() {
				return true
			}
		}
	}
	return false
}

// DefaultTerminal picks tmux-256color when its terminfo exists, else
// screen-256color (doctor explains how to install the former).
func DefaultTerminal() string {
	if TerminfoExists("tmux-256color") {
		return "tmux-256color"
	}
	return "screen-256color"
}
