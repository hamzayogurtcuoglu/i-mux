//go:build !windows

package sysx

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// The signals imux's processes send each other.
var (
	SIGUSR1  os.Signal = syscall.SIGUSR1
	SIGWINCH os.Signal = syscall.SIGWINCH
	SIGTERM  os.Signal = syscall.SIGTERM
	SIGHUP   os.Signal = syscall.SIGHUP
	SIGINT   os.Signal = syscall.SIGINT
)

var stdStop = []os.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT}

// Notify is signal.Notify.
func Notify(c chan<- os.Signal, sig ...os.Signal) { signal.Notify(c, sig...) }

// Stop is signal.Stop.
func Stop(c chan<- os.Signal) { signal.Stop(c) }

// Signal sends sig to the process pid.
func Signal(pid int, sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return errors.New("sysx: not a signal")
	}
	return syscall.Kill(pid, s)
}

// Alive reports whether the process pid exists.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Lock takes an flock on f: exclusive, waiting for it when wait is set. It
// returns false (and no error) when wait is off and another process holds it.
func Lock(f *os.File, wait bool) (bool, error) {
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	for {
		err := syscall.Flock(int(f.Fd()), how)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		default:
			return false, err
		}
	}
}

// Unlock releases Lock's lock.
func Unlock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

// Exec replaces the process with path (argv, env). It returns only on failure.
func Exec(path string, argv, env []string) error { return syscall.Exec(path, argv, env) }

// ExecKeepsLocks reports whether Exec keeps the process's file locks (the
// new program is the same process). On Windows it is a new process.
const ExecKeepsLocks = true

// KillGroupOnCancel puts cmd in its own process group and makes its context
// cancel kill the whole group: children it started go too, and pipes they
// hold do not keep the caller waiting.
func KillGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

// StartDetached starts the command newCmd makes in a session of its own (no
// controlling terminal) and does not wait for it.
func StartDetached(newCmd func() *exec.Cmd) error {
	cmd, err := StartDetachedCmd(newCmd)
	if err != nil {
		return err
	}
	return cmd.Process.Release()
}

// StartDetachedCmd starts the command newCmd makes in a session of its own
// and returns it started, for the caller to wait for or leave.
func StartDetachedCmd(newCmd func() *exec.Cmd) (*exec.Cmd, error) {
	cmd := newCmd()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// SetUmask makes every file this process creates private.
func SetUmask() { syscall.Umask(0o077) }

// Executable reports whether a regular file's mode lets it run.
func Executable(fi os.FileInfo) bool { return fi.Mode().Perm()&0o111 != 0 }

// CrossDevice reports whether err is a rename across file systems.
func CrossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }
