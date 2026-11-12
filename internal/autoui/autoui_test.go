package autoui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
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

func allMenus(c Ctx) map[string]*workspace.Menu {
	return map[string]*workspace.Menu{
		"scheduled": ItemMenu(c, Item{ID: id, Name: evilName, Record: true, Scheduled: true, Runs: 3, Continue: true}),
		"new":       ItemMenu(c, Item{ID: id, Name: evilName}),
		"by-hand":   ItemMenu(c, Item{ID: id, Name: evilName, Record: true, Runs: 1, Continue: true}),
		"running":   ItemMenu(c, Item{ID: id, Name: evilName, Record: true, Scheduled: true, Running: true, Runs: 1, Paused: true, Continue: true}),
		"gone":      ItemMenu(c, Item{ID: id, Name: evilName, Record: true, Scheduled: true, Gone: true, Runs: 2}),
		"remove":    ConfirmRemove(c, id, evilName, true),
		"forget":    ConfirmRemove(c, id, evilName, false),
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
				if strings.Contains(it.Name, "auto.") || strings.Contains(it.Name, "click.") {
					t.Errorf("%s %s: untranslated %q", loc, name, it.Name)
				}
				if !it.Disabled && (it.Key == "" || keys[it.Key]) {
					t.Errorf("%s %s: key %q of %q missing or taken", loc, name, it.Key, it.Name)
				}
				keys[it.Key] = true
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

// TestItemMenus (T66.6): Run… opens the Run panel, Change schedule… opens
// it at the time; Stop while it runs; output, runs and Continue once it
// ran; Pause or Resume for a time; Remove or Forget.
func TestItemMenus(t *testing.T) {
	i18n.SetLocale("en")
	for name, want := range map[string]string{
		"scheduled": "Run…|Output|History…|Continue in a pane|-|Change schedule…|Pause|-|Open definition|Remove schedule…",
		"new":       "Run…|-|Schedule…|-|Open definition",
		"by-hand":   "Run…|Output|Continue in a pane|-|Schedule…|-|Open definition|Forget runs…",
		"running":   "Stop|Output|-|Change schedule…|Resume|-|Open definition|Remove schedule…",
		"gone":      "Output|History…|-|Remove schedule…",
		"remove":    "-Its runs and their output go too.|-|Remove|Cancel",
	} {
		names, _ := shown(allMenus(testCtx())[name])
		if got := strings.Join(names, "|"); got != want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
	_, cmds := shown(allMenus(testCtx())["scheduled"])
	for label, want := range map[string]string{
		"Run…":             "/opt/imux __auto run 0123456789ab --client /dev/pts/3 --pane %5 --sidebar %6 --x 12 --y 7",
		"Change schedule…": "__auto run 0123456789ab --at-when",
		"History…":         "__auto pick history 0123456789ab",
		"Output":           "__auto log 0123456789ab",
		"Remove schedule…": "__auto confirm remove 0123456789ab",
	} {
		if !strings.Contains(cmds[label], want) {
			t.Errorf("%s: %s", label, cmds[label])
		}
	}
	if m := allMenus(testCtx())["forget"]; m.Title != "Forget the runs of "+evilName+"?" {
		t.Errorf("forget title %q", m.Title)
	}
}

// TestHistory (T66.6): a run's line, and its output on a click.
func TestHistory(t *testing.T) {
	i18n.SetLocale("en")
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.Local)
	h := HistoryPick(testCtx(), id, evilName, []auto.Run{
		{Start: now.Add(-time.Minute), End: now, Status: auto.StatusOK, Summary: "v2.3", Log: "20261112-085900.log", CLI: "claude", Model: "haiku", Manual: true},
		{Start: now.Add(-time.Hour), End: now.Add(-time.Hour + 5*time.Second), Status: auto.StatusSkipped, Summary: "Claude is limited"},
	}, now)
	if len(h.Items) != 2 || h.Items[0].Label != "✓ 11:59  v2.3" || h.Items[0].Description != "1m · Claude · haiku · by hand" || h.Items[1].Label != "↷ 11:00  Claude is limited" {
		t.Errorf("history %+v", h.Items)
	}
	if got := h.Items[0].Argv; !reflect.DeepEqual(got[:5], []string{"__auto", "log", id, "--log", "20261112-085900.log"}) || got[5] != "--cwd" || got[6] != evilDir {
		t.Errorf("history argv %q", got)
	}
	if got := h.Items[1].Argv; got[3] != "--cwd" {
		t.Errorf("a run without a log %q", got)
	}
}
