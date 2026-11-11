//go:build !windows

package cli

import "strings"

// droppedPath is path as a terminal types a dropped file: spaces escaped.
func droppedPath(path string) string { return strings.ReplaceAll(path, " ", `\ `) }
