// Package sysx hides the few operating system calls imux makes that differ
// between Unix (macOS, Linux) and Windows (D85): signals between imux's own
// processes, file locks, replacing the process with another program,
// detached and killable children, file modes, and the terminal of a TUI.
//
// On Unix every function is the plain syscall imux always used. On Windows,
// which has no SIGUSR1, SIGWINCH, flock or exec, the same behavior is built
// from named events, LockFileEx, a waited-for child and the console's API.
package sysx

import "os"

// Wake asks the imux process pid to look again now (SIGUSR1 on Unix).
func Wake(pid int) error { return Signal(pid, SIGUSR1) }

// Std returns the signals a long-running imux process quits on: SIGTERM,
// SIGHUP and SIGINT.
func Std() []os.Signal { return []os.Signal{SIGTERM, SIGHUP, SIGINT} }
