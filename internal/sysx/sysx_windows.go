//go:build windows

package sysx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// winSignal is a signal Windows does not have; imux delivers it itself.
type winSignal string

func (s winSignal) Signal()        {}
func (s winSignal) String() string { return string(s) }

// The signals imux's processes send each other and quit on. SIGUSR1 and
// SIGTERM travel as named events (Local\imux-<name>-<pid>), which the
// receiving process creates in Notify; SIGWINCH comes from watching the
// console's size. SIGTERM also comes when the console closes.
var (
	SIGUSR1  os.Signal = winSignal("user defined signal 1")
	SIGWINCH os.Signal = winSignal("window changed")
	SIGTERM  os.Signal = syscall.SIGTERM
	SIGHUP   os.Signal = syscall.SIGHUP
	SIGINT   os.Signal = syscall.SIGINT
)

// ResizePoll is how often a TUI's console size is checked for SIGWINCH.
const ResizePoll = 150 * time.Millisecond

// eventName is the event that carries sig to the process pid, in the
// session's own namespace.
func eventName(sig os.Signal, pid int) string {
	n := "usr1"
	if sig == SIGTERM {
		n = "term"
	}
	return `Local\imux-` + n + "-" + strconv.Itoa(pid)
}

// watcher delivers a channel's signals that Windows does not have.
type watcher struct {
	stop windows.Handle // set by Stop: the event waits end
	done chan struct{}  // closed by Stop: the size poll ends
	wg   sync.WaitGroup
}

var (
	notifyMu sync.Mutex
	watchers = map[chan<- os.Signal]*watcher{}
)

// Notify delivers sig to c: the console's own signals (Ctrl+C, the console
// closing) through signal.Notify, SIGUSR1 and SIGTERM from imux's named
// events, and SIGWINCH from the console's size.
func Notify(c chan<- os.Signal, sig ...os.Signal) {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	w := watchers[c]
	if w == nil {
		stop, err := windows.CreateEvent(nil, 1, 0, nil) // manual reset
		if err != nil {
			stop = 0
		}
		w = &watcher{stop: stop, done: make(chan struct{})}
		watchers[c] = w
	}
	var std []os.Signal
	for _, s := range sig {
		switch s {
		case SIGUSR1:
			w.listen(c, s)
		case SIGWINCH:
			w.wg.Add(1)
			go w.pollSize(c)
		case SIGTERM:
			w.listen(c, s)
			std = append(std, s)
		default:
			std = append(std, s)
		}
	}
	if len(std) > 0 {
		signal.Notify(c, std...)
	}
}

// listen creates the named event for sig and forwards each time it is set.
func (w *watcher) listen(c chan<- os.Signal, sig os.Signal) {
	if w.stop == 0 {
		return
	}
	name, err := windows.UTF16PtrFromString(eventName(sig, os.Getpid()))
	if err != nil {
		return
	}
	h, err := windows.CreateEvent(nil, 0, 0, name) // auto reset
	if err != nil {
		return
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() { _ = windows.CloseHandle(h) }()
		for {
			r, err := windows.WaitForMultipleObjects([]windows.Handle{w.stop, h}, false, windows.INFINITE)
			if err != nil || r != windows.WAIT_OBJECT_0+1 {
				return // stopped
			}
			select {
			case c <- sig:
			default:
			}
		}
	}()
}

// pollSize sends SIGWINCH whenever the console's size changes.
func (w *watcher) pollSize(c chan<- os.Signal) {
	defer w.wg.Done()
	cols, rows := ConsoleSize()
	t := time.NewTicker(ResizePoll)
	defer t.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-t.C:
			if nc, nr := ConsoleSize(); nc != cols || nr != rows {
				cols, rows = nc, nr
				select {
				case c <- SIGWINCH:
				default:
				}
			}
		}
	}
}

// Stop undoes Notify for c.
func Stop(c chan<- os.Signal) {
	signal.Stop(c)
	notifyMu.Lock()
	w := watchers[c]
	delete(watchers, c)
	notifyMu.Unlock()
	if w == nil {
		return
	}
	close(w.done)
	if w.stop != 0 {
		_ = windows.SetEvent(w.stop)
	}
	w.wg.Wait()
	if w.stop != 0 {
		_ = windows.CloseHandle(w.stop)
	}
}

// Signal sends sig to the imux process pid: SIGUSR1 and SIGTERM set its named
// event; a process that did not ask for SIGTERM is terminated instead.
func Signal(pid int, sig os.Signal) error {
	if sig == SIGUSR1 || sig == SIGTERM {
		name, err := windows.UTF16PtrFromString(eventName(sig, pid))
		if err != nil {
			return err
		}
		h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
		if err == nil {
			defer func() { _ = windows.CloseHandle(h) }()
			return windows.SetEvent(h)
		}
		if sig == SIGUSR1 {
			return err
		}
	}
	if sig == SIGTERM || sig == os.Kill {
		p, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		defer func() { _ = p.Release() }()
		return p.Kill()
	}
	return fmt.Errorf("sysx: cannot send %v on Windows", sig)
}

// stillActive is the exit code of a process that has not ended.
const stillActive = 259

// Alive reports whether the process pid exists.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// lockOffsetHigh puts Lock's byte at 2^62, past any data: a Windows lock is
// mandatory, so a locked byte that holds data could not be read by others
// (watch.pid holds the watcher's pid), while flock keeps no one from reading.
const lockOffsetHigh = 1 << 30

// Lock takes an exclusive LockFileEx lock on f, waiting for it when wait is
// set. It returns false (and no error) when wait is off and another holds it.
func Lock(f *os.File, wait bool) (bool, error) {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &windows.Overlapped{OffsetHigh: lockOffsetHigh})
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION), errors.Is(err, windows.ERROR_IO_PENDING):
		return false, nil
	default:
		return false, err
	}
}

// Unlock releases Lock's lock.
func Unlock(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{OffsetHigh: lockOffsetHigh})
}

// Exec runs path (argv, env) in this console as a child, waits for it and
// exits with its exit code: Windows cannot replace a running process. Ctrl+C
// reaches the child, which decides; this process only waits. It returns only
// when the program could not be started.
func Exec(path string, argv, env []string) error {
	cmd := &exec.Cmd{Path: path, Args: argv, Env: env, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	signal.Ignore(os.Interrupt)
	if err := cmd.Start(); err != nil {
		signal.Reset(os.Interrupt)
		return err
	}
	err := cmd.Wait()
	var ee *exec.ExitError
	switch {
	case err == nil:
		os.Exit(0)
	case errors.As(err, &ee):
		os.Exit(ee.ExitCode())
	}
	os.Exit(1)
	return nil
}

// KillGroupOnCancel starts cmd in a process group of its own, with no
// console window, and makes its context's cancel end it with every process
// it started.
func KillGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	cmd.Cancel = func() error { return KillTree(cmd.Process.Pid) }
}

// TermGroup ends the process pgid (a KillGroupOnCancel command's pid) with
// every process it started: a console program has no SIGTERM to be asked by.
func TermGroup(pgid int) error { return KillTree(pgid) }

// KillGroup ends the process pgid with every process it started.
func KillGroup(pgid int) error { return KillTree(pgid) }

// KillTree ends pid and every process it started (taskkill /T).
func KillTree(pid int) error {
	k := exec.Command(filepath.Join(systemRoot(), "System32", "taskkill.exe"), "/T", "/F", "/PID", strconv.Itoa(pid))
	k.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
	if err := k.Run(); err != nil {
		p, ferr := os.FindProcess(pid)
		if ferr != nil {
			return err
		}
		defer func() { _ = p.Release() }()
		return p.Kill()
	}
	return nil
}

// systemRoot is the Windows folder.
func systemRoot() string {
	if r := os.Getenv("SystemRoot"); r != "" {
		return r
	}
	return `C:\Windows`
}

// StartDetached starts the command newCmd makes without a console window,
// in a process group of its own and out of the job of the pane that started
// it when the job allows that (so closing the pane does not end it), and
// does not wait for it.
func StartDetached(newCmd func() *exec.Cmd) error {
	cmd, err := StartDetachedCmd(newCmd)
	if err != nil {
		return err
	}
	return cmd.Process.Release()
}

// StartDetachedCmd starts the command newCmd makes as StartDetached does and
// returns it started, for the caller to wait for or leave.
func StartDetachedCmd(newCmd func() *exec.Cmd) (*exec.Cmd, error) {
	const detached = windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW
	var err error
	for _, flags := range []uint32{detached | windows.CREATE_BREAKAWAY_FROM_JOB, detached} {
		cmd := newCmd()
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		if err = cmd.Start(); err == nil {
			return cmd, nil
		}
	}
	return nil, err
}

// HomeEnv is the variable that names the user's home folder: the profile,
// which the agents' CLIs use as their home on Windows too.
const HomeEnv = "USERPROFILE"

// PathKey is path p as the file system tells paths apart, for keys and
// hashes: Windows' paths ignore case, and "/" is "\".
func PathKey(p string) string { return strings.ToLower(filepath.Clean(p)) }

// SetUmask does nothing on Windows: the files in the user's profile are the
// user's alone.
func SetUmask() {}

// PermBits reports whether a file's group and other permission bits mean
// something. On Windows the profile's permissions keep imux's files private,
// and a file's mode only says whether it is read-only.
const PermBits = false

// HasTmux reports whether tmux runs on this system: not on Windows, where
// i-mux-core is the only engine (D85).
const HasTmux = false

// ExeSuffix ends an executable's file name.
const ExeSuffix = ".exe"

// POSIXShell reports whether a POSIX sh runs the command lines i-mux gives
// its engine: not on Windows, where they take the engine's argv form (§9.2a).
const POSIXShell = false

// ShellArgv is how the user's interactive shell starts in a pane, as
// i-mux-core starts one: sh (default-shell) when it names a program that
// exists, else pwsh, else Windows PowerShell, else %COMSPEC%. PowerShell
// gets -NoLogo; Windows shells have no login form.
func ShellArgv(sh string) []string {
	if !filepath.IsAbs(sh) || !isFile(sh) {
		sh = defaultShell()
	}
	switch strings.TrimSuffix(strings.ToLower(filepath.Base(sh)), ".exe") {
	case "pwsh", "powershell":
		return []string{sh, "-NoLogo"}
	}
	return []string{sh}
}

// defaultShell is i-mux-core's shell when default-shell names none.
func defaultShell() string {
	if p, err := exec.LookPath("pwsh.exe"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	if ps := filepath.Join(systemRoot(), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"); isFile(ps) {
		return ps
	}
	if c := os.Getenv("COMSPEC"); c != "" && filepath.IsAbs(c) {
		return c
	}
	return filepath.Join(systemRoot(), "System32", "cmd.exe")
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// Executable reports whether a regular file can run: on Windows its
// extension decides (PATHEXT: .exe, .cmd, .bat, .com ...).
func Executable(fi os.FileInfo) bool {
	ext := strings.ToLower(filepath.Ext(fi.Name()))
	if ext == "" {
		return false
	}
	pe := os.Getenv("PATHEXT")
	if pe == "" {
		pe = ".COM;.EXE;.BAT;.CMD"
	}
	for _, e := range strings.Split(strings.ToLower(pe), ";") {
		if e == ext {
			return true
		}
	}
	return false
}

// CrossDevice reports whether err is a rename across volumes.
func CrossDevice(err error) bool { return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) }
