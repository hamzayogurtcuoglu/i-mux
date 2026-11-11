//go:build !windows

package winx

import "os/exec"

func hide(*exec.Cmd) {}

// WindowsPowerShell is Windows PowerShell's path ("" on Unix).
func WindowsPowerShell() string { return "" }

// GitRoot is the folder Git for Windows is installed in ("" on Unix).
func GitRoot() string { return "" }

// GitTool is the path of a program Git for Windows brings ("" on Unix).
func GitTool(string) string { return "" }

// GitBash is Git for Windows' bash ("" on Unix).
func GitBash() string { return "" }
