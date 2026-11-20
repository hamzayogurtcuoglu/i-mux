//go:build e2e && !windows

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// exeSuffix is a program file's suffix.
const exeSuffix = ""

// noTerminal starts a program in a session of its own, with no terminal to
// ask on.
func noTerminal() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// maxRSS is the peak resident memory of a finished program, in bytes.
func maxRSS(ps *os.ProcessState) int64 {
	r := ps.SysUsage().(*syscall.Rusage).Maxrss
	if runtime.GOOS == "linux" {
		r *= 1024 // kilobytes on Linux, bytes on macOS
	}
	return r
}

// killPid kills process pid at once (SIGKILL).
func killPid(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }

// executable reports whether st is a program file.
func executable(st os.FileInfo) bool { return st.Mode()&0o111 != 0 }

// posixShell is the sh of the panes and of the fake agents' scripts.
func posixShell() string { return "/bin/sh" }

// basePath is the suite's PATH: the engine and the base system only, never
// a real agent CLI.
func basePath(engine string) string { return filepath.Dir(engine) + ":/usr/bin:/bin" }

// stageFakes copies e2e/fakeagents' scripts (fake.sh and its wrappers) into
// dir; the Go port in fakeagent/ is Windows' fake.
func stageFakes(dir string) error {
	ents, err := os.ReadDir("fakeagents")
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join("fakeagents", e.Name()), filepath.Join(dir, e.Name()), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// skipOnSystem skips nothing: every test applies on macOS and Linux.
func skipOnSystem(*testing.T) {}

// binRunning reports whether a process runs the program at path (pgrep).
func binRunning(path string) bool {
	out, _ := exec.Command("pgrep", "-f", "^"+regexp.QuoteMeta(path)).Output()
	return len(strings.TrimSpace(string(out))) > 0
}
