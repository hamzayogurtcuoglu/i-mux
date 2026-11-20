//go:build !windows

package testutil

import "testing"

// ModesApply: a file's unix permission bits are what keeps it private here.
const ModesApply = true

// SkipOnWindows skips t on Windows only (T90.9), saying why: the test checks
// something only macOS and Linux have.
func SkipOnWindows(testing.TB, string) {}

// Exe is a program's file name: name itself (name.exe on Windows).
func Exe(name string) string { return name }
