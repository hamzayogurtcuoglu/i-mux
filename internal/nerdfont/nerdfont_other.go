//go:build !windows

package nerdfont

// program is the helper to run for name: the one on PATH.
func program(name string) string { return name }
