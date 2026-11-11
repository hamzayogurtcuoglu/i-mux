//go:build windows

package cli

import "strings"

// droppedPath is path as Windows Terminal types a dropped file: in double
// quotes when it has a space (a backslash is a folder separator here, so
// "\ " cannot escape one).
func droppedPath(path string) string {
	if strings.ContainsAny(path, " \t") {
		return `"` + path + `"`
	}
	return path
}
