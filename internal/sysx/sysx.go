// Package sysx hides the few operating system calls imux makes that differ
// between Unix (macOS, Linux) and Windows: signals between imux's own
// processes, file locks, replacing the process with another program,
// detached and killable children, and the terminal of a TUI.
//
// On Unix every function is the plain syscall imux always used. On Windows,
// which has no SIGUSR1, SIGWINCH, flock or exec, the same behavior is built
// from named events, LockFileEx, a waited-for child and console modes.
package sysx

import "os"

// Wake asks the imux process pid to look again now (SIGUSR1 on Unix).
func Wake(pid int) error { return Signal(pid, SIGUSR1) }

// Std returns the standard stop signals a long-running imux process quits
// on: SIGTERM, SIGHUP and SIGINT where the system has them.
func Std() []os.Signal { return stdStop }
