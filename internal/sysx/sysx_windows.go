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

// The signals imux's processes send each other. SIGUSR1 and SIGTERM travel
// as named events (Local\imux-<name>-<pid>), which the receiving process
// creates in Notify; SIGWINCH comes from watching the console's size.
var (
	SIGUSR1  os.Signal = winSignal("user defined signal 1")
	SIGWINCH os.Signal = winSignal("window changed")
	SIGTERM  os.Signal = syscall.SIGTERM
	SIGHUP   os.Signal = syscall.SIGHUP
	SIGINT   os.Signal = syscall.SIGINT
)

var stdStop = []os.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT}

// ResizePoll is how often a TUI's console size is checked for SIGWINCH.
const ResizePoll = 150 * time.Millisecond

func eventName(sig os.Signal, pid int) string {
	n := "usr1"
	if sig == SIGTERM {
		n = "term"
	}
	return `Local\imux-` + n + "-" + strconv.Itoa(pid)
}

type watcher struct {
	done   chan struct{}
	events []windows.Handle
}

var (
	notifyMu sync.Mutex
	watchers = map[chan<- os.Signal]*watcher{}
)

// Notify delivers sig to c: the console's own signals (Ctrl+C, the window
// closing) through signal.Notify, and SIGUSR1, SIGTERM and SIGWINCH from
// imux's named events and the console's size.
func Notify(c chan<- os.Signal, sig ...os.Signal) {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	w := watchers[c]
	if w == nil {
		w = &watcher{done: make(chan struct{})}
		watchers[c] = w
	}
	var std []os.Signal
	for _, s := range sig {
		switch s {
		case SIGUSR1:
			w.listen(c, s)
		case SIGWINCH:
			go pollSize(c, w.done)
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
	name, err := windows.UTF16PtrFromString(eventName(sig, os.Getpid()))
	if err != nil {
		return
	}
	h, err := windows.CreateEvent(nil, 0, 0, name) // auto-reset
	if err != nil {
		return
	}
	w.events = append(w.events, h)
	go func() {
		for {
			r, err := windows.WaitForSingleObject(h, 250)
			select {
			case <-w.done:
				return
			default:
			}
			if err != nil {
				return
			}
			if r == windows.WAIT_OBJECT_0 {
				select {
				case c <- sig:
				default:
				}
			}
		}
	}()
}

// pollSize sends SIGWINCH whenever the console's size changes.
func pollSize(c chan<- os.Signal, done chan struct{}) {
	w, h := ConsoleSize()
	t := time.NewTicker(ResizePoll)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			if nw, nh := ConsoleSize(); nw != w || nh != h {
				w, h = nw, nh
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
	defer notifyMu.Unlock()
	if w := watchers[c]; w != nil {
		close(w.done)
		for _, h := range w.events {
			_ = windows.CloseHandle(h)
		}
		delete(watchers, c)
	}
}

// Signal sends sig to the imux process pid: SIGUSR1 and SIGTERM set its named
// event (a process that did not ask for SIGTERM is terminated instead).
func Signal(pid int, sig os.Signal) error {
	if sig == SIGUSR1 || sig == SIGTERM {
		name, err := windows.UTF16PtrFromString(eventName(sig, pid))
		if err != nil {
			return err
		}
		h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
		if err == nil {
			defer windows.CloseHandle(h)
			return windows.SetEvent(h)
		}
		if sig == SIGUSR1 {
			return err
		}
	}
	if sig == SIGTERM || sig == os.Kill || sig == syscall.SIGKILL {
		p, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return p.Kill()
	}
	return fmt.Errorf("sysx: cannot send %v on Windows", sig)
}

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
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// Lock takes an exclusive LockFileEx lock on f's first byte, waiting for it
// when wait is set. It returns false (and no error) when wait is off and
// another process holds it.
func Lock(f *os.File, wait bool) (bool, error) {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, ol)
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
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}

// ExecKeepsLocks reports whether Exec keeps the process's file locks: on
// Windows the program runs as a child, so a lock the new program must take
// has to be released first.
const ExecKeepsLocks = false

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

// KillGroupOnCancel starts cmd in a process group of its own and makes its
// context cancel kill it with every process it started (taskkill /T).
func KillGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	cmd.Cancel = func() error { return KillTree(cmd.Process.Pid) }
}

// KillTree ends pid and every process it started.
func KillTree(pid int) error {
	k := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid))
	k.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
	if err := k.Run(); err != nil {
		if p, ferr := os.FindProcess(pid); ferr == nil {
			return p.Kill()
		}
		return err
	}
	return nil
}

// StartDetached starts the command newCmd makes without a window and outside
// the job of the pane that started it (when the job allows that), so closing
// the pane does not end it, and does not wait for it.
func StartDetached(newCmd func() *exec.Cmd) error {
	flags := []uint32{
		windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW | windows.CREATE_BREAKAWAY_FROM_JOB,
		windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}
	var err error
	for _, f := range flags {
		cmd := newCmd()
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: f, HideWindow: true}
		if err = cmd.Start(); err == nil {
			return cmd.Process.Release()
		}
	}
	return err
}

// SetUmask does nothing on Windows: files in the user's profile are private.
func SetUmask() {}

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
