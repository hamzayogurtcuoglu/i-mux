//go:build windows

package testutil

import "testing"

// ModesApply: Windows has no unix permission bits (Go reports 0666 or
// 0444); a profile's ACL keeps i-mux's files private there.
const ModesApply = false

// SkipOnWindows skips t on Windows only (T90.9), saying why: the test checks
// something only macOS and Linux have.
func SkipOnWindows(t testing.TB, why string) {
	t.Helper()
	t.Skip("not on Windows: " + why)
}

// Exe is a program's file name: name.exe (name itself on macOS and Linux).
func Exe(name string) string { return name + ".exe" }
