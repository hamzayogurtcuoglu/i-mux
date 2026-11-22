//go:build !windows

package tmux

// SidebarHooks are the hooks that wake the sidebars (`imux __sidebar-refresh`):
// a pane focused, split, killed or exited, and a window's active pane changed.
// Tests set it.
var SidebarHooks = AllSidebarHooks
