// Package autoui builds the Scheduler's menus and Quick Picks (§9.3
// "Scheduler", D61): when a definition runs, with which agent CLI and
// model, and an item's menu.
//
// As Source Control's (D58), a menu's commands carry no names or paths:
// imux with fixed words (a preset time, a CLI's id), a definition's
// 12-hex-digit id and the client's ids; the project is in OptCwd, a typed
// time in OptText. A Quick Pick's items are argv, which no shell or tmux
// parser reads.
package autoui

import (
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/quickpick"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// Global options the menus and prompts pass values through.
const (
	OptCwd  = "@imux_auto_cwd"  // the project root a menu is about
	OptText = "@imux_auto_text" // a prompt's answer: a time typed
)

// Flows: what the menus ask, one after the other, and what is done then.
const (
	FlowNew  = "new"  // ⏰: when, the CLI, the model; then it is scheduled
	FlowRun  = "run"  // 🚀 the first time: the CLI, the model; then it runs
	FlowTime = "time" // Change time…
	FlowCLI  = "cli"  // Change CLI and model…
)

// ValidFlow reports a known flow.
func ValidFlow(f string) bool {
	return f == FlowNew || f == FlowRun || f == FlowTime || f == FlowCLI
}

// Preset is a time the When menu offers.
type Preset struct {
	When  string // the stored form
	Label string // its i18n key
	Key   string
}

// Presets are the When menu's times.
var Presets = []Preset{
	{"5m", "auto.when_5m", "5"},
	{"15m", "auto.when_15m", "1"},
	{"hourly", "auto.when_hourly", "h"},
	{"09:00", "auto.when_daily", "d"},
}

// Word is a time as one word of a menu command ("09:00,18:30").
func Word(when string) string { return strings.ReplaceAll(strings.TrimSpace(when), " ", ",") }

// Ctx is where the menus act.
type Ctx struct {
	Self    string // the imux binary
	Dir     string // the project root
	Client  string
	Pane    string // the pane the sidebar follows
	Sidebar string // the sidebar's pane
	X, Y    int    // where the menus open in the sidebar (1-based); 0: centered
}

// Flags are the context's flags for imux commands.
func (c Ctx) Flags() []string {
	var a []string
	if c.Client != "" {
		a = append(a, "--client", c.Client)
	}
	if c.Pane != "" {
		a = append(a, "--pane", c.Pane)
	}
	if c.Sidebar != "" {
		a = append(a, "--sidebar", c.Sidebar)
	}
	return a
}

// Cmd is a menu item's command: `imux __auto args` with the context, which
// opens its menu where this one was.
func (c Ctx) Cmd(args ...string) string {
	a := append(append([]string{"__auto"}, args...), c.Flags()...)
	if c.X > 0 && c.Y > 0 && c.Sidebar != "" {
		a = append(a, "--x", strconv.Itoa(c.X), "--y", strconv.Itoa(c.Y))
	}
	return tmux.RunShell(c.Self, a...)
}

// Argv is a Quick Pick item's argv: `__auto` words, the project and the
// context, then data after "--".
func (c Ctx) Argv(words []string, data ...string) []string {
	a := append(append(append([]string{"__auto"}, words...), "--cwd", c.Dir), c.Flags()...)
	if len(data) > 0 {
		a = append(append(a, "--"), data...)
	}
	return a
}

type item = workspace.MenuItem

func it(key, label, cmd string) item { return item{Name: i18n.T(label), Key: key, Cmd: cmd} }

func sep() item { return workspace.Separator() }

// Item is what an item's menu is about.
type Item struct {
	ID        string
	Name      string
	Scheduled bool // it has a schedule (a time, or a CLI it ran with)
	Manual    bool // it runs only by hand
	Paused    bool
	ReadOnly  bool
	Running   bool
	Runs      int
	Session   bool // its last run left a conversation to continue (Claude)
	Gone      bool // its definition is not there any more
}

// ItemMenu is an item's menu (⋯ or a right-click): Run now or Stop, its
// output and history, Continue in a pane, its time, CLI and model, access,
// Pause or Resume, Open definition, Remove schedule.
func ItemMenu(c Ctx, x Item) *workspace.Menu {
	id := x.ID
	var items []item
	switch {
	case x.Running:
		items = append(items, it("s", "auto.menu_stop", c.Cmd("stop", id)))
	case !x.Gone:
		items = append(items, it("r", "auto.menu_run", c.Cmd("run", id)))
	}
	if x.Runs > 0 || x.Running {
		items = append(items, it("o", "auto.menu_output", c.Cmd("log", id)))
	}
	if x.Runs > 1 {
		items = append(items, it("h", "auto.menu_history", c.Cmd("pick", "history", id)))
	}
	if x.Session && !x.Running {
		items = append(items, it("c", "auto.menu_continue", c.Cmd("continue", id)))
	}
	items = append(items, sep())
	if !x.Scheduled {
		items = append(items, it("t", "auto.menu_schedule", c.Cmd("menu", "when", id, "--flow", FlowNew)))
	} else if !x.Gone {
		items = append(items,
			it("t", "auto.menu_time", c.Cmd("menu", "when", id, "--flow", FlowTime)),
			it("m", "auto.menu_cli", c.Cmd("menu", "cli", id, "--flow", FlowCLI)),
			item{Name: i18n.T("auto.menu_access") + " ›", Key: "a", Cmd: c.Cmd("menu", "access", id)})
		switch {
		case x.Manual:
		case x.Paused:
			items = append(items, it("p", "auto.menu_resume", c.Cmd("resume", id)))
		default:
			items = append(items, it("p", "auto.menu_pause", c.Cmd("pause", id)))
		}
	}
	items = append(items, sep())
	if !x.Gone {
		items = append(items, it("e", "auto.menu_open", c.Cmd("open", id)))
	}
	if x.Scheduled {
		items = append(items, it("x", "auto.menu_remove", c.Cmd("confirm", "remove", id)))
	}
	return &workspace.Menu{Title: x.Name, Items: items}
}

// WhenMenu asks when name runs: the presets, Custom… (a prompt), and for
// Change time… "Only by hand"; cur (stored form) is marked.
func WhenMenu(c Ctx, id, name, flow, cur string) *workspace.Menu {
	mark := func(label, when string) string {
		if flow == FlowTime && when == cur {
			return "● " + label
		}
		return label
	}
	var items []item
	for _, p := range Presets {
		items = append(items, item{Name: mark(i18n.T(p.Label), p.When), Key: p.Key, Cmd: c.Cmd("when", id, p.When, "--flow", flow)})
	}
	items = append(items, it("c", "auto.when_custom", c.Cmd("ask", "when", id, "--flow", flow)))
	if flow == FlowTime {
		items = append(items, sep(), item{Name: mark(i18n.T("auto.when_manual"), ""), Key: "m", Cmd: c.Cmd("when", id, "manual", "--flow", flow)})
	}
	return &workspace.Menu{Title: i18n.T("auto.when_title", name), Items: items}
}

// CLI is an agent CLI the CLI menu offers.
type CLI struct {
	ID        session.AgentID
	Name      string // "Claude"
	Key       string
	Installed bool
	Limited   string // "15:00" while its usage limit lasts
	Own       bool   // the definition is written for it
	Current   bool   // the schedule's CLI now
}

// CLIMenu asks which agent CLI runs name: every CLI i-mux knows, the
// definition's own first and marked; the ones not installed disabled.
func CLIMenu(c Ctx, id, name, flow, when string, clis []CLI) *workspace.Menu {
	var items []item
	for _, a := range clis {
		label := a.Name
		if a.Current {
			label = "● " + label
		}
		if a.Own {
			label += " · " + i18n.T("auto.cli_own")
		}
		switch {
		case !a.Installed:
			label += " " + i18n.T("menu.not_installed")
		case a.Limited != "":
			label += " · " + i18n.T("sidebar.limited", a.Limited)
		}
		args := []string{"cli", id, string(a.ID), "--flow", flow}
		if when != "" {
			args = append(args, "--when", Word(when))
		}
		items = append(items, item{Name: label, Key: a.Key, Cmd: c.Cmd(args...), Disabled: !a.Installed})
	}
	return &workspace.Menu{Title: i18n.T("auto.cli_title", name), Items: items}
}

// AccessMenu chooses what a run may do: anything (it runs alone) or only
// read.
func AccessMenu(c Ctx, id, name string, readOnly bool) *workspace.Menu {
	full, read := "  ", "  "
	if readOnly {
		read = "● "
	} else {
		full = "● "
	}
	return &workspace.Menu{Title: name, Items: []item{
		{Name: full + i18n.T("auto.access_full"), Key: "f", Cmd: c.Cmd("access", id, auto.AccessFull)},
		{Name: read + i18n.T("auto.access_read"), Key: "r", Cmd: c.Cmd("access", id, auto.AccessRead)},
		sep(),
		{Name: "‹ " + i18n.T("scm.back"), Key: "BSpace", Cmd: c.Cmd("menu", "item", id)},
	}}
}

// ConfirmRemove asks before a schedule and its runs go.
func ConfirmRemove(c Ctx, id, name string) *workspace.Menu {
	return &workspace.Menu{Title: i18n.T("auto.remove_title", name), Items: []item{
		{Name: i18n.T("auto.remove_text"), Disabled: true},
		sep(),
		it("y", "auto.remove_yes", c.Cmd("remove", id)),
		it("n", "click.cancel", ""),
	}}
}

// RootMenu is the view's menu (its empty space and title): ✨ New agent…
// and a blank agent file. They run `imux __skill dialog --agents`, which
// reads the project from the skill options.
func RootMenu(c Ctx) *workspace.Menu {
	args := func(extra ...string) []string {
		a := append([]string{"__skill", "dialog", "--agents"}, extra...)
		if c.Client != "" {
			a = append(a, "--client", c.Client)
		}
		return a
	}
	return &workspace.Menu{Title: i18n.T("rail.schedule"), Items: []item{
		it("n", "auto.menu_new", tmux.RunShell(c.Self, args()...)),
		it("t", "auto.menu_template", tmux.RunShell(c.Self, args("--template")...)),
	}}
}

// ModelPick asks which model cli runs name with: the default (defNote says
// whose), the ones used before, the CLI's own list, and the name typed.
func ModelPick(c Ctx, id, name, flow, when string, cli CLI, models []auto.Model, used []string, defNote string) *quickpick.Pick {
	words := []string{"model", id, "--flow", flow, "--cli", string(cli.ID)}
	if when != "" {
		words = append(words, "--when", Word(when))
	}
	items := []quickpick.Item{{Label: i18n.T("auto.model_default"), Description: defNote, Argv: c.Argv(words), Always: true}}
	seen := map[string]bool{}
	for _, u := range used {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		items = append(items, quickpick.Item{Label: u, Group: i18n.T("auto.model_used"), Argv: c.Argv(words, u)})
	}
	for _, m := range models {
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		items = append(items, quickpick.Item{Label: m.ID, Description: m.Name, Group: i18n.T("auto.model_group", cli.Name), Argv: c.Argv(words, m.ID)})
	}
	items = append(items, quickpick.Item{Label: i18n.T("auto.model_typed"), Group: " ", Always: true, Query: true, Argv: c.Argv(words)})
	return &quickpick.Pick{
		Title: i18n.T("auto.model_title", name, cli.Name), Placeholder: i18n.T("auto.model_filter"),
		Empty: i18n.T("auto.model_none"), Items: items, Client: c.Client, Pane: c.Pane,
	}
}

// HistoryPick lists name's runs, newest first; one shows its output.
func HistoryPick(c Ctx, id, name string, runs []auto.Run, now time.Time) *quickpick.Pick {
	var items []quickpick.Item
	for _, r := range runs {
		label := r.Status.Mark() + " " + textfmt.Clock(now, r.Start)
		if s := r.Text(); s != "" {
			label += "  " + s
		}
		desc := []string{auto.Took(r.Duration())}
		if r.CLI != "" {
			desc = append(desc, session.ShortName(session.AgentID(r.CLI)))
		}
		if r.Model != "" {
			desc = append(desc, r.Model)
		}
		if r.Manual {
			desc = append(desc, i18n.T("auto.by_hand"))
		}
		items = append(items, quickpick.Item{Label: label, Description: strings.Join(desc, " · "), Argv: c.Argv([]string{"log", id, "--log", r.Log})})
	}
	return &quickpick.Pick{
		Title: i18n.T("auto.history_title", name), Placeholder: i18n.T("scm.pick_filter"),
		Empty: i18n.T("auto.history_empty"), Items: items, Client: c.Client, Pane: c.Pane,
	}
}
