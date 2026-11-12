package autoui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// A project and a definition name a shell or tmux would read as commands.
const (
	evilDir  = "/tmp/a b;#(touch x)$(y)"
	evilName = "tag;$(rm -rf z)#(w)"
	id       = "0123456789ab"
)

// evilBits are pieces of the data above that no command may hold.
var evilBits = []string{"a b;", "touch x", "$(y)", "rm -rf", "#(w)", "tag;"}

func testCtx() Ctx {
	return Ctx{Self: "/opt/imux", Dir: evilDir, Client: "/dev/pts/3", Pane: "%5", Sidebar: "%6", X: 12, Y: 7}
}

func clis() []CLI {
	return []CLI{
		{ID: session.Claude, Name: "Claude", Key: "c", Installed: true, Own: true},
		{ID: session.Codex, Name: "Codex", Key: "x"},
		{ID: session.Copilot, Name: "Copilot", Key: "p", Installed: true, Limited: "15:00", Current: true},
	}
}

func allMenus(c Ctx) map[string]*workspace.Menu {
	return map[string]*workspace.Menu{
		"item":         ItemMenu(c, Item{ID: id, Name: evilName, Scheduled: true, Runs: 3, Session: true}),
		"item-new":     ItemMenu(c, Item{ID: id, Name: evilName}),
		"item-running": ItemMenu(c, Item{ID: id, Name: evilName, Scheduled: true, Running: true, Runs: 1, Paused: true}),
		"item-manual":  ItemMenu(c, Item{ID: id, Name: evilName, Scheduled: true, Manual: true, Runs: 1}),
		"item-gone":    ItemMenu(c, Item{ID: id, Name: evilName, Scheduled: true, Gone: true, Runs: 2}),
		"when":         WhenMenu(c, id, evilName, FlowNew, ""),
		"when-time":    WhenMenu(c, id, evilName, FlowTime, "hourly"),
		"cli":          CLIMenu(c, id, evilName, FlowNew, "09:00 18:30", clis()),
		"access":       AccessMenu(c, id, evilName, true),
		"remove":       ConfirmRemove(c, id, evilName),
		"root":         RootMenu(c),
	}
}

// shown is a menu as display-menu shows it: its items' names ("-" for a
// separator) and their commands.
func shown(m *workspace.Menu) (names []string, cmds map[string]string) {
	c := m.Cmd("", "")
	i := 0
	for c[i] != "--" {
		i++
	}
	cmds = map[string]string{}
	for i++; i < len(c); {
		if c[i] == "" {
			names = append(names, "-")
			i++
			continue
		}
		names = append(names, c[i])
		cmds[c[i]] = c[i+2]
		i += 3
	}
	return names, cmds
}

// TestMenusCarryNoData (T66.2): every item has a translated name and a key
// of its own; its command runs imux with fixed words, the definition's id
// and the client's ids, never the project or the name (D58).
func TestMenusCarryNoData(t *testing.T) {
	for _, loc := range []string{"en", "tr"} {
		i18n.SetLocale(loc)
		for name, m := range allMenus(testCtx()) {
			keys := map[string]bool{}
			for _, it := range m.Items {
				if it.Name == "" {
					continue
				}
				if strings.Contains(it.Name, "auto.") || strings.Contains(it.Name, "menu.") {
					t.Errorf("%s %s: untranslated %q", loc, name, it.Name)
				}
				if !it.Disabled && (it.Key == "" || keys[it.Key]) {
					t.Errorf("%s %s: key %q of %q missing or taken", loc, name, it.Key, it.Name)
				}
				keys[it.Key] = true
				if it.Cmd == "" && !it.Disabled && it.Key != "n" {
					t.Errorf("%s %s: %q does nothing", loc, name, it.Name)
				}
				for _, bit := range evilBits {
					if strings.Contains(it.Cmd, bit) {
						t.Errorf("%s %s: %q carries data: %s", loc, name, it.Name, it.Cmd)
					}
				}
				if it.Cmd != "" && !strings.Contains(it.Cmd, "/opt/imux") {
					t.Errorf("%s %s: %q runs no imux: %s", loc, name, it.Name, it.Cmd)
				}
			}
		}
	}
	i18n.SetLocale("en")
}

func TestItemMenus(t *testing.T) {
	i18n.SetLocale("en")
	for name, want := range map[string]string{
		"item":         "Run now|Last output|History…|Continue in a pane|-|Change time…|Change CLI and model…|Access ›|Pause|-|Open definition|Remove schedule…",
		"item-new":     "Run now|-|Schedule…|-|Open definition",
		"item-running": "Stop|Last output|-|Change time…|Change CLI and model…|Access ›|Resume|-|Open definition|Remove schedule…",
		"item-manual":  "Run now|Last output|-|Change time…|Change CLI and model…|Access ›|-|Open definition|Remove schedule…",
		"item-gone":    "Last output|History…|-|Remove schedule…",
		"when":         "Every 5 minutes|Every 15 minutes|Every hour on the hour|Every day at 09:00|Custom…",
		"when-time":    "Every 5 minutes|Every 15 minutes|● Every hour on the hour|Every day at 09:00|Custom…|-|Only by hand",
		"cli":          "Claude · its own|-Codex (not installed)|● Copilot · limit 15:00",
		"access":       "  Full: commands and edits without asking|● Read only|-|‹ Back",
		"root":         "New agent…|Blank agent file…",
	} {
		names, _ := shown(allMenus(testCtx())[name])
		if got := strings.Join(names, "|"); got != want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
	_, cmds := shown(allMenus(testCtx())["item"])
	for label, want := range map[string]string{
		"Run now":               "/opt/imux __auto run 0123456789ab --client /dev/pts/3 --pane %5 --sidebar %6 --x 12 --y 7",
		"Change time…":          "__auto menu when 0123456789ab --flow time",
		"Change CLI and model…": "__auto menu cli 0123456789ab --flow cli",
		"History…":              "__auto pick history 0123456789ab",
		"Remove schedule…":      "__auto confirm remove 0123456789ab",
	} {
		if !strings.Contains(cmds[label], want) {
			t.Errorf("%s: %s", label, cmds[label])
		}
	}
	_, cmds = shown(allMenus(testCtx())["when"])
	if c := cmds["Every day at 09:00"]; !strings.Contains(c, "__auto when 0123456789ab 09:00 --flow new") {
		t.Errorf("daily: %s", c)
	}
	if c := cmds["Custom…"]; !strings.Contains(c, "__auto ask when 0123456789ab --flow new") {
		t.Errorf("custom: %s", c)
	}
	_, cmds = shown(allMenus(testCtx())["cli"])
	if c := cmds["Claude · its own"]; !strings.Contains(c, "__auto cli 0123456789ab claude --flow new --when 09:00,18:30") {
		t.Errorf("cli: %s", c)
	}
	_, cmds = shown(allMenus(testCtx())["root"])
	if c := cmds["Blank agent file…"]; !strings.Contains(c, "__skill dialog --agents --template --client /dev/pts/3") {
		t.Errorf("root: %s", c)
	}
}

// TestPicks (T66.2): the model Quick Pick's items are argv: the default
// first, the models used before, the CLI's, the name typed; the history's
// items show a run's log.
func TestPicks(t *testing.T) {
	i18n.SetLocale("en")
	c := testCtx()
	cli := CLI{ID: session.Claude, Name: "Claude"}
	p := ModelPick(c, id, evilName, FlowNew, "09:00 18:30", cli, []auto.Model{{ID: "fable"}, {ID: "opus"}}, []string{"opus", "my-model", "opus"}, "the agent's own: haiku")
	var labels []string
	for _, it := range p.Items {
		labels = append(labels, it.Group+"/"+it.Label)
	}
	if got := strings.Join(labels, "|"); got != "/Default|Used before/opus|Used before/my-model|Claude's models/fable| /Use the model name typed" {
		t.Errorf("items %s", got)
	}
	base := []string{"__auto", "model", id, "--flow", "new", "--cli", "claude", "--when", "09:00,18:30", "--cwd", evilDir, "--client", "/dev/pts/3", "--pane", "%5", "--sidebar", "%6"}
	if !reflect.DeepEqual(p.Items[0].Argv, base) || !p.Items[0].Always || p.Items[0].Description != "the agent's own: haiku" {
		t.Errorf("default %+v", p.Items[0])
	}
	if got := p.Items[3].Argv; !reflect.DeepEqual(got, append(append([]string{}, base...), "--", "fable")) {
		t.Errorf("fable %q", got)
	}
	if typed := p.Items[4]; !typed.Query || !typed.Always || !reflect.DeepEqual(typed.Argv, base) {
		t.Errorf("typed %+v", typed)
	}
	if p.Title != "Model for "+evilName+" · Claude" || p.Client != "/dev/pts/3" {
		t.Errorf("pick %q %q", p.Title, p.Client)
	}

	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.Local)
	h := HistoryPick(c, id, evilName, []auto.Run{
		{Start: now.Add(-time.Minute), End: now, Status: auto.StatusOK, Summary: "v2.3", Log: "20261112-085900.log", CLI: "claude", Model: "haiku", Manual: true},
		{Start: now.Add(-time.Hour), End: now.Add(-time.Hour + 5*time.Second), Status: auto.StatusQuiet, Log: "20261112-080000.log", CLI: "copilot"},
	}, now)
	if len(h.Items) != 2 || h.Items[0].Label != "✓ 11:59  v2.3" || h.Items[0].Description != "1m · Claude · haiku · by hand" || h.Items[1].Label != "· 11:00  nothing new" {
		t.Errorf("history %+v", h.Items)
	}
	if got := h.Items[1].Argv; !reflect.DeepEqual(got[:5], []string{"__auto", "log", id, "--log", "20261112-080000.log"}) {
		t.Errorf("history argv %q", got)
	}
	if Word(" 09:00 18:30 ") != "09:00,18:30" || !ValidFlow(FlowCLI) || ValidFlow("x") {
		t.Error("Word / ValidFlow")
	}
}
