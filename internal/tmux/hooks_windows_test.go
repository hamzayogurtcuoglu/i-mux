//go:build windows

package tmux

import (
	"slices"
	"strings"
	"testing"
)

// windowsHooks is SidebarHooks as Windows sets it, before TestMain sets the
// full list for the goldens.
var windowsHooks = SidebarHooks

// On Windows one pane switch starts one imux.exe, not two: pane-focus-in
// wakes the sidebars, window-pane-changed (fired with it) does not.
func TestSidebarHooksWindows(t *testing.T) {
	if slices.Contains(windowsHooks, "window-pane-changed") || !slices.Contains(windowsHooks, "pane-focus-in") {
		t.Fatalf("hooks %q", windowsHooks)
	}
	old := SidebarHooks
	SidebarHooks = windowsHooks
	t.Cleanup(func() { SidebarHooks = old })
	conf := GenerateConf(confInput("3.8"))
	if strings.Contains(conf, "window-pane-changed") || strings.Count(conf, "__sidebar-refresh") != len(windowsHooks) {
		t.Fatalf("conf:\n%s", conf)
	}
}
