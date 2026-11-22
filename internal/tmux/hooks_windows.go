//go:build windows

package tmux

// SidebarHooks are the hooks that wake the sidebars (`imux __sidebar-refresh`).
// On Windows, without window-pane-changed: the engine fires it with
// pane-focus-in on every select-pane, and each hook starts an imux.exe and
// redraws every sidebar, which a Windows process start makes slow (the
// sidebars caught up a second after a switch). Tests set it.
var SidebarHooks = []string{"pane-focus-in", "after-split-window", "after-kill-pane", "pane-exited"}
