//go:build !windows

package pwshshim

// Run does nothing on Unix and reports false.
func Run([]string) (int, bool) { return 0, false }

// Dir returns "": Unix needs no shim.
func Dir() string { return "" }
