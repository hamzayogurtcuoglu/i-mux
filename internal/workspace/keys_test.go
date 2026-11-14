package workspace

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/quickpick"
	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
)

// rows are a pick's rows as "label = keys" with what they run.
func rows(p *quickpick.Pick) string {
	var out []string
	for _, it := range p.Items {
		out = append(out, it.Group+" | "+it.Label+" = "+it.Description+" → "+strings.Join(it.Argv, " "))
	}
	return strings.Join(out, "\n")
}

// TestKeysPick (T72.4): the help lists actions with the keys the config
// binds (a key set to "" is left out), in groups, each running its action
// for the pane and client; Ctrl+V only while pasting images is on.
func TestKeysPick(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	p := &Pane{ID: "%3"}
	got := rows(w.KeysPick(p, "c1", "C-b"))
	for _, want := range []string{
		"Agents | Pane menu = Ctrl+\\  ·  prefix a → __menu --pane %3 --client c1",
		"Agents | Continue in Claude = Ctrl+\\ then c → switch claude --pane %3 --client c1",
		"Agents | Continue in OpenCode = Ctrl+\\ then e → switch opencode",
		"Agents | Earlier conversations = prefix S → __menu --sessions",
		"Agents | Go to a file of the project = Ctrl+P → __goto",
		"Agents | Paste the clipboard's image into an agent = Ctrl+V  ·  Alt+V → ",
		"Panes | Split right = prefix v → __menu --split h",
		"Panes | Move to another pane = prefix ←↑→↓ → ",
		"Panes | Swap with the pane on the left = prefix H → __pane swap left --pane %3 --client c1",
		"Panes | Swap with the pane on the right = prefix L → __pane swap right",
		"Panes | Resize panes (h j k l, Esc when done) = prefix r → __pane resize",
		"Tabs | New tab = prefix c → __menu --tab",
		"Sidebar | Show or hide the sidebar = prefix b → sidebar toggle",
		"Sidebar | These keys = prefix ?  ·  ? → ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	if pk := w.KeysPick(p, "c1", "C-b"); pk.Title != "Keys · prefix = Ctrl+B" || pk.Client != "c1" || pk.Pane != "%3" {
		t.Errorf("pick %q %q %q", pk.Title, pk.Client, pk.Pane)
	}
	// What the config changes or turns off.
	w.Cfg.Paste.Images, w.Cfg.Tmux.SwapPaneKeys, w.Cfg.Tmux.ResizeKey = false, false, ""
	w.Cfg.Tmux.MenuKeyRoot, w.Cfg.Tmux.SplitRightKey, w.Cfg.Tmux.DirectPaneKeys = "", "|", true
	got = rows(w.KeysPick(p, "", "C-a"))
	for _, gone := range []string{"Ctrl+V", "Swap with", "Resize panes", "Ctrl+\\"} {
		if strings.Contains(got, gone) {
			t.Errorf("lists %q:\n%s", gone, got)
		}
	}
	for _, want := range []string{
		"Pane menu = prefix a →", "Continue in Claude = prefix a then c →", "Split right = prefix | →",
		"Move to another pane = prefix ←↑→↓  ·  Ctrl+←↑→↓", "Split left, down, up or right = Ctrl+Alt+←↑→↓",
		"The prefix = Ctrl+A: press, release, then the key",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
}

func TestKeyName(t *testing.T) {
	for in, want := range map[string]string{
		`C-\`: `Ctrl+\`, "C-p": "Ctrl+P", "M-v": "Alt+V", "C-M-Left": "Ctrl+Alt+←", "S-Up": "Shift+↑",
		"a": "a", "S": "S", "?": "?", "": "",
	} {
		if got := KeyName(in); got != want {
			t.Errorf("KeyName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNeighbor (T72.1): the pane across the border on a side, the one
// sharing the most of it; none past the window's edge.
func TestNeighbor(t *testing.T) {
	// A sidebar (%0), then %1 over %2, and %3 on the right, full height.
	panes := []PaneBox{
		{ID: "%0", Left: 0, Top: 0, Right: 42, Bottom: 39, Role: "sidebar"},
		{ID: "%1", Left: 44, Top: 0, Right: 99, Bottom: 9},
		{ID: "%2", Left: 44, Top: 11, Right: 99, Bottom: 39},
		{ID: "%3", Left: 101, Top: 0, Right: 159, Bottom: 39},
	}
	for _, c := range []struct{ cur, dir, want string }{
		{"%1", DirLeft, "%0"},
		{"%1", DirRight, "%3"},
		{"%1", DirDown, "%2"},
		{"%1", DirUp, ""},
		{"%2", DirUp, "%1"},
		{"%3", DirLeft, "%2"}, // %2 shares more rows than %1
		{"%3", DirRight, ""},
		{"%0", DirLeft, ""},
		{"%0", DirRight, "%2"},
		{"%9", DirLeft, ""},
	} {
		if got := Neighbor(panes, c.cur, c.dir); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.cur, c.dir, got, c.want)
		}
	}
}

// TestSwapPane (T72.1): swap-pane -d with the neighbor (the moved pane
// keeps the focus); a sidebar, the window's edge or a zoomed window: no
// swap.
func TestSwapPane(t *testing.T) {
	w, f, _ := testWS(t)
	layout := "%0\t0\t0\t42\t39\tsidebar\t0\n%1\t44\t0\t99\t39\t\t0\n%2\t101\t0\t159\t39\t\t0\n"
	f.replies["list-panes"] = layout
	swapped := func(pane, dir string) string {
		f.calls = nil
		if _, err := w.SwapPane(t.Context(), pane, dir); err != nil {
			t.Fatal(err)
		}
		for _, c := range f.calls {
			if j := strings.Join(c, " "); strings.Contains(j, "swap-pane") {
				return j[strings.Index(j, "swap-pane"):]
			}
		}
		return ""
	}
	if got := swapped("%1", DirRight); got != "swap-pane -d -s %1 -t %2" {
		t.Errorf("right: %q", got)
	}
	for _, c := range [][2]string{{"%1", DirLeft}, {"%0", DirRight}, {"%2", DirRight}, {"%1", DirUp}} {
		if got := swapped(c[0], c[1]); got != "" {
			t.Errorf("%s %s swapped: %q", c[0], c[1], got)
		}
	}
	f.replies["list-panes"] = strings.ReplaceAll(layout, "\t0\n", "\t1\n")
	if got := swapped("%1", DirRight); got != "" {
		t.Errorf("zoomed: %q", got)
	}
}

// The split menu opens a pane left of or above the target with before
// (Ctrl+Alt+← ↑).
func TestSplitMenuBefore(t *testing.T) {
	i18n.SetLocale("en")
	w, _, _ := testWS(t)
	m := w.SplitMenu(&Pane{ID: "%1"}, "c1", true, true)
	if m.Title != "Split left: new pane with…" || !strings.Contains(m.Items[0].Cmd, "split claude -h --before --pane %1") {
		t.Errorf("left: %q %q", m.Title, m.Items[0].Cmd)
	}
	if m := w.SplitMenu(&Pane{ID: "%1"}, "", false, false); m.Title != "Split down: new pane with…" || strings.Contains(m.Items[0].Cmd, "--before") {
		t.Errorf("down: %q %q", m.Title, m.Items[0].Cmd)
	}
}

// TestSetAppearance (T73.4): the terminal's light or dark is kept in
// @imux_appearance; following it, tmux takes the other theme's colors at
// once; the same appearance again does nothing.
func TestSetAppearance(t *testing.T) {
	defer theme.Set(theme.Builtin(theme.Default))
	w, f, _ := testWS(t)
	f.replies[""] = "tmux 3.4" // tmux -V
	run := func(a string) (bool, string) {
		f.calls = nil
		changed, err := w.SetAppearance(t.Context(), a)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, c := range f.calls {
			b.WriteString(strings.Join(c, " ") + "\n")
		}
		return changed, b.String()
	}
	f.replies["show-options"] = "dark\n"
	if changed, calls := run("dark"); changed || strings.Contains(calls, "set-option") {
		t.Errorf("the same appearance: %v\n%s", changed, calls)
	}
	changed, calls := run("light")
	if !changed || !strings.Contains(calls, "set-option -g @imux_appearance light") || strings.Contains(calls, "@imux_c_accent") {
		t.Errorf("not following: %v\n%s", changed, calls)
	}
	w.Cfg.Theme.AutoSwitch = true
	if _, calls := run("light"); !strings.Contains(calls, "set-option -g @imux_c_accent #1e66f5") || !strings.Contains(calls, "menu-style") {
		t.Errorf("following, light: not Latte's colors\n%s", calls)
	}
	if changed, _ := run("bright"); changed {
		t.Error("a bad appearance changed it")
	}
}
