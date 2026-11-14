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
// (Catppuccin Mocha by default); following the terminal, the sidebar's
// OSC 11 answer of a white background turns everything to Catppuccin
// Latte; a theme chosen in Settings applies at once.
func TestThemes(t *testing.T) {
	h := newH(t, sidebarConfig+"[theme]\nauto_switch = true\n")
	_, bar := h.startWithSidebar()
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
	h.waitFor("the sidebar in Latte's text color", 5*time.Second, func() bool {
		return strings.Contains(colors(), "38;2;76;79;105") // #4c4f69
	})

	// Settings → Theme → Nord: tmux at once, and no more following.
	h.mustImux("__settings", "set", "theme", "nord")
	if a := opt("@imux_c_accent"); a != "#88c0d0" {
		t.Fatalf("Nord's accent at once: %q", a)
	}
	h.waitFor("the sidebar in Nord's text color", 10*time.Second, func() bool {
		return strings.Contains(colors(), "38;2;236;239;244") // #eceff4
	})
}
