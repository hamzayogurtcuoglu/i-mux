//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestThemes (M73, D68): tmux and the sidebar draw with the theme
// (Catppuccin Mocha here), the sidebar on the theme's background and text
// color, the agent's pane untouched; following the terminal, its white
// background turns everything to Catppuccin Latte; a theme chosen in
// Settings applies at once, and the i-mux theme puts back the look from
// before themes.
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
	if a, b := opt("@imux_c_accent"), opt("pane-active-border-style"); a != "#89b4fa" || !strings.Contains(b, "#89b4fa") {
		t.Fatalf("Mocha's accent: %q, border %q", a, b)
	}
	h.waitFor("the sidebar in Mocha's text color", 5*time.Second, func() bool {
		return strings.Contains(colors(), "38;2;205;214;244") // #cdd6f4
	})
	// The sidebar paints Mocha's panel itself (a tmux style would make
	// tmux answer OSC 11 with it); the agent's pane keeps its own colors.
	h.waitFor("the sidebar on Mocha's panel", 5*time.Second, func() bool {
		return strings.Contains(colors(), "48;2;24;24;37") // #181825
	})
	for _, p := range []string{agent, bar} {
		if s := strings.TrimSpace(h.mustTmux("show-options", "-pqv", "-t", p, "window-style")); s != "" {
			t.Fatalf("pane %s has a tmux style: %q", p, s)
		}
	}

	// The terminal says its background is white: the answer to tmux's own
	// OSC 11 query when the client attached. tmux then knows the client's
	// background, and the theme follows (tmux 3.6+'s client-light-theme
	// hook; the sidebar's OSC 11 query, which tmux answers with it).
	c.Write("\x1b]11;rgb:ffff/ffff/ffff\x1b\\")
	time.Sleep(300 * time.Millisecond)
	// A resize makes the sidebar ask again (tmux before 3.6 has no hook).
	if pid, err := strconv.Atoi(h.paneOpt(bar, "#{@imux_sidebar_pid}")); err == nil && pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGWINCH)
	}
	h.waitFor("light: Catppuccin Latte", 10*time.Second, func() bool {
		return opt("@imux_appearance") == "light" && opt("@imux_c_accent") == "#1e66f5"
	})
	h.waitFor("the sidebar on Latte's panel", 5*time.Second, func() bool {
		return strings.Contains(colors(), "48;2;239;241;245") // #eff1f5
	})

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
	if a, b := opt("@imux_c_accent"), opt("pane-active-border-style"); a != "cyan" || strings.Contains(b, "#88c0d0") {
		t.Fatalf("i-mux: accent %q, border %q", a, b)
	}
	h.waitFor("the sidebar in i-mux's colors on the terminal's background", 10*time.Second, func() bool {
		c := colors()
		return strings.Contains(c, "38;5;247") && !strings.Contains(c, "38;2;") && !strings.Contains(c, "48;2;")
	})
}
