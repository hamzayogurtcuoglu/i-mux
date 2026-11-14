//go:build e2e

package e2e

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// TestThemes (M73, D68, D70): tmux, the sidebar and every pane draw with
// the theme (Catppuccin Mocha here): the panes, the agents' too, on its
// panel and text color with its 16 colors, the sidebar painting itself;
// following the terminal (tmux 3.6+), its white background turns
// everything to Catppuccin Latte; a theme chosen in Settings applies at
// once, and the i-mux theme puts back the look from before themes.
func TestThemes(t *testing.T) {
	h := newH(t, sidebarConfig+"[theme]\nname = \"catppuccin\"\nauto_switch = true\n")
	agent, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	opt := func(name string) string {
		return strings.TrimSpace(h.mustTmux("show-options", "-gqv", name))
	}
	colors := func() string {
		out, _ := h.tmux("capture-pane", "-e", "-p", "-t", bar)
		return out
	}
	if a, b := opt("@imux_c_accent"), opt("pane-active-border-style"); a != "#89b4fa" || b != "bg=#181825,fg=#89b4fa" {
		t.Fatalf("Mocha's accent: %q, border %q", a, b)
	}
	// Every pane on Mocha's panel and text color, with its red; the agent's
	// pane gets them from the window (no style of its own).
	if w, r := opt("window-style"), opt(tmux.OptPaneColors+"[1]"); w != "bg=#181825,fg=#cdd6f4" || r != "#f38ba8" {
		t.Fatalf("panes: %q, red %q", w, r)
	}
	if bg := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agent, "#{pane_bg}")); bg != "" && bg != "#181825" {
		t.Logf("the agent's pane_bg %q", bg)
	}
	h.waitFor("the sidebar in Mocha's text color on its panel", 5*time.Second, func() bool {
		c := colors()
		return strings.Contains(c, "38;2;205;214;244") && strings.Contains(c, "48;2;24;24;37") // #cdd6f4 on #181825
	})

	// The terminal turns light: its own report (DEC mode 2031, which tmux
	// 3.6+ asks terminals for); tmux runs its client-light-theme hook and
	// the theme follows. Before 3.6 nothing tells i-mux (a pane's OSC 11
	// answer is the theme's own background).
	if !tmuxAtLeast(h, 3, 6) {
		t.Log("tmux before 3.6: no light/dark hooks; following the terminal is not tested")
	} else {
		c.Write("\x1b[?997;2n")
		h.waitFor("light: Catppuccin Latte", 10*time.Second, func() bool {
			return opt("@imux_appearance") == "light" && opt("@imux_c_accent") == "#1e66f5"
		})
		h.waitFor("the sidebar on Latte's panel", 5*time.Second, func() bool {
			return strings.Contains(colors(), "48;2;239;241;245") // #eff1f5
		})
		if w := opt("window-style"); !strings.HasPrefix(w, "bg=#eff1f5,fg=#") {
			t.Fatalf("the panes on Latte: %q", w)
		}
	}

	// Settings → Theme → Nord: tmux at once, and no more following.
	h.mustImux("__settings", "set", "theme", "nord")
	if a := opt("@imux_c_accent"); a != "#88c0d0" {
		t.Fatalf("Nord's accent at once: %q", a)
	}
	h.waitFor("the sidebar in Nord's text color", 10*time.Second, func() bool {
		return strings.Contains(colors(), "38;2;236;239;244") // #eceff4
	})

	// Settings → Theme → i-mux: the colors before themes, tmux's own
	// borders and menus, the terminal's background.
	h.mustImux("__settings", "set", "theme", "i-mux")
	// (The watcher, restarted by the config's change, loads tmux.conf again;
	// a quick second change may land before it.)
	h.waitFor("i-mux: tmux's own borders, the panes on the terminal's colors", 10*time.Second, func() bool {
		a, b, w := opt("@imux_c_accent"), opt("pane-active-border-style"), opt("window-style")
		return a == "cyan" && !strings.Contains(b, "#88c0d0") && w == "default"
	})
	h.waitFor("the sidebar in i-mux's colors on the terminal's background", 10*time.Second, func() bool {
		c := colors()
		return strings.Contains(c, "38;5;247") && !strings.Contains(c, "38;2;") && !strings.Contains(c, "48;2;")
	})
}

// tmuxAtLeast reports whether the tests' tmux is maj.min or later.
func tmuxAtLeast(h *H, maj, min int) bool {
	out, err := exec.Command(h.Tmux, "-V").Output()
	if err != nil {
		return false
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		return false
	}
	v := strings.TrimRight(f[1], "abcdefghijklmnopqrstuvwxyz")
	a, b, _ := strings.Cut(v, ".")
	x, _ := strconv.Atoi(a)
	y, _ := strconv.Atoi(b)
	return x > maj || x == maj && y >= min
}
