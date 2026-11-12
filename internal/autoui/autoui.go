// Package autoui builds the Scheduler's menus and Quick Picks (§9.3
// "Scheduler", D61): an item's menu (⋯ or a right-click), the question
// before a schedule goes, and the list of its runs. What runs it and when
// is chosen in the Run panel (autodialog).
//
// As Source Control's (D58), a menu's commands carry no names or paths:
// imux with fixed words, a definition's 12-hex-digit id and the client's
// ids; the project is in OptCwd. A Quick Pick's items are argv, which no
// shell or tmux parser reads.
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

// OptCwd is the global option that holds the project root a menu is about.
const OptCwd = "@imux_auto_cwd"

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

// Cmd is a menu item's command: `imux __auto args` with the context, whose
// menu opens where this one was.
func (c Ctx) Cmd(args ...string) string {
	a := append(append([]string{"__auto"}, args...), c.Flags()...)
	if c.X > 0 && c.Y > 0 && c.Sidebar != "" {
		a = append(a, "--x", strconv.Itoa(c.X), "--y", strconv.Itoa(c.Y))
	}
	return tmux.RunShell(c.Self, a...)
}

// Argv is a Quick Pick item's argv: `__auto` words, the project and the
// context.
func (c Ctx) Argv(words ...string) []string {
	return append(append(append([]string{"__auto"}, words...), "--cwd", c.Dir), c.Flags()...)
}

type item = workspace.MenuItem

func it(key, label, cmd string) item { return item{Name: i18n.T(label), Key: key, Cmd: cmd} }

func sep() item { return workspace.Separator() }

// Item is what an item's menu is about.
type Item struct {
	ID        string
	Name      string
	Record    bool // it has a schedule entry (a time, or the CLI it ran with)
	Scheduled bool // it has a time (not only by hand)
	Paused    bool
	Running   bool
	Runs      int
	Continue  bool // its last run left a conversation to continue
	Gone      bool // its definition is not there any more
}

// ItemMenu is an item's menu (⋯ or a right-click): Run… (the Run panel)
// or Stop, its output and runs, Continue in a pane, its schedule, Pause or
// Resume, Open definition, and Remove.
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
	if x.Continue && !x.Running {
		items = append(items, it("c", "auto.menu_continue", c.Cmd("continue", id)))
	}
	items = append(items, sep())
	if !x.Gone {
		label := "auto.menu_schedule"
		if x.Scheduled {
			label = "auto.menu_time"
		}
		items = append(items, it("t", label, c.Cmd("run", id, "--at-when")))
	}
	if x.Scheduled && !x.Gone {
		if x.Paused {
			items = append(items, it("p", "auto.menu_resume", c.Cmd("resume", id)))
		} else {
			items = append(items, it("p", "auto.menu_pause", c.Cmd("pause", id)))
		}
	}
	items = append(items, sep())
	if !x.Gone {
		items = append(items, it("e", "auto.menu_open", c.Cmd("open", id)))
	}
	if x.Record {
		label := "auto.menu_forget"
		if x.Scheduled {
			label = "auto.menu_remove"
		}
		items = append(items, it("x", label, c.Cmd("confirm", "remove", id)))
	}
	return &workspace.Menu{Title: x.Name, Items: items}
}

// ConfirmRemove asks before a schedule (or what is kept of runs by hand)
// and its runs go.
func ConfirmRemove(c Ctx, id, name string, scheduled bool) *workspace.Menu {
	title := i18n.T("auto.forget_title", name)
	if scheduled {
		title = i18n.T("auto.remove_title", name)
	}
	return &workspace.Menu{Title: title, Items: []item{
		{Name: i18n.T("auto.remove_text"), Disabled: true},
		sep(),
		it("y", "auto.remove_yes", c.Cmd("remove", id)),
		it("n", "click.cancel", ""),
	}}
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
		words := []string{"log", id}
		if r.Log != "" {
			words = append(words, "--log", r.Log)
		}
		items = append(items, quickpick.Item{Label: label, Description: strings.Join(desc, " · "), Argv: c.Argv(words...)})
	}
	return &quickpick.Pick{
		Title: i18n.T("auto.history_title", name), Placeholder: i18n.T("scm.pick_filter"),
		Empty: i18n.T("auto.history_empty"), Items: items, Client: c.Client, Pane: c.Pane,
	}
}
